package tools

import (
	"context"
	"testing"
)

func TestBuildTool_Info(t *testing.T) {
	tool := NewBuildTool()
	info := tool.Info()

	if info.Name != BuildToolName {
		t.Errorf("expected name %q, got %q", BuildToolName, info.Name)
	}

	if info.Description == "" {
		t.Error("description should not be empty")
	}

	props := info.Parameters["properties"].(map[string]any)
	required := info.Parameters["required"].([]string)

	// Check required fields
	for _, req := range []string{"platform", "action"} {
		found := false
		for _, r := range required {
			if r == req {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing required parameter: %s", req)
		}
	}

	// Check platform enum
	platformProp := props["platform"].(map[string]any)
	enum := platformProp["enum"].([]string)
	expectedPlatforms := []string{"android", "windows", "linux", "macos", "ios", "all"}
	for _, ep := range expectedPlatforms {
		found := false
		for _, e := range enum {
			if e == ep {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing platform in enum: %s", ep)
		}
	}

	// Check action enum
	actionProp := props["action"].(map[string]any)
	actionEnum := actionProp["enum"].([]string)
	expectedActions := []string{"build", "clean", "test", "lint", "package", "sign", "notarize", "release", "doctor"}
	for _, ea := range expectedActions {
		found := false
		for _, e := range actionEnum {
			if e == ea {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing action in enum: %s", ea)
		}
	}
}

func TestBuildTool_Run_InvalidParams(t *testing.T) {
	tool := NewBuildTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  BuildToolName,
		Input: `{invalid json`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for invalid JSON")
	}
}

func TestBuildTool_Run_MissingRequired(t *testing.T) {
	tool := NewBuildTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  BuildToolName,
		Input: `{}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for missing platform/action")
	}
}

func TestBuildTool_Run_UnknownPlatform(t *testing.T) {
	tool := NewBuildTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  BuildToolName,
		Input: `{"platform": "unknown", "action": "build"}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for unknown platform")
	}
}

func TestBuildTool_Run_UnknownAction(t *testing.T) {
	tool := NewBuildTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  BuildToolName,
		Input: `{"platform": "android", "action": "unknown_action"}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for unknown action")
	}
}

func TestSignTool_Info(t *testing.T) {
	tool := NewSignTool()
	info := tool.Info()

	if info.Name != SignToolName {
		t.Errorf("expected name %q, got %q", SignToolName, info.Name)
	}

	props := info.Parameters["properties"].(map[string]any)
	platformProp := props["platform"].(map[string]any)
	enum := platformProp["enum"].([]string)
	expectedPlatforms := []string{"windows", "macos", "ios", "android"}
	for _, ep := range expectedPlatforms {
		found := false
		for _, e := range enum {
			if e == ep {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing platform in enum: %s", ep)
		}
	}
}

func TestSignTool_Run_MissingFile(t *testing.T) {
	tool := NewSignTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  SignToolName,
		Input: `{"platform": "windows"}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for missing file")
	}
}

func TestNotarizeTool_Info(t *testing.T) {
	tool := NewNotarizeTool()
	info := tool.Info()

	if info.Name != NotarizeToolName {
		t.Errorf("expected name %q, got %q", NotarizeToolName, info.Name)
	}

	required := info.Parameters["required"].([]string)
	for _, req := range []string{"file", "apple_id", "password", "team_id"} {
		found := false
		for _, r := range required {
			if r == req {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing required parameter: %s", req)
		}
	}
}

func TestNotarizeTool_Run_MissingParams(t *testing.T) {
	tool := NewNotarizeTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  NotarizeToolName,
		Input: `{"file": "app.dmg"}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for missing credentials")
	}
}

func TestPackageTool_Info(t *testing.T) {
	tool := NewPackageTool()
	info := tool.Info()

	if info.Name != PackageToolName {
		t.Errorf("expected name %q, got %q", PackageToolName, info.Name)
	}

	platformProp := info.Parameters["properties"].(map[string]any)["platform"].(map[string]any)
	enum := platformProp["enum"].([]string)
	expectedPlatforms := []string{"android", "windows", "linux", "macos", "ios"}
	for _, ep := range expectedPlatforms {
		found := false
		for _, e := range enum {
			if e == ep {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing platform in enum: %s", ep)
		}
	}

	formatProp := info.Parameters["properties"].(map[string]any)["format"].(map[string]any)
	formatEnum := formatProp["enum"].([]string)
	expectedFormats := []string{"apk", "aab", "exe", "msi", "zip", "appimage", "deb", "rpm", "tar.gz", "dmg", "pkg", "app", "ipa"}
	for _, ef := range expectedFormats {
		found := false
		for _, e := range formatEnum {
			if e == ef {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing format in enum: %s", ef)
		}
	}
}

func TestPackageTool_Run_MissingParams(t *testing.T) {
	tool := NewPackageTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  PackageToolName,
		Input: `{"platform": "android"}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for missing format/input/output")
	}
}