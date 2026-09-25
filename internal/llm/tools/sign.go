package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/svpc-ai/svpc/internal/permission"
)

// SignTool signs a built artifact with the platform's native signing tool.
type SignTool struct {
	runner *runner
}

func NewSignTool(permissions permission.Service) BaseTool {
	return &SignTool{runner: newRunner(permissions)}
}

func (t *SignTool) Info() ToolInfo {
	return ToolInfo{
		Name: SignToolName,
		Description: "Sign a build artifact. Windows uses signtool with a certificate file, " +
			"macOS and iOS use codesign (and an optional provisioning profile), " +
			"Android uses apksigner with a keystore.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"platform": map[string]any{
					"type": "string",
					"enum": []string{"windows", "macos", "ios", "android"},
				},
				"file":                 map[string]any{"type": "string", "description": "Artifact to sign"},
				"certificate":          map[string]any{"type": "string", "description": "Certificate (.pfx) or thumbprint (Windows)"},
				"password":             map[string]any{"type": "string", "description": "Certificate password"},
				"timestamp":            map[string]any{"type": "string", "description": "Timestamp server URL (Windows)"},
				"identity":             map[string]any{"type": "string", "description": "Developer ID Application name (macOS)"},
				"entitlements":         map[string]any{"type": "string", "description": "Entitlements plist (macOS)"},
				"provisioning_profile": map[string]any{"type": "string", "description": "Provisioning profile (macOS/iOS)"},
				"bundle_id":            map[string]any{"type": "string", "description": "Bundle identifier (macOS/iOS)"},
				"keystore":             map[string]any{"type": "string", "description": "Android keystore path"},
				"keystore_alias":       map[string]any{"type": "string", "description": "Android keystore alias"},
				"keystore_password":    map[string]any{"type": "string", "description": "Keystore password"},
				"key_password":         map[string]any{"type": "string", "description": "Key password"},
			},
			"required": []string{"platform", "file"},
		},
	}
}

type signParams struct {
	Platform            string `json:"platform"`
	File                string `json:"file"`
	Certificate         string `json:"certificate"`
	Password            string `json:"password"`
	Timestamp           string `json:"timestamp"`
	Identity            string `json:"identity"`
	Entitlements        string `json:"entitlements"`
	ProvisioningProfile string `json:"provisioning_profile"`
	BundleID            string `json:"bundle_id"`
	Keystore            string `json:"keystore"`
	KeystoreAlias       string `json:"keystore_alias"`
	KeystorePassword    string `json:"keystore_password"`
	KeyPassword         string `json:"key_password"`
}

func (t *SignTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var p signParams
	if err := json.Unmarshal([]byte(call.Input), &p); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}
	if p.File == "" {
		return NewTextErrorResponse("file is required"), nil
	}

	binary, args, err := signCommand(p)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	sessionID, messageID := sessionContext(ctx)
	if err := t.runner.ask(ctx, sessionID, messageID, SignToolName,
		fmt.Sprintf("sign %s for %s", p.File, p.Platform), p.Platform,
		redactSign(p)); err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	res, err := t.runner.exec(ctx, binary, args...)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}
	if res.ExitCode == 0 && strings.TrimSpace(res.Stdout+res.Stderr) == "" {
		return NewTextResponse(fmt.Sprintf("signed %s (%s)", p.File, p.Platform)), nil
	}
	return NewTextResponse(res.String()), nil
}

// signCommand maps a platform onto its signing tool.
func signCommand(p signParams) (string, []string, error) {
	switch strings.ToLower(p.Platform) {
	case "windows":
		if p.Certificate == "" {
			return "", nil, fmt.Errorf("certificate is required for windows signing")
		}
		args := []string{"sign", "/fd", "sha256"}
		if isThumbprint(p.Certificate) {
			args = append(args, "/sha1", p.Certificate)
		} else {
			args = append(args, "/f", p.Certificate)
		}
		if p.Password != "" {
			args = append(args, "/p", p.Password)
		}
		if p.Timestamp != "" {
			args = append(args, "/tr", p.Timestamp, "/td", "sha256")
		}
		return "signtool", append(args, p.File), nil

	case "macos", "ios":
		args := []string{"--force", "--sign", orDefault(p.Identity, "-")}
		if p.Entitlements != "" {
			args = append(args, "--entitlements", p.Entitlements)
		}
		if p.ProvisioningProfile != "" {
			args = append(args, "--profile", p.ProvisioningProfile)
		}
		if p.Timestamp != "" {
			args = append(args, "--timestamp", p.Timestamp)
		}
		return "codesign", append(args, p.File), nil

	case "android":
		if p.Keystore == "" || p.KeystoreAlias == "" {
			return "", nil, fmt.Errorf("keystore and keystore_alias are required for android signing")
		}
		args := []string{"sign", "--ks", p.Keystore, "--ks-key-alias", p.KeystoreAlias}
		if p.KeystorePassword != "" {
			args = append(args, "--ks-pass", "pass:"+p.KeystorePassword)
		}
		if p.KeyPassword != "" {
			args = append(args, "--key-pass", "pass:"+p.KeyPassword)
		}
		return "apksigner", append(args, p.File), nil

	default:
		return "", nil, fmt.Errorf("unsupported platform: %s", p.Platform)
	}
}

// isThumbprint reports whether the value looks like a certificate thumbprint
// rather than a file path.
func isThumbprint(v string) bool {
	v = strings.TrimPrefix(strings.TrimSpace(v), "sha1:")
	v = strings.ReplaceAll(v, " ", "")
	if len(v) != 40 {
		return false
	}
	_, err := strconv.ParseUint(v[:8], 16, 32)
	return err == nil
}

func redactSign(p signParams) map[string]any {
	out := map[string]any{"platform": p.Platform, "file": p.File}
	if p.Identity != "" {
		out["identity"] = p.Identity
	}
	if p.Certificate != "" {
		out["certificate"] = p.Certificate
	}
	for _, secret := range []string{"password", "keystore_password", "key_password"} {
		if p.Password != "" {
			out[secret] = "***"
		}
	}
	return out
}

// NotarizeTool submits a macOS or iOS artifact to Apple for notarization.
type NotarizeTool struct {
	runner *runner
}

func NewNotarizeTool(permissions permission.Service) BaseTool {
	return &NotarizeTool{runner: newRunner(permissions)}
}

func (t *NotarizeTool) Info() ToolInfo {
	return ToolInfo{
		Name: NotarizeToolName,
		Description: "Notarize a macOS or iOS build with Apple using notarytool, then staple the " +
			"ticket. The app-specific password is passed through the environment so it never " +
			"appears in a command line.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file":     map[string]any{"type": "string", "description": "DMG, ZIP, PKG or app bundle to notarize"},
				"apple_id": map[string]any{"type": "string", "description": "Apple ID email"},
				"password": map[string]any{"type": "string", "description": "App-specific password"},
				"team_id":  map[string]any{"type": "string", "description": "Apple Team ID"},
				"staple":   map[string]any{"type": "boolean", "description": "Staple the ticket after submission"},
			},
			"required": []string{"file", "apple_id", "password", "team_id"},
		},
	}
}

type notarizeParams struct {
	File     string `json:"file"`
	AppleID  string `json:"apple_id"`
	Password string `json:"password"`
	TeamID   string `json:"team_id"`
	Staple   bool   `json:"staple"`
}

func (t *NotarizeTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var p notarizeParams
	if err := json.Unmarshal([]byte(call.Input), &p); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}
	if p.File == "" || p.AppleID == "" || p.Password == "" || p.TeamID == "" {
		return NewTextErrorResponse("file, apple_id, password and team_id are required"), nil
	}

	sessionID, messageID := sessionContext(ctx)
	if err := t.runner.ask(ctx, sessionID, messageID, NotarizeToolName,
		"notarize "+p.File, "notarize", map[string]any{"file": p.File, "team_id": p.TeamID}); err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	// NOTARY_PASSWORD is read by notarytool itself, which keeps the secret out
	// of the argument list and the transcript.
	env := []string{"NOTARY_PASSWORD=" + p.Password}
	submit := []string{"notarytool", "submit", p.File,
		"--apple-id", p.AppleID, "--team-id", p.TeamID, "--wait"}

	res, err := runWithEnv(ctx, t.runner, "xcrun", submit, env)
	if err != nil {
		return res, nil
	}
	if res.IsError {
		return res, nil
	}

	if p.Staple {
		staple, err := t.runner.exec(ctx, "xcrun", "stapler", "staple", p.File)
		if err != nil {
			return NewTextErrorResponse(err.Error()), nil
		}
		return NewTextResponse(staple.String()), nil
	}
	return res, nil
}

// PackageTool turns a build artifact into a distributable package.
type PackageTool struct {
	runner *runner
}

func NewPackageTool(permissions permission.Service) BaseTool {
	return &PackageTool{runner: newRunner(permissions)}
}

func (t *PackageTool) Info() ToolInfo {
	return ToolInfo{
		Name: PackageToolName,
		Description: "Package a build artifact: APK/AAB for Android, ZIP/EXE for Windows, " +
			"AppImage/deb/rpm for Linux, DMG/ZIP for macOS, IPA for iOS.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"platform": map[string]any{
					"type": "string",
					"enum": []string{"android", "windows", "linux", "macos", "ios"},
				},
				"format": map[string]any{
					"type": "string",
					"enum": []string{"apk", "aab", "exe", "msi", "zip", "appimage", "deb", "rpm", "tar.gz", "dmg", "pkg", "ipa"},
				},
				"input":       map[string]any{"type": "string", "description": "Artifact or directory to package"},
				"output":      map[string]any{"type": "string", "description": "Output path for the package"},
				"version":     map[string]any{"type": "string", "description": "Version, used for the package name"},
				"arch":        map[string]any{"type": "string", "description": "Architecture, used for the deb/rpm name"},
				"app_name":    map[string]any{"type": "string", "description": "Application name for deb/rpm metadata"},
				"maintainer":  map[string]any{"type": "string", "description": "Maintainer for deb/rpm metadata"},
				"description": map[string]any{"type": "string", "description": "Short package description"},
			},
			"required": []string{"platform", "format", "input", "output"},
		},
	}
}

type packageParams struct {
	Platform    string `json:"platform"`
	Format      string `json:"format"`
	Input       string `json:"input"`
	Output      string `json:"output"`
	Version     string `json:"version"`
	Arch        string `json:"arch"`
	AppName     string `json:"app_name"`
	Maintainer  string `json:"maintainer"`
	Description string `json:"description"`
}

func (t *PackageTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var p packageParams
	if err := json.Unmarshal([]byte(call.Input), &p); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}
	if p.Input == "" || p.Output == "" || p.Platform == "" || p.Format == "" {
		return NewTextErrorResponse("platform, format, input and output are required"), nil
	}

	binary, args, err := packageCommand(p)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	sessionID, messageID := sessionContext(ctx)
	if err := t.runner.ask(ctx, sessionID, messageID, PackageToolName,
		fmt.Sprintf("package %s as %s", p.Input, p.Format), p.Format,
		map[string]any{"platform": p.Platform, "format": p.Format, "output": p.Output}); err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	res, err := t.runner.exec(ctx, binary, args...)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}
	return NewTextResponse(res.String()), nil
}

// packageCommand maps a format onto the tool that can produce it.
func packageCommand(p packageParams) (string, []string, error) {
	format := strings.ToLower(p.Format)

	switch format {
	case "zip", "tar.gz":
		tar := "tar"
		args := []string{"-czf", p.Output}
		if format == "zip" {
			// tar cannot write zip; delegate to the archiver the system has.
			if _, err := lookPath("zip"); err == nil {
				return "zip", []string{"-r", p.Output, p.Input}, nil
			}
			tar = "tar"
			args = []string{"-cf", p.Output, p.Input}
		}
		return tar, append(args, "-C", parentOf(p.Input), baseOf(p.Input)), nil

	case "dmg":
		// create-dmg is the usual choice; hdiutil is the fallback.
		if _, err := lookPath("create-dmg"); err == nil {
			return "create-dmg", []string{p.Output, p.Input}, nil
		}
		return "hdiutil", []string{"create", "-volname", p.Input, "-srcfolder", p.Input,
			"-ov", "-format", "UDZO", p.Output}, nil

	case "pkg":
		return "pkgbuild", []string{"--install-location", "/Applications",
			p.Input, p.Output}, nil

	case "deb", "rpm":
		name := orDefault(p.AppName, baseOf(p.Input))
		meta := []string{
			"--name", name,
			"--version", orDefault(p.Version, "1.0.0"),
			"--arch", orDefault(p.Arch, "amd64"),
			"--maintainer", orDefault(p.Maintainer, "maintainer@example.com"),
			"--description", orDefault(p.Description, name),
			"--license", "MIT",
			"--depends", "bash",
			"--output", p.Output,
		}
		if format == "deb" {
			return "nfpm", append([]string{"pack", "-p", "deb"}, append(meta, p.Input)...), nil
		}
		return "nfpm", append([]string{"pack", "-p", "rpm"}, append(meta, p.Input)...), nil

	case "appimage":
		return "appimage-builder", []string{}, nil

	case "apk", "aab", "ipa", "exe", "msi", "app":
		// These come straight out of the native toolchain; there is no separate
		// packaging step to run.
		return "", nil, fmt.Errorf(
			"%s is produced directly by the %s build — run the build action instead of packaging",
			format, p.Platform)
	}

	return "", nil, fmt.Errorf("unsupported format: %s", p.Format)
}
