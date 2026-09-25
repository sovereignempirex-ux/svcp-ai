package tools

import (
	"context"
	"testing"
)

func TestImageGenTool_Info(t *testing.T) {
	tool := NewImageGenTool()
	info := tool.Info()

	if info.Name != ImageGenToolName {
		t.Errorf("expected name %q, got %q", ImageGenToolName, info.Name)
	}

	if info.Description == "" {
		t.Error("description should not be empty")
	}

	props := info.Parameters["properties"].(map[string]any)
	required := info.Parameters["required"].([]string)

	// Check required fields
	for _, req := range []string{"provider", "prompt"} {
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

	// Check properties exist
	for _, prop := range []string{"provider", "prompt", "model", "size", "quality", "style", "n", "seed", "negative_prompt", "aspect_ratio"} {
		if props[prop] == nil {
			t.Errorf("missing property: %s", prop)
		}
	}
}

func TestImageGenTool_Run_InvalidParams(t *testing.T) {
	tool := NewImageGenTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  ImageGenToolName,
		Input: `{invalid json`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for invalid JSON")
	}
}

func TestImageGenTool_Run_MissingPrompt(t *testing.T) {
	tool := NewImageGenTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  ImageGenToolName,
		Input: `{"provider": "openai"}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for missing prompt")
	}
}

func TestImageGenTool_Run_MissingToken(t *testing.T) {
	tool := NewImageGenTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  ImageGenToolName,
		Input: `{"provider": "openai", "prompt": "test"}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for missing token")
	}
}

func TestImageEditTool_Info(t *testing.T) {
	tool := NewImageEditTool()
	info := tool.Info()

	if info.Name != ImageEditToolName {
		t.Errorf("expected name %q, got %q", ImageEditToolName, info.Name)
	}

	props := info.Parameters["properties"].(map[string]any)
	for _, prop := range []string{"provider", "prompt", "image_path", "mask_path", "model", "size", "n"} {
		if props[prop] == nil {
			t.Errorf("missing property: %s", prop)
		}
	}
}

func TestImageEditTool_Run_MissingParams(t *testing.T) {
	tool := NewImageEditTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  ImageEditToolName,
		Input: `{"provider": "openai"}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for missing prompt/image_path")
	}
}

func TestImageVariationTool_Info(t *testing.T) {
	tool := NewImageVariationTool()
	info := tool.Info()

	if info.Name != ImageVariationToolName {
		t.Errorf("expected name %q, got %q", ImageVariationToolName, info.Name)
	}

	props := info.Parameters["properties"].(map[string]any)
	for _, prop := range []string{"provider", "image_path", "model", "size", "n"} {
		if props[prop] == nil {
			t.Errorf("missing property: %s", prop)
		}
	}
}

func TestImageVariationTool_Run_MissingImage(t *testing.T) {
	tool := NewImageVariationTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  ImageVariationToolName,
		Input: `{"provider": "openai"}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for missing image_path")
	}
}