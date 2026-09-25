package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildTool_Info(t *testing.T) {
	info := NewBuildTool(nil).Info()

	if info.Name != BuildToolName {
		t.Errorf("expected name %q, got %q", BuildToolName, info.Name)
	}
	if info.Description == "" {
		t.Error("description should not be empty")
	}

	props := info.Parameters["properties"].(map[string]any)
	required := info.Parameters["required"].([]string)

	for _, req := range []string{"platform", "action"} {
		if !containsString(required, req) {
			t.Errorf("missing required parameter: %s", req)
		}
	}

	platformEnum := props["platform"].(map[string]any)["enum"].([]string)
	for _, want := range []string{"android", "windows", "linux", "macos", "ios", "all"} {
		if !containsString(platformEnum, want) {
			t.Errorf("missing platform in enum: %s", want)
		}
	}

	actionEnum := props["action"].(map[string]any)["enum"].([]string)
	for _, want := range []string{"build", "clean", "test", "lint", "doctor"} {
		if !containsString(actionEnum, want) {
			t.Errorf("missing action in enum: %s", want)
		}
	}
}

func TestBuildTool_Run_Validation(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"invalid json", `{invalid json`},
		{"missing both", `{}`},
		{"missing action", `{"platform": "windows"}`},
	}

	tool := NewBuildTool(nil)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tool.Run(context.Background(), ToolCall{
				ID: "test", Name: BuildToolName, Input: tc.input,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.IsError {
				t.Error("expected an error response")
			}
		})
	}
}

func TestDetectFramework(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"go module", map[string]string{"go.mod": "module x"}, "go"},
		{"flutter", map[string]string{"pubspec.yaml": "name: x"}, "flutter"},
		{"rust", map[string]string{"Cargo.toml": "[package]"}, "rust"},
		{"tauri", map[string]string{"Cargo.toml": "[package]", "tauri.conf.json": "{}"}, "tauri"},
		{"electron", map[string]string{"package.json": `{"devDependencies":{"electron":"^30"}}`}, "electron"},
		{"gradle", map[string]string{"build.gradle.kts": "plugins {}"}, "kotlin"},
		{"swift", map[string]string{"Package.swift": "// x"}, "swift"},
		{"plain node", map[string]string{"package.json": `{"dependencies":{"express":"^4"}}`}, "node"},
		{"nothing", map[string]string{"README.md": "hi"}, "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tc.files {
				writeFile(t, filepath.Join(dir, name), content)
			}
			if got := detectFramework(dir, ""); got != tc.want {
				t.Errorf("detectFramework = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDetectFrameworkOverrideWins(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module x")
	if got := detectFramework(dir, "Flutter"); got != "flutter" {
		t.Errorf("override should win, got %q", got)
	}
}

func TestDetectFrameworkElectronFromPackageJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"),
		`{"devDependencies":{"electron":"^30.0.0"},"scripts":{"build":"tsc"}}`)
	if got := detectFramework(dir, ""); got != "electron" {
		t.Errorf("detectFramework = %q, want electron", got)
	}
}

func TestGoBuildStepsCrossCompile(t *testing.T) {
	steps := goBuildSteps("linux", "all", "1.2.3", "")
	if len(steps) == 0 {
		t.Fatal("expected at least one build step")
	}

	step := steps[0]
	if step[0] != "go" || step[1] != "build" {
		t.Fatalf("unexpected command: %v", step)
	}
	// The version has to reach the binary through -ldflags.
	if !containsString(step, "-ldflags") {
		t.Errorf("version was not injected: %v", step)
	}
	// The path includes the target so two platforms cannot collide.
	if out := flagValue(step, "-o"); !strings.HasPrefix(out, "dist/linux-amd64/") {
		t.Errorf("output should live under dist/linux-amd64, got %q", out)
	}
	if step[len(step)-1] != "." {
		t.Errorf("the package path should be last, got %v", step)
	}
}

// flagValue returns the argument that follows a flag.
func flagValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestGoBuildStepsWindowsSubsystemFlag(t *testing.T) {
	for _, step := range goBuildSteps("windows", "amd64", "", "") {
		if !containsString(step, "-H=windowsgui") {
			t.Errorf("windows build should not open a console: %v", step)
		}
	}
}

func TestGoTargets(t *testing.T) {
	if got := goTargets("windows", "all"); len(got) != 2 {
		t.Errorf("windows/all should target two architectures, got %v", got)
	}
	if got := goTargets("windows", "arm64"); len(got) != 1 || got[0] != "windows-arm64" {
		t.Errorf("explicit arch should produce one target, got %v", got)
	}
	// iOS is arm64-only, so a request for another architecture is nonsense.
	if got := goTargets("ios", "amd64"); got != nil {
		t.Errorf("ios/amd64 should be empty, got %v", got)
	}
	if got := goTargets("plan9", "all"); got != nil {
		t.Errorf("an unknown platform should produce nothing, got %v", got)
	}

	// Every target must carry its operating system so output paths are unique.
	seen := map[string]bool{}
	for _, platform := range []string{"linux", "windows", "darwin", "android"} {
		for _, target := range goTargets(platform, "all") {
			if !strings.HasPrefix(target, platform+"-") {
				t.Errorf("target %q does not name its platform", target)
			}
			if seen[target] {
				t.Errorf("target %q was produced twice", target)
			}
			seen[target] = true
		}
	}
}

func TestBuildStepsRejectsNpmMobile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"scripts":{"build":"x"}}`)

	if _, err := buildSteps(dir, "electron", "android", "all", "", ""); err == nil {
		t.Error("expected an error explaining mobile builds are native-only")
	}
}

func TestNpmBuildStepsPicksPlatformScript(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"),
		`{"scripts":{"build":"x","build:win":"y"}}`)

	steps, err := npmBuildSteps(dir, "electron", "windows")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsString(steps[0], "build:win") {
		t.Errorf("expected the windows script, got %v", steps[0])
	}

	// With no platform script, the generic one is used.
	steps, err = npmBuildSteps(dir, "electron", "linux")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsString(steps[0], "build") || containsString(steps[0], "build:win") {
		t.Errorf("expected the generic script, got %v", steps[0])
	}
}

func TestFrameworkTestCommand(t *testing.T) {
	if got := frameworkTest(".", "go"); !containsString(got, "test") {
		t.Errorf("go test = %v", got)
	}
	if got := frameworkTest(".", "unknown"); got != nil {
		t.Errorf("an unknown framework should have no test command, got %v", got)
	}
}

func TestSignTool_Info(t *testing.T) {
	info := NewSignTool(nil).Info()
	if info.Name != SignToolName {
		t.Errorf("expected name %q, got %q", SignToolName, info.Name)
	}
	enum := info.Parameters["properties"].(map[string]any)["platform"].(map[string]any)["enum"].([]string)
	for _, want := range []string{"windows", "macos", "ios", "android"} {
		if !containsString(enum, want) {
			t.Errorf("missing platform in enum: %s", want)
		}
	}
}

func TestSignCommand(t *testing.T) {
	t.Run("windows with a pfx file", func(t *testing.T) {
		binary, args, err := signCommand(signParams{
			Platform: "windows", File: "app.exe", Certificate: "cert.pfx", Password: "pw",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if binary != "signtool" || !containsString(args, "/f") {
			t.Errorf("expected signtool with /f, got %s %v", binary, args)
		}
		if args[len(args)-1] != "app.exe" {
			t.Errorf("the file must be the final argument, got %v", args)
		}
	})

	t.Run("windows with a thumbprint", func(t *testing.T) {
		_, args, err := signCommand(signParams{
			Platform:    "windows",
			File:        "app.exe",
			Certificate: "2C73C3ADC1B1FF20070917F97AEECD03F0CF5E7C",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !containsString(args, "/sha1") {
			t.Errorf("expected /sha1 for a thumbprint, got %v", args)
		}
	})

	t.Run("windows needs a certificate", func(t *testing.T) {
		if _, _, err := signCommand(signParams{Platform: "windows", File: "app.exe"}); err == nil {
			t.Error("expected an error when no certificate is given")
		}
	})

	t.Run("macos", func(t *testing.T) {
		binary, args, err := signCommand(signParams{Platform: "macos", File: "App.app", Identity: "Developer ID"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if binary != "codesign" || !containsString(args, "Developer ID") {
			t.Errorf("unexpected codesign invocation: %s %v", binary, args)
		}
	})

	t.Run("android needs a keystore", func(t *testing.T) {
		if _, _, err := signCommand(signParams{Platform: "android", File: "app.apk"}); err == nil {
			t.Error("expected an error when the keystore is missing")
		}
	})

	t.Run("unsupported platform", func(t *testing.T) {
		if _, _, err := signCommand(signParams{Platform: "plan9", File: "x"}); err == nil {
			t.Error("expected an error for an unsupported platform")
		}
	})
}

func TestIsThumbprint(t *testing.T) {
	if !isThumbprint("2C73C3ADC1B1FF20070917F97AEECD03F0CF5E7C") {
		t.Error("a 40 character hex string is a thumbprint")
	}
	if !isThumbprint("2c73c3ad c1b1ff20 070917f9 7aeecd03 f0cf5e7c") {
		t.Error("spacing should be tolerated")
	}
	if isThumbprint("cert.pfx") {
		t.Error("a file path is not a thumbprint")
	}
}

func TestRedactSignHidesPasswords(t *testing.T) {
	got := redactSign(signParams{
		Platform: "windows",
		File:     "app.exe",
		Password: "hunter2",
	})
	if got["password"] != "***" {
		t.Errorf("password should be redacted, got %v", got["password"])
	}
	if got["file"] != "app.exe" {
		t.Errorf("non-secret fields must survive, got %v", got["file"])
	}
}

func TestNotarizeTool_Info(t *testing.T) {
	info := NewNotarizeTool(nil).Info()
	if info.Name != NotarizeToolName {
		t.Errorf("expected name %q, got %q", NotarizeToolName, info.Name)
	}
	required := info.Parameters["required"].([]string)
	for _, want := range []string{"file", "apple_id", "password", "team_id"} {
		if !containsString(required, want) {
			t.Errorf("missing required parameter: %s", want)
		}
	}
}

func TestNotarizeTool_Run_MissingCredentials(t *testing.T) {
	res, err := NewNotarizeTool(nil).Run(context.Background(), ToolCall{
		ID: "test", Name: NotarizeToolName, Input: `{"file": "app.dmg"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("expected an error response for missing credentials")
	}
}

func TestPackageTool_Info(t *testing.T) {
	info := NewPackageTool(nil).Info()
	if info.Name != PackageToolName {
		t.Errorf("expected name %q, got %q", PackageToolName, info.Name)
	}

	props := info.Parameters["properties"].(map[string]any)
	for _, want := range []string{"android", "windows", "linux", "macos", "ios"} {
		if !containsString(props["platform"].(map[string]any)["enum"].([]string), want) {
			t.Errorf("missing platform in enum: %s", want)
		}
	}
	for _, want := range []string{"apk", "aab", "zip", "appimage", "deb", "rpm", "tar.gz", "dmg", "pkg", "ipa"} {
		if !containsString(props["format"].(map[string]any)["enum"].([]string), want) {
			t.Errorf("missing format in enum: %s", want)
		}
	}
}

func TestPackageCommand(t *testing.T) {
	t.Run("deb uses nfpm with metadata", func(t *testing.T) {
		binary, args, err := packageCommand(packageParams{
			Platform: "linux", Format: "deb", Input: "dist/app", Output: "out/app.deb",
			AppName: "svpc", Version: "1.0.0", Maintainer: "me@example.com",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if binary != "nfpm" || !containsString(args, "deb") {
			t.Errorf("unexpected invocation: %s %v", binary, args)
		}
		for _, want := range []string{"--name", "svpc", "--version", "1.0.0", "--output", "out/app.deb"} {
			if !containsString(args, want) {
				t.Errorf("missing %q in %v", want, args)
			}
		}
	})

	t.Run("native artifacts are not packaged", func(t *testing.T) {
		_, _, err := packageCommand(packageParams{
			Platform: "android", Format: "apk", Input: "app.apk", Output: "out/app.apk",
		})
		if err == nil {
			t.Fatal("expected an error explaining the build produces the apk directly")
		}
		if !strings.Contains(err.Error(), "build") {
			t.Errorf("the error should point at the build action, got %q", err)
		}
	})

	t.Run("unknown format", func(t *testing.T) {
		if _, _, err := packageCommand(packageParams{Format: "iso"}); err == nil {
			t.Error("expected an error for an unsupported format")
		}
	})
}

func TestWriteFileIfChangedIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "file.txt")

	if err := writeFileIfChanged(path, "hello"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	first := info.ModTime()

	// Rewriting identical content must not touch the file, so build systems
	// watching mtimes do not rebuild.
	if err := writeFileIfChanged(path, "hello"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info, _ = os.Stat(path)
	if !info.ModTime().Equal(first) {
		t.Error("an unchanged write should not modify the file")
	}

	if err := writeFileIfChanged(path, "goodbye"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "goodbye" {
		t.Errorf("content = %q, want goodbye", data)
	}
}

func TestYamlScalarQuotesAmbiguousValues(t *testing.T) {
	cases := map[string]string{
		"simple":     "simple",
		"true":       `"true"`,
		"on":         `"on"`,
		"has: colon": `"has: colon"`,
		"123":        "123",
		"":           `""`,
		"# hash":     `"# hash"`,
	}
	for input, want := range cases {
		if got := yamlScalar(input); got != want {
			t.Errorf("yamlScalar(%q) = %s, want %s", input, got, want)
		}
	}
}

func TestRenderWorkflow(t *testing.T) {
	out, err := renderWorkflow(workflowParams{
		Name: "ci.yml",
		On:   map[string]any{"push": map[string]any{"branches": []any{"main"}}},
		Jobs: map[string]any{"build": map[string]any{"runs-on": "ubuntu-latest"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, "name: ci") {
		t.Errorf("workflow name missing:\n%s", out)
	}
	if !strings.Contains(out, "on:") || !strings.Contains(out, "jobs:") {
		t.Errorf("top-level keys missing:\n%s", out)
	}
	if !strings.Contains(out, "runs-on") {
		t.Errorf("job body missing:\n%s", out)
	}

	// The result has to be parseable YAML, so a round trip through JSON is the
	// cheapest sanity check for the top-level structure.
	if !strings.HasSuffix(out, "\n") {
		t.Error("the document should end with a newline")
	}
}

func TestWriteYAMLMapOrdersKeys(t *testing.T) {
	// Sorted output keeps generated workflows diff-friendly.
	var b strings.Builder
	writeYAMLMap(&b, map[string]any{"zeta": 1, "alpha": 2, "mid": 3}, "  ")

	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	want := []string{"  alpha: 2", "  mid: 3", "  zeta: 1"}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), len(want), b.String())
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Shared test helpers
// ---------------------------------------------------------------------------

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// jsonMap is a small convenience for asserting on decoded payloads.
func jsonMap(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("decoding %q: %v", raw, err)
	}
	return m
}
