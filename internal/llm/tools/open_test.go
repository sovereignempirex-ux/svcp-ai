package tools

import (
	"context"
	"testing"
)

func TestOpenTool_Info(t *testing.T) {
	tool := NewOpenTool()
	info := tool.Info()

	if info.Name != OpenToolName {
		t.Errorf("expected name %q, got %q", OpenToolName, info.Name)
	}

	if info.Description == "" {
		t.Error("description should not be empty")
	}

	if info.Parameters == nil {
		t.Error("parameters should not be nil")
	}
}

func TestOpenTool_Run_InvalidParams(t *testing.T) {
	tool := NewOpenTool()

	// Test with invalid JSON
	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  OpenToolName,
		Input: `{invalid json`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for invalid JSON")
	}
}

func TestOpenTool_Run_NoParams(t *testing.T) {
	tool := NewOpenTool()

	// Test with empty params (no url or path)
	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  OpenToolName,
		Input: `{"url": "", "path": ""}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response when no params provided")
	}
}
