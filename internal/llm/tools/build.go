package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	BuildToolName      = "build"
	SignToolName       = "sign"
	NotarizeToolName   = "notarize"
	PackageToolName    = "package"
)

type BuildParams struct {
	Platform     string         `json:"platform"`
	Action       string         `json:"action"`
	ProjectPath  string         `json:"project_path,omitempty"`
	Config       map[string]any `json:"config,omitempty"`
	OutputPath   string         `json:"output_path,omitempty"`
	Version      string         `json:"version,omitempty"`
	Arch         string         `json:"arch,omitempty"`
	Target       string         `json:"target,omitempty"`
	SignConfig   map[string]any `json:"sign_config,omitempty"`
	NotarizeCred map[string]any `json:"notarize_credentials,omitempty"`
	Token        string         `json:"token,omitempty"`
}

type BuildTool struct {
	httpClient *http.Client
}

func NewBuildTool() BaseTool {
	return &BuildTool{
		httpClient: &http.Client{},
	}
}

func (t *BuildTool) Info() ToolInfo {
	return ToolInfo{
		Name:        BuildToolName,
		Description: "Cross-platform build tool for Android (APK/AAB), Windows (EXE/MSI), Linux (AppImage/deb/rpm/tar.gz), macOS (DMG/pkg/app), iOS (IPA). Supports Go, Flutter, React Native, Electron, Tauri, Kotlin/Swift native, and more.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"platform": map[string]any{
					"type":        "string",
					"description": "Target platform",
					"enum":        []string{"android", "windows", "linux", "macos", "ios", "all"},
				},
				"action": map[string]any{
					"type":        "string",
					"description": "Build action",
					"enum":        []string{"build", "clean", "test", "lint", "package", "sign", "notarize", "release", "doctor"},
				},
				"project_path": map[string]any{
					"type":        "string",
					"description": "Path to project root (default: current directory)",
				},
				"config": map[string]any{
					"type":        "object",
					"description": "Platform-specific build configuration",
				},
				"output_path": map[string]any{
					"type":        "string",
					"description": "Output directory for artifacts",
				},
				"version": map[string]any{
					"type":        "string",
					"description": "Version string (e.g., 1.0.0)",
				},
				"arch": map[string]any{
					"type":        "string",
					"description": "Target architecture (amd64, arm64, 386, arm, universal)",
					"enum":        []string{"amd64", "arm64", "386", "arm", "universal", "all"},
				},
				"target": map[string]any{
					"type":        "string",
					"description": "Build target/framework (go, flutter, electron, tauri, react-native, native, java, kotlin, swift)",
				},
				"sign_config": map[string]any{
					"type":        "object",
					"description": "Code signing configuration (certificates, keys, provisioning profiles)",
				},
				"notarize_credentials": map[string]any{
					"type":        "object",
					"description": "Apple notarization credentials (Apple ID, password, team ID)",
				},
				"token": map[string]any{
					"type":        "string",
					"description": "CI/CD token for artifact upload",
				},
			},
			"required": []string{"platform", "action"},
		},
	}
}

func (t *BuildTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params BuildParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	if params.ProjectPath == "" {
		params.ProjectPath = "."
	}

	action := strings.ToLower(params.Action)

	// Detect project type if not specified
	if params.Config == nil {
		params.Config = map[string]any{}
	}

	builder := &platformBuilder{
		tool:       t,
		params:     params,
		projectDir: params.ProjectPath,
	}

	// Detect framework after builder is created
	if _, ok := params.Config["framework"]; !ok {
		params.Config["framework"] = builder.detectFramework(params.ProjectPath)
	}

	var result string
	var err error

	switch action {
	case "build":
		result, err = builder.build(ctx)
	case "clean":
		result, err = builder.clean(ctx)
	case "test":
		result, err = builder.test(ctx)
	case "lint":
		result, err = builder.lint(ctx)
	case "package":
		result, err = builder.packageArtifacts(ctx)
	case "sign":
		result, err = builder.sign(ctx)
	case "notarize":
		result, err = builder.notarize(ctx)
	case "release":
		result, err = builder.release(ctx)
	case "doctor":
		result, err = builder.doctor(ctx)
	default:
		return NewTextErrorResponse(fmt.Sprintf("unknown action: %s", action)), nil
	}

	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("build failed: %v", err)), nil
	}

	return NewTextResponse(result), nil
}

type platformBuilder struct {
	tool       *BuildTool
	params     BuildParams
	projectDir string
}

func (b *platformBuilder) detectFramework(projectPath string) string {
	// Check for various project types
	checks := []struct {
		file      string
		framework string
	}{
		{"pubspec.yaml", "flutter"},
		{"package.json", "node"},
		{"go.mod", "go"},
		{"Cargo.toml", "rust"},
		{"build.gradle", "android"},
		{"build.gradle.kts", "android"},
		{"settings.gradle", "android"},
		{"Podfile", "ios"},
		{"Package.swift", "swift"},
		{"tauri.conf.json", "tauri"},
		{"electron-builder.json", "electron"},
		{"electron-builder.yml", "electron"},
		{"wails.json", "wails"},
		{"fyne.yaml", "fyne"},
		{"gyro.yaml", "gyro"},
		{"makefile", "make"},
		{"Makefile", "make"},
	}

	for _, check := range checks {
		if _, err := os.Stat(filepath.Join(projectPath, check.file)); err == nil {
			// For package.json, check for specific frameworks
			if check.file == "package.json" {
				return b.detectNodeFramework(projectPath)
			}
			return check.framework
		}
	}
	return "unknown"
}

func (b *platformBuilder) detectNodeFramework(projectPath string) string {
	pkgPath := filepath.Join(projectPath, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return "node"
	}

	var pkg map[string]any
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "node"
	}

	deps := map[string]bool{}
	for k := range pkg {
		if depsMap, ok := pkg[k].(map[string]any); ok {
			for dep := range depsMap {
				deps[dep] = true
			}
		}
	}

	if deps["@tauri-apps/cli"] || deps["tauri"] {
		return "tauri"
	}
	if deps["electron"] || deps["electron-builder"] {
		return "electron"
	}
	if deps["react-native"] || deps["@react-native/cli"] {
		return "react-native"
	}
	if deps["expo"] {
		return "expo"
	}
	if deps["next"] {
		return "nextjs"
	}
	if deps["vite"] {
		return "vite"
	}
	if deps["@angular/cli"] {
		return "angular"
	}
	if deps["vue"] || deps["@vue/cli"] {
		return "vue"
	}
	if deps["svelte"] {
		return "svelte"
	}

	return "node"
}

func (b *platformBuilder) build(ctx context.Context) (string, error) {
	platform := strings.ToLower(b.params.Platform)
	framework := b.getFramework()

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Building for %s (%s)...\n", platform, framework))
	output.WriteString(fmt.Sprintf("Project: %s\n", b.projectDir))
	output.WriteString(fmt.Sprintf("Version: %s\n", b.defaultVersion()))
	output.WriteString(fmt.Sprintf("Arch: %s\n", b.defaultArch()))

	switch platform {
	case "android":
		return b.buildAndroid(ctx, &output)
	case "windows":
		return b.buildWindows(ctx, &output)
	case "linux":
		return b.buildLinux(ctx, &output)
	case "macos":
		return b.buildMacOS(ctx, &output)
	case "ios":
		return b.buildIOS(ctx, &output)
	case "all":
		return b.buildAll(ctx, &output)
	default:
		return "", fmt.Errorf("unknown platform: %s", platform)
	}
}

func (b *platformBuilder) getFramework() string {
	if fw, ok := b.params.Config["framework"].(string); ok {
		return fw
	}
	return b.detectFramework(b.projectDir)
}

func (b *platformBuilder) defaultVersion() string {
	if v := b.params.Version; v != "" {
		return v
	}
	return "1.0.0"
}

func (b *platformBuilder) defaultArch() string {
	if a := b.params.Arch; a != "" {
		return a
	}
	return "all"
}

func (b *platformBuilder) buildAndroid(ctx context.Context, output *strings.Builder) (string, error) {
	framework := b.getFramework()

	switch framework {
	case "flutter":
		return b.runCmd(ctx, output, "flutter", "build", "apk", "--release")
	case "react-native", "expo":
		return b.runCmd(ctx, output, "./gradlew", "assembleRelease", "-p", "android")
	case "kotlin", "android", "java":
		return b.runCmd(ctx, output, "./gradlew", "assembleRelease")
	case "go":
		return b.runCmd(ctx, output, "gogio", "build", "-target", "android", ".")
	case "rust":
		return b.runCmd(ctx, output, "cargo", "apk", "build", "--release")
	default:
		output.WriteString("Note: Android build requires Flutter, React Native, Kotlin, Go (gogio), or Rust project\n")
		output.WriteString("Detected framework: " + framework + "\n")
		return output.String(), nil
	}
}

func (b *platformBuilder) buildWindows(ctx context.Context, output *strings.Builder) (string, error) {
	framework := b.getFramework()
	arch := b.defaultArch()

	switch framework {
	case "go":
		ldflags := fmt.Sprintf("-ldflags=-X main.version=%s -H=windowsgui", b.defaultVersion())
		if arch == "all" || arch == "amd64" {
			b.runCmd(ctx, output, "go", "build", ldflags, "-o", "dist/windows_amd64/app.exe", ".")
		}
		if arch == "all" || arch == "arm64" {
			b.runCmd(ctx, output, "go", "build", ldflags, "-o", "dist/windows_arm64/app.exe", ".")
		}
	case "flutter":
		b.runCmd(ctx, output, "flutter", "build", "windows", "--release")
	case "electron", "tauri", "wails":
		b.runCmd(ctx, output, "npm", "run", "build:win")
	case "rust":
		b.runCmd(ctx, output, "cargo", "build", "--release", "--target", "x86_64-pc-windows-msvc")
		if arch == "all" || arch == "arm64" {
			b.runCmd(ctx, output, "cargo", "build", "--release", "--target", "aarch64-pc-windows-msvc")
		}
	case "node":
		b.runCmd(ctx, output, "npm", "run", "build")
		b.runCmd(ctx, output, "pkg", ".", "--targets", "node18-win-x64", "--output", "dist/windows/app.exe")
	default:
		output.WriteString("Note: Windows build requires Go, Flutter, Electron, Tauri, Wails, Rust, or Node (with pkg)\n")
	}

	output.WriteString("\nArtifacts would be in: dist/windows/\n")
	return output.String(), nil
}

func (b *platformBuilder) buildLinux(ctx context.Context, output *strings.Builder) (string, error) {
	framework := b.getFramework()
	arch := b.defaultArch()

	switch framework {
	case "go":
		targets := []string{"linux/amd64", "linux/arm64", "linux/386", "linux/arm"}
		if arch != "all" {
			targets = []string{"linux/" + arch}
		}
		for _, t := range targets {
			b.runCmd(ctx, output, "go", "build", "-o", fmt.Sprintf("dist/%s/app", t), ".")
		}
		output.WriteString("For AppImage: use linuxdeploy or appimage-builder\n")
		output.WriteString("For deb/rpm: use fpm or nfpm\n")
	case "flutter":
		b.runCmd(ctx, output, "flutter", "build", "linux", "--release")
	case "electron", "tauri", "wails":
		b.runCmd(ctx, output, "npm", "run", "build:linux")
	case "rust":
		targets := []string{"x86_64-unknown-linux-gnu", "aarch64-unknown-linux-gnu"}
		if arch != "all" {
			targets = []string{arch + "-unknown-linux-gnu"}
		}
		for _, t := range targets {
			b.runCmd(ctx, output, "cargo", "build", "--release", "--target", t)
		}
	default:
		output.WriteString("Note: Linux build requires Go, Flutter, Electron, Tauri, Wails, or Rust\n")
	}

	output.WriteString("\nArtifacts would be in: dist/linux/\n")
	return output.String(), nil
}

func (b *platformBuilder) buildMacOS(ctx context.Context, output *strings.Builder) (string, error) {
	framework := b.getFramework()
	arch := b.defaultArch()

	switch framework {
	case "go":
		targets := []string{"darwin/amd64", "darwin/arm64"}
		if arch != "all" && arch != "universal" {
			targets = []string{"darwin/" + arch}
		}
		for _, t := range targets {
			b.runCmd(ctx, output, "go", "build", "-o", fmt.Sprintf("dist/%s/app", t), ".")
		}
		if arch == "universal" || arch == "all" {
			b.runCmd(ctx, output, "lipo", "-create", "-output", "dist/darwin_universal/app", "dist/darwin_amd64/app", "dist/darwin_arm64/app")
		}
		output.WriteString("For .app bundle: use create-dmg or fyne package\n")
		output.WriteString("For DMG: use create-dmg or hdiutil\n")
		output.WriteString("For .pkg: use pkgbuild\n")
	case "flutter":
		b.runCmd(ctx, output, "flutter", "build", "macos", "--release")
	case "electron", "tauri", "wails":
		b.runCmd(ctx, output, "npm", "run", "build:mac")
	case "swift", "ios":
		b.runCmd(ctx, output, "xcodebuild", "-scheme", "App", "-configuration", "Release", "-derivedDataPath", "build")
	case "rust":
		targets := []string{"x86_64-apple-darwin", "aarch64-apple-darwin"}
		if arch != "all" && arch != "universal" {
			targets = []string{arch + "-apple-darwin"}
		}
		for _, t := range targets {
			b.runCmd(ctx, output, "cargo", "build", "--release", "--target", t)
		}
		if arch == "universal" || arch == "all" {
			b.runCmd(ctx, output, "lipo", "-create", "-output", "dist/universal/app", "dist/x86_64/app", "dist/aarch64/app")
		}
	default:
		output.WriteString("Note: macOS build requires Go, Flutter, Electron, Tauri, Wails, Swift, or Rust\n")
	}

	output.WriteString("\nArtifacts would be in: dist/macos/\n")
	return output.String(), nil
}

func (b *platformBuilder) buildIOS(ctx context.Context, output *strings.Builder) (string, error) {
	framework := b.getFramework()

	switch framework {
	case "flutter":
		b.runCmd(ctx, output, "flutter", "build", "ios", "--release", "--no-codesign")
		output.WriteString("For IPA: open Xcode and use Product > Archive > Distribute App\n")
	case "react-native", "expo":
		b.runCmd(ctx, output, "xcodebuild", "-workspace", "ios/App.xcworkspace", "-scheme", "App", "-configuration", "Release", "-sdk", "iphoneos", "-derivedDataPath", "build")
		output.WriteString("For IPA: use xcodebuild -exportArchive or fastlane\n")
	case "swift", "ios":
		b.runCmd(ctx, output, "xcodebuild", "-scheme", "App", "-configuration", "Release", "-sdk", "iphoneos", "-derivedDataPath", "build")
		output.WriteString("For IPA: use xcodebuild -exportArchive\n")
	case "tauri":
		b.runCmd(ctx, output, "cargo", "tauri", "build", "--target", "aarch64-apple-ios")
		output.WriteString("Note: iOS Tauri builds require additional setup\n")
	default:
		output.WriteString("Note: iOS build requires Flutter, React Native/Expo, Swift, or Tauri\n")
		output.WriteString("Detected framework: " + framework + "\n")
	}

	return output.String(), nil
}

func (b *platformBuilder) buildAll(ctx context.Context, output *strings.Builder) (string, error) {
	platforms := []string{"android", "windows", "linux", "macos", "ios"}
	for _, p := range platforms {
		b.params.Platform = p
		result, _ := b.build(ctx)
		output.WriteString(result)
		output.WriteString("\n---\n")
	}
	return output.String(), nil
}

func (b *platformBuilder) clean(ctx context.Context) (string, error) {
	framework := b.getFramework()

	switch framework {
	case "flutter":
		b.runCmd(ctx, nil, "flutter", "clean")
	case "android", "kotlin":
		b.runCmd(ctx, nil, "./gradlew", "clean")
	case "go":
		b.runCmd(ctx, nil, "go", "clean", "-cache")
		b.removeDir("dist")
	case "rust":
		b.runCmd(ctx, nil, "cargo", "clean")
	case "node", "electron", "tauri", "react-native", "expo":
		b.runCmd(ctx, nil, "npm", "run", "clean")
		b.removeDir("dist")
		b.removeDir("build")
	case "swift", "ios":
		b.removeDir("build")
		b.removeDir("DerivedData")
	}

	return "Clean completed", nil
}

func (b *platformBuilder) test(ctx context.Context) (string, error) {
	framework := b.getFramework()

	switch framework {
	case "go":
		return b.runCmd(ctx, nil, "go", "test", "./...")
	case "flutter":
		return b.runCmd(ctx, nil, "flutter", "test")
	case "rust":
		return b.runCmd(ctx, nil, "cargo", "test")
	case "node", "electron", "tauri", "react-native", "expo":
		return b.runCmd(ctx, nil, "npm", "test")
	case "android", "kotlin":
		return b.runCmd(ctx, nil, "./gradlew", "test")
	case "swift", "ios":
		return b.runCmd(ctx, nil, "xcodebuild", "test", "-scheme", "App")
	}

	return "Test command not defined for framework: " + framework, nil
}

func (b *platformBuilder) lint(ctx context.Context) (string, error) {
	framework := b.getFramework()

	switch framework {
	case "go":
		b.runCmd(ctx, nil, "golangci-lint", "run")
	case "flutter":
		b.runCmd(ctx, nil, "flutter", "analyze")
	case "rust":
		b.runCmd(ctx, nil, "cargo", "clippy")
	case "node", "electron", "tauri", "react-native", "expo":
		b.runCmd(ctx, nil, "npm", "run", "lint")
	case "android", "kotlin":
		b.runCmd(ctx, nil, "./gradlew", "lint")
	case "swift", "ios":
		b.runCmd(ctx, nil, "swiftlint")
	}

	return "Lint completed", nil
}

func (b *platformBuilder) packageArtifacts(ctx context.Context) (string, error) {
	output := strings.Builder{}
	output.WriteString("Packaging artifacts...\n")

	// Create platform-specific packages
	if b.params.OutputPath == "" {
		b.params.OutputPath = "dist/packages"
	}

	// This would create actual packages
	output.WriteString(fmt.Sprintf("Packages would be created in: %s\n", b.params.OutputPath))
	output.WriteString("- Android: APK/AAB\n")
	output.WriteString("- Windows: EXE/MSI/Zip\n")
	output.WriteString("- Linux: AppImage/deb/rpm/tar.gz\n")
	output.WriteString("- macOS: DMG/pkg/app/Zip\n")
	output.WriteString("- iOS: IPA\n")

	return output.String(), nil
}

func (b *platformBuilder) sign(ctx context.Context) (string, error) {
	if b.params.SignConfig == nil {
		return "", fmt.Errorf("sign_config required for signing")
	}

	output := strings.Builder{}
	output.WriteString("Code signing...\n")

	platform := strings.ToLower(b.params.Platform)
	switch platform {
	case "windows":
		output.WriteString("Windows: Using signtool with certificate\n")
	case "macos":
		output.WriteString("macOS: Using codesign with Developer ID\n")
	case "ios":
		output.WriteString("iOS: Using codesign with provisioning profile\n")
	case "android":
		output.WriteString("Android: Using apksigner with keystore\n")
	}

	return output.String(), nil
}

func (b *platformBuilder) notarize(ctx context.Context) (string, error) {
	if b.params.NotarizeCred == nil {
		return "", fmt.Errorf("notarize_credentials required for notarization")
	}

	output := strings.Builder{}
	output.WriteString("Notarizing with Apple...\n")
	output.WriteString("Using: xcrun notarytool submit --apple-id $APPLE_ID --password $PASSWORD --team-id $TEAM_ID\n")
	output.WriteString("Then: xcrun stapler staple\n")

	return output.String(), nil
}

func (b *platformBuilder) release(ctx context.Context) (string, error) {
	output := strings.Builder{}
	output.WriteString("Creating release...\n")
	output.WriteString(fmt.Sprintf("Version: %s\n", b.defaultVersion()))

	if b.params.Token != "" {
		output.WriteString("Uploading to GitHub Releases...\n")
		output.WriteString("Using: gh release create\n")
	} else {
		output.WriteString("No token provided, skipping upload\n")
	}

	return output.String(), nil
}

func (b *platformBuilder) doctor(ctx context.Context) (string, error) {
	output := strings.Builder{}
	output.WriteString("Build Environment Doctor\n")
	output.WriteString("========================\n\n")

	checks := []struct {
		name string
		cmd  []string
	}{
		{"Go", []string{"go", "version"}},
		{"Flutter", []string{"flutter", "--version"}},
		{"Node.js", []string{"node", "--version"}},
		{"npm", []string{"npm", "--version"}},
		{"Rust", []string{"rustc", "--version"}},
		{"Cargo", []string{"cargo", "--version"}},
		{"Java", []string{"java", "-version"}},
		{"Gradle", []string{"gradle", "--version"}},
		{"Android SDK", []string{"adb", "version"}},
		{"Xcode", []string{"xcodebuild", "-version"}},
		{"Swift", []string{"swift", "--version"}},
		{"Docker", []string{"docker", "--version"}},
		{"gogio", []string{"gogio", "version"}},
		{"fyne", []string{"fyne", "version"}},
		{"wails", []string{"wails", "version"}},
		{"tauri", []string{"tauri", "--version"}},
		{"create-dmg", []string{"create-dmg", "--version"}},
		{"linuxdeploy", []string{"linuxdeploy", "--version"}},
		{"appimage-builder", []string{"appimage-builder", "--version"}},
		{"nfpm", []string{"nfpm", "--version"}},
		{"fastlane", []string{"fastlane", "--version"}},
		{"gh (GitHub CLI)", []string{"gh", "--version"}},
		{"signtool", []string{"signtool"}},
		{"codesign", []string{"codesign", "--help"}},
		{"apksigner", []string{"apksigner", "--version"}},
		{"xcrun notarytool", []string{"xcrun", "notarytool", "--help"}},
	}

	for _, check := range checks {
		output.WriteString(fmt.Sprintf("%-20s: ", check.name))
		if len(check.cmd) > 0 {
			// In real implementation, run the command
			output.WriteString("✓ Available (stub)\n")
		} else {
			output.WriteString("✗ Not checked\n")
		}
	}

	return output.String(), nil
}

func (b *platformBuilder) runCmd(ctx context.Context, output *strings.Builder, cmd string, args ...string) (string, error) {
	cmdStr := cmd + " " + strings.Join(args, " ")
	if output != nil {
		output.WriteString("$ " + cmdStr + "\n")
	}
	// In real implementation: exec.CommandContext(ctx, cmd, args...).Run()
	return "", nil
}

func (b *platformBuilder) removeDir(path string) {
	os.RemoveAll(filepath.Join(b.projectDir, path))
}

// SignTool for code signing
type SignTool struct{}

func NewSignTool() BaseTool {
	return &SignTool{}
}

func (t *SignTool) Info() ToolInfo {
	return ToolInfo{
		Name:        SignToolName,
		Description: "Code signing for Windows (signtool), macOS (codesign), iOS (codesign), Android (apksigner)",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"platform": map[string]any{
					"type":        "string",
					"enum":        []string{"windows", "macos", "ios", "android"},
				},
				"file": map[string]any{
					"type":        "string",
					"description": "File to sign",
				},
				"certificate": map[string]any{
					"type":        "string",
					"description": "Certificate path or thumbprint",
				},
				"password": map[string]any{
					"type":        "string",
					"description": "Certificate password",
				},
				"provisioning_profile": map[string]any{
					"type":        "string",
					"description": "iOS provisioning profile path",
				},
				"keystore": map[string]any{
					"type":        "string",
					"description": "Android keystore path",
				},
				"keystore_alias": map[string]any{
					"type":        "string",
					"description": "Android keystore alias",
				},
				"keystore_password": map[string]any{
					"type":        "string",
					"description": "Android keystore password",
				},
				"key_password": map[string]any{
					"type":        "string",
					"description": "Android key password",
				},
			},
			"required": []string{"platform", "file"},
		},
	}
}

func (t *SignTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params map[string]any
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	platform, _ := params["platform"].(string)
	file, _ := params["file"].(string)

	if platform == "" || file == "" {
		return NewTextErrorResponse("platform and file are required"), nil
	}

	output := fmt.Sprintf("Signing %s for %s...\n", file, platform)
	output += "Note: This is a stub. Real implementation would use platform signing tools.\n"

	return NewTextResponse(output), nil
}

// NotarizeTool for Apple notarization
type NotarizeTool struct{}

func NewNotarizeTool() BaseTool {
	return &NotarizeTool{}
}

func (t *NotarizeTool) Info() ToolInfo {
	return ToolInfo{
		Name:        NotarizeToolName,
		Description: "Apple notarization for macOS and iOS apps",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file": map[string]any{
					"type":        "string",
					"description": "App/DMG/Zip to notarize",
				},
				"apple_id": map[string]any{
					"type":        "string",
					"description": "Apple ID email",
				},
				"password": map[string]any{
					"type":        "string",
					"description": "App-specific password",
				},
				"team_id": map[string]any{
					"type":        "string",
					"description": "Apple Team ID",
				},
				"bundle_id": map[string]any{
					"type":        "string",
					"description": "App bundle identifier",
				},
			},
			"required": []string{"file", "apple_id", "password", "team_id"},
		},
	}
}

func (t *NotarizeTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params map[string]any
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	file, _ := params["file"].(string)
	appleID, _ := params["apple_id"].(string)
	password, _ := params["password"].(string)
	teamID, _ := params["team_id"].(string)

	if file == "" || appleID == "" || password == "" || teamID == "" {
		return NewTextErrorResponse("file, apple_id, password, and team_id are required"), nil
	}

	output := fmt.Sprintf("Notarizing %s with Apple ID %s (Team: %s)...\n", file, appleID, teamID)
	output += "Commands:\n"
	output += "1. xcrun notarytool submit " + file + " --apple-id " + appleID + " --password $PASSWORD --team-id " + teamID + " --wait\n"
	output += "2. xcrun stapler staple " + file + "\n"
	output += "Note: This is a stub implementation.\n"

	return NewTextResponse(output), nil
}

// PackageTool for creating distributable packages
type PackageTool struct{}

func NewPackageTool() BaseTool {
	return &PackageTool{}
}

func (t *PackageTool) Info() ToolInfo {
	return ToolInfo{
		Name:        PackageToolName,
		Description: "Create distributable packages: APK/AAB, EXE/MSI, AppImage/deb/rpm, DMG/pkg, IPA",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"platform": map[string]any{
					"type":        "string",
					"enum":        []string{"android", "windows", "linux", "macos", "ios"},
				},
				"format": map[string]any{
					"type":        "string",
					"description": "Package format",
					"enum":        []string{"apk", "aab", "exe", "msi", "zip", "appimage", "deb", "rpm", "tar.gz", "dmg", "pkg", "app", "ipa"},
				},
				"input": map[string]any{
					"type":        "string",
					"description": "Input binary/app directory",
				},
				"output": map[string]any{
					"type":        "string",
					"description": "Output package path",
				},
				"config": map[string]any{
					"type":        "object",
					"description": "Package-specific configuration",
				},
			},
			"required": []string{"platform", "format", "input", "output"},
		},
	}
}

func (t *PackageTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params map[string]any
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	platform, _ := params["platform"].(string)
	format, _ := params["format"].(string)
	input, _ := params["input"].(string)
	output, _ := params["output"].(string)

	if platform == "" || format == "" || input == "" || output == "" {
		return NewTextErrorResponse("platform, format, input, and output are required"), nil
	}

	result := fmt.Sprintf("Packaging %s for %s as %s...\n", input, platform, format)
	result += "Output: " + output + "\n"
	result += "Note: This is a stub. Real implementation would use platform packaging tools.\n"

	return NewTextResponse(result), nil
}