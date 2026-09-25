package tools

import (
	"context"
	"testing"
)

func TestHostingTool_Info(t *testing.T) {
	tool := NewHostingTool()
	info := tool.Info()

	if info.Name != "hosting" {
		t.Errorf("expected name %q, got %q", "hosting", info.Name)
	}

	if info.Description == "" {
		t.Error("description should not be empty")
	}

	if info.Parameters == nil {
		t.Error("parameters should not be nil")
	}

	// Check required parameters
	props := info.Parameters["properties"].(map[string]any)
	if props["provider"] == nil {
		t.Error("provider parameter missing")
	}
	if props["action"] == nil {
		t.Error("action parameter missing")
	}
}

func TestHostingTool_Run_InvalidParams(t *testing.T) {
	tool := NewHostingTool()

	// Test with invalid JSON
	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  "hosting",
		Input: `{invalid json`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for invalid JSON")
	}
}

func TestHostingTool_Run_MissingRequiredParams(t *testing.T) {
	tool := NewHostingTool()

	// Test with missing provider and action
	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  "hosting",
		Input: `{"owner": "test", "repo": "test"}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for missing provider/action")
	}
}

func TestHostingTool_Run_UnknownAction(t *testing.T) {
	tool := NewHostingTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  "hosting",
		Input: `{"provider": "github", "action": "unknown_action", "owner": "test", "repo": "test"}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for unknown action")
	}
}

func TestHostingTool_Run_MissingToken(t *testing.T) {
	tool := NewHostingTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  "hosting",
		Input: `{"provider": "github", "action": "list_issues", "owner": "test", "repo": "test"}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for missing token")
	}
}