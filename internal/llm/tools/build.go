package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/svpc-ai/svpc/internal/permission"
)

const (
	BuildToolName    = "build"
	SignToolName     = "sign"
	NotarizeToolName = "notarize"
	PackageToolName  = "package"
)

type buildParams struct {
	Platform    string            `json:"platform"`
	Action      string            `json:"action"`
	ProjectPath string            `json:"project_path"`
	Config      map[string]any    `json:"config"`
	OutputPath  string            `json:"output_path"`
	Version     string            `json:"version"`
	Arch        string            `json:"arch"`
	Target      string            `json:"target"`
	LdFlags     string            `json:"ldflags,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	NoTests     bool              `json:"no_tests,omitempty"`
}

// BuildTool compiles a project for one or more platforms. The framework is
// detected from the files on disk, so the model does not have to guess.
type BuildTool struct {
	runner *runner
}

func NewBuildTool(permissions permission.Service) BaseTool {
	return &BuildTool{runner: newRunner(permissions)}
}

func (t *BuildTool) Info() ToolInfo {
	return ToolInfo{
		Name: BuildToolName,
		Description: "Build a project for a target platform. Platforms: android, windows, linux, " +
			"macos, ios, all. Actions: build, clean, test, lint, doctor. " +
			"The framework (go, flutter, react-native, electron, tauri, rust, node, kotlin, swift) " +
			"is detected automatically unless config.framework says otherwise.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"platform": map[string]any{
					"type": "string",
					"enum": []string{"android", "windows", "linux", "macos", "ios", "all"},
				},
				"action": map[string]any{
					"type": "string",
					"enum": []string{"build", "clean", "test", "lint", "doctor"},
				},
				"project_path": map[string]any{"type": "string", "description": "Project root (default: working directory)"},
				"config":       map[string]any{"type": "object", "description": "Overrides, e.g. {\"framework\":\"go\"}"},
				"version":      map[string]any{"type": "string", "description": "Version string, injected into Go builds"},
				"arch":         map[string]any{"type": "string", "enum": []string{"amd64", "arm64", "386", "arm", "universal", "all"}},
				"ldflags":      map[string]any{"type": "string", "description": "Extra -ldflags for Go builds"},
				"no_tests":     map[string]any{"type": "boolean", "description": "Skip the test step after a build"},
			},
			"required": []string{"platform", "action"},
		},
	}
}

func (t *BuildTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var p buildParams
	if err := json.Unmarshal([]byte(call.Input), &p); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	if strings.TrimSpace(p.Platform) == "" || strings.TrimSpace(p.Action) == "" {
		return NewTextErrorResponse("platform and action are required"), nil
	}

	dir := p.ProjectPath
	if dir == "" {
		dir = t.runner.workingDir
	}

	// A project_path pointing somewhere else is worth its own confirmation.
	sessionID, messageID := sessionContext(ctx)
	if p.ProjectPath != "" {
		if err := t.runner.askIn(ctx, sessionID, messageID, BuildToolName,
			fmt.Sprintf("build %s in %s", p.Platform, p.ProjectPath), p.Action,
			dir, map[string]any{"platform": p.Platform, "path": p.ProjectPath}); err != nil {
			return NewTextErrorResponse(err.Error()), nil
		}
	}

	framework := detectFramework(dir, stringField(p.Config, "framework"))

	platforms := []string{strings.ToLower(p.Platform)}
	if platforms[0] == "all" {
		platforms = []string{"android", "windows", "linux", "macos", "ios"}
	}

	var out strings.Builder
	ran := 0
	var failures int

	for _, platform := range platforms {
		result, err := runBuild(ctx, t.runner, dir, framework, platform, p)
		switch {
		case err != nil:
			fmt.Fprintf(&out, "== %s ==\n%s\n\n", platform, err.Error())
			failures++
		case result == "":
			// Nothing to do for this platform/framework combination.
			continue
		default:
			fmt.Fprintf(&out, "== %s ==\n%s\n\n", platform, result)
			ran++
		}
	}

	if out.Len() == 0 {
		return NewTextResponse(fmt.Sprintf(
			"Nothing to %s: no supported build path for framework %q on %s.",
			p.Action, framework, p.Platform)), nil
	}
	if failures > 0 {
		return NewTextResponse(out.String()), nil
	}

	// Testing after a successful build is what makes a build meaningful.
	if p.Action == "build" && !p.NoTests {
		if cmd := frameworkTest(dir, framework); cmd != nil {
			if res, err := t.runner.execIn(ctx, dir, cmd[0], cmd[1:]...); err == nil {
				out.WriteString("== tests ==\n")
				if res.ExitCode == 0 {
					if s := strings.TrimSpace(res.Stdout); s != "" {
						out.WriteString(s + "\n")
					} else {
						out.WriteString("passed\n")
					}
				} else {
					out.WriteString("failed\n" + res.String() + "\n")
				}
			}
		}
	}

	ran++
	if ran == 0 {
		return NewTextErrorResponse("no build step ran"), nil
	}
	return NewTextResponse(out.String()), nil
}

// runBuild executes the build for one platform.
func runBuild(ctx context.Context, r *runner, dir, framework, platform string, p buildParams) (string, error) {
	action := strings.ToLower(p.Action)
	arch := orDefault(p.Arch, "all")

	switch action {
	case "doctor":
		return doctor(r, dir, framework)
	case "clean":
		cmd := frameworkClean(dir, framework)
		if cmd == nil {
			return "", fmt.Errorf("clean is not defined for %s", framework)
		}
		res, err := r.execIn(ctx, dir, cmd[0], cmd[1:]...)
		return joinResult(res, err), nil
	case "lint":
		cmd := frameworkLint(dir, framework)
		if cmd == nil {
			return "", fmt.Errorf("lint is not defined for %s", framework)
		}
		res, err := r.execIn(ctx, dir, cmd[0], cmd[1:]...)
		return joinResult(res, err), nil
	case "test":
		cmd := frameworkTest(dir, framework)
		if cmd == nil {
			return "", fmt.Errorf("test is not defined for %s", framework)
		}
		res, err := r.execIn(ctx, dir, cmd[0], cmd[1:]...)
		return joinResult(res, err), nil
	case "build":
		// fall through
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}

	steps, err := buildSteps(dir, framework, platform, arch, p.Version, p.LdFlags)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	for _, step := range steps {
		res, err := r.execIn(ctx, dir, step[0], step[1:]...)
		if err != nil {
			return out.String(), err
		}
		fmt.Fprintf(&out, "$ %s %s\n", step[0], strings.Join(step[1:], " "))
		if s := strings.TrimSpace(res.Stdout); s != "" {
			out.WriteString(s + "\n")
		}
		if s := strings.TrimSpace(res.Stderr); s != "" {
			out.WriteString(s + "\n")
		}
		if res.ExitCode != 0 {
			return out.String(), fmt.Errorf("build failed with exit code %d", res.ExitCode)
		}
	}
	return out.String(), nil
}

func joinResult(res commandResult, err error) string {
	if err != nil {
		return err.Error()
	}
	return res.String()
}

// doctor reports whether the toolchain for a framework is present.
func doctor(r *runner, dir, framework string) (string, error) {
	checks := map[string][]string{
		"go":           {"go", "version"},
		"flutter":      {"flutter", "--version"},
		"rust":         {"cargo", "--version"},
		"node":         {"node", "--version"},
		"electron":     {"npm", "--version"},
		"tauri":        {"cargo", "tauri", "--version"},
		"kotlin":       {"java", "-version"},
		"android":      {"gradle", "--version"},
		"swift":        {"swift", "--version"},
		"react-native": {"node", "--version"},
	}

	var out strings.Builder
	fmt.Fprintf(&out, "framework: %s\nproject:   %s\n\ntoolchain:\n", framework, dir)

	cmds, ok := checks[framework]
	if !ok {
		cmds = []string{"go", "version"}
		fmt.Fprintf(&out, "  (no known toolchain for %s, checking go)\n", framework)
	}

	if _, err := lookPath(cmds[0]); err != nil {
		fmt.Fprintf(&out, "  %-8s %s\n", cmds[0], "not installed")
		return out.String(), nil
	}
	res, err := r.execIn(context.Background(), dir, cmds[0], cmds[1:]...)
	if err != nil {
		fmt.Fprintf(&out, "  %-8s error: %v\n", cmds[0], err)
		return out.String(), nil
	}
	version := strings.TrimSpace(firstNonEmpty(res.Stdout, res.Stderr))
	version = strings.SplitN(version, "\n", 2)[0]
	fmt.Fprintf(&out, "  %-8s %s\n", cmds[0], version)
	return out.String(), nil
}

// ---------------------------------------------------------------------------
// Framework detection
// ---------------------------------------------------------------------------

// detectFramework infers the build system from the project files.
func detectFramework(dir, override string) string {
	if override != "" {
		return strings.ToLower(override)
	}

	if fileExists(dir, "go.mod") {
		return "go"
	}
	if fileExists(dir, "pubspec.yaml") {
		return "flutter"
	}
	if fileExists(dir, "Cargo.toml") {
		if fileExists(dir, "src-tauri") || fileExists(dir, "tauri.conf.json") {
			return "tauri"
		}
		return "rust"
	}
	if fileExists(dir, "package.json") {
		if hasDependency(dir, "electron") {
			return "electron"
		}
		if hasDependency(dir, "@tauri-apps/cli") {
			return "tauri"
		}
		if hasDependency(dir, "react-native") {
			return "react-native"
		}
		if hasDependency(dir, "expo") {
			return "expo"
		}
		return "node"
	}
	if fileExists(dir, "build.gradle") || fileExists(dir, "build.gradle.kts") || fileExists(dir, "settings.gradle") {
		return "kotlin"
	}
	if fileExists(dir, "Package.swift") {
		return "swift"
	}
	if fileExists(dir, "ios") && fileExists(dir, "android") {
		return "flutter"
	}
	return "unknown"
}

func frameworkClean(dir, framework string) []string {
	switch framework {
	case "go":
		return []string{"go", "clean", "-cache"}
	case "flutter":
		return []string{"flutter", "clean"}
	case "rust", "tauri":
		return []string{"cargo", "clean"}
	case "kotlin":
		return []string{gradlew(dir), "clean"}
	case "swift":
		return []string{"xcodebuild", "clean"}
	default:
		return nil
	}
}

func frameworkTest(dir, framework string) []string {
	switch framework {
	case "go":
		return []string{"go", "test", "./..."}
	case "flutter":
		return []string{"flutter", "test"}
	case "rust", "tauri":
		return []string{"cargo", "test"}
	case "node", "electron", "react-native", "expo":
		return []string{"npm", "test"}
	case "kotlin":
		return []string{gradlew(dir), "test"}
	case "swift":
		return []string{"xcodebuild", "test"}
	default:
		return nil
	}
}

func frameworkLint(dir, framework string) []string {
	switch framework {
	case "go":
		return []string{"go", "vet", "./..."}
	case "flutter":
		return []string{"flutter", "analyze"}
	case "rust", "tauri":
		return []string{"cargo", "clippy"}
	case "node", "electron", "react-native", "expo":
		return []string{"npm", "run", "lint"}
	case "kotlin":
		return []string{gradlew(dir), "lint"}
	case "swift":
		return []string{"swiftlint"}
	default:
		return nil
	}
}

// gradlew returns the wrapper when the project ships one, so a Gradle build
// uses the pinned version instead of whatever is on PATH.
func gradlew(dir string) string {
	if fileExists(dir, "gradlew") || fileExists(dir, "gradlew.bat") {
		return "./gradlew"
	}
	return "gradle"
}

// ---------------------------------------------------------------------------
// Platform build steps
// ---------------------------------------------------------------------------

// buildSteps returns the commands that produce a build for one platform.
func buildSteps(dir, framework, platform, arch, version, extraLdflags string) ([][]string, error) {
	switch framework {
	case "go":
		return goBuildSteps(platform, arch, version, extraLdflags), nil

	case "flutter":
		switch platform {
		case "android":
			return [][]string{{"flutter", "build", "apk", "--release"}}, nil
		case "ios":
			return [][]string{{"flutter", "build", "ios", "--release", "--no-codesign"}}, nil
		case "windows":
			return [][]string{{"flutter", "build", "windows", "--release"}}, nil
		case "macos":
			return [][]string{{"flutter", "build", "macos", "--release"}}, nil
		case "linux":
			return [][]string{{"flutter", "build", "linux", "--release"}}, nil
		}

	case "kotlin":
		if platform == "android" {
			return [][]string{{gradlew(dir), "assembleRelease"}}, nil
		}

	case "swift":
		switch platform {
		case "ios":
			return [][]string{{"xcodebuild", "-scheme", "App", "-configuration", "Release",
				"-sdk", "iphoneos", "-derivedDataPath", "build"}}, nil
		case "macos":
			return [][]string{{"xcodebuild", "-scheme", "App", "-configuration", "Release",
				"-derivedDataPath", "build"}}, nil
		}

	case "react-native", "expo":
		if platform == "android" {
			return [][]string{{gradlew(dir), "assembleRelease", "-p", "android"}}, nil
		}
		if platform == "ios" {
			return [][]string{{"xcodebuild", "-workspace", "ios/App.xcworkspace",
				"-scheme", "App", "-configuration", "Release",
				"-sdk", "iphoneos", "-derivedDataPath", "build"}}, nil
		}

	case "electron", "node", "tauri":
		return npmBuildSteps(dir, framework, platform)

	case "rust":
		return rustBuildSteps(platform, arch), nil
	}

	return nil, nil
}

// goBuildSteps compiles for the requested targets. A cross compile needs no
// extra toolchain for pure Go, which makes this the most useful case.
func goBuildSteps(platform, arch, version, extraLdflags string) [][]string {
	flags := extraLdflags
	if version != "" {
		v := "-X main.version=" + version
		if flags == "" {
			flags = v
		} else {
			flags = flags + " " + v
		}
	}

	targets := goTargets(platform, arch)
	steps := make([][]string, 0, len(targets))
	for _, t := range targets {
		// The target is "<goos>-<goarch>" so the output path cannot collide
		// between operating systems.
		out := "dist/" + t + "/" + goBinaryName(t, platform)
		step := []string{"go", "build"}
		if flags != "" {
			step = append(step, "-ldflags", flags)
		}
		// A GUI binary should not open a console window on Windows.
		if platform == "windows" {
			step = append(step, "-H=windowsgui")
		}
		steps = append(steps, append(step, "-o", out, "."))
	}
	return steps
}

// goTargets maps a platform onto the Go toolchain targets that satisfy it.
// Each entry is "<goos>-<goarch>", which is also the name used for the output
// directory, so builds for different platforms never overwrite each other.
func goTargets(platform, arch string) []string {
	all := map[string][]string{
		"linux":   {"amd64", "arm64", "386", "arm"},
		"windows": {"amd64", "arm64"},
		"darwin":  {"amd64", "arm64"},
		"android": {"arm64", "amd64"},
		"ios":     {"arm64"},
	}
	list, ok := all[platform]
	if !ok {
		return nil
	}
	if arch != "" && arch != "all" && arch != "universal" {
		// iOS has only ever shipped arm64, so anything else is unsatisfiable.
		if platform == "ios" && arch != "arm64" {
			return nil
		}
		return []string{platform + "-" + arch}
	}

	targets := make([]string, 0, len(list))
	for _, a := range list {
		targets = append(targets, platform+"-"+a)
	}
	return targets
}

func goBinaryName(target, platform string) string {
	name := "svpc"
	if platform == "windows" {
		return name + ".exe"
	}
	return name
}

func rustBuildSteps(platform, arch string) [][]string {
	var targets []string
	switch platform {
	case "linux":
		if arch == "" || arch == "all" {
			targets = []string{"x86_64-unknown-linux-gnu", "aarch64-unknown-linux-gnu"}
		} else {
			targets = []string{arch + "-unknown-linux-gnu"}
		}
	case "windows":
		if arch == "" || arch == "all" {
			targets = []string{"x86_64-pc-windows-msvc", "aarch64-pc-windows-msvc"}
		} else {
			targets = []string{arch + "-pc-windows-msvc"}
		}
	case "macos":
		if arch == "" || arch == "all" || arch == "universal" {
			targets = []string{"x86_64-apple-darwin", "aarch64-apple-darwin"}
		} else {
			targets = []string{arch + "-apple-darwin"}
		}
	default:
		return nil
	}

	steps := make([][]string, 0, len(targets))
	for _, t := range targets {
		steps = append(steps, []string{"cargo", "build", "--release", "--target", t})
	}
	return steps
}

// npmBuildSteps picks the per-platform build script the project defines,
// falling back to the generic one when it is not present.
func npmBuildSteps(dir, framework, platform string) ([][]string, error) {
	if !fileExists(dir, "package.json") {
		return nil, fmt.Errorf("no package.json in %s", dir)
	}

	script := "build"
	candidates := map[string][]string{
		"windows": {"build:win", "build:windows"},
		"macos":   {"build:mac", "build:macos"},
		"linux":   {"build:linux"},
	}
	for _, name := range candidates[platform] {
		if npmScriptExists(dir, name) {
			script = name
			break
		}
	}

	if platform == "android" || platform == "ios" {
		return nil, fmt.Errorf("%s mobile builds are handled by the native toolchain, not npm", framework)
	}
	return [][]string{{"npm", "run", script}}, nil
}
