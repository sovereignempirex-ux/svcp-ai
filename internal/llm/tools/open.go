package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/skratchdot/open-golang/open"
)

const OpenToolName = "open"

type OpenParams struct {
	URL  string `json:"url,omitempty"`
	Path string `json:"path,omitempty"`
}

type OpenTool struct{}

func NewOpenTool() BaseTool {
	return &OpenTool{}
}

func (t *OpenTool) Info() ToolInfo {
	return ToolInfo{
		Name:        OpenToolName,
		Description: "Open a URL in the default browser, or open a file/application with the system default handler. Provide either 'url' (for web URLs) or 'path' (for local files/apps).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "A web URL to open in the default browser (e.g., 'https://github.com/svpc-ai/svpc')",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "A local file path or application to open with the system default handler (e.g., '/path/to/file.txt' or 'code' for VS Code)",
				},
			},
		},
		Required: []string{},
	}
}

func (t *OpenTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params OpenParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	// Validate that at least one parameter is provided
	if params.URL == "" && params.Path == "" {
		return NewTextErrorResponse("either 'url' or 'path' parameter is required"), nil
	}

	var target string
	if params.URL != "" {
		target = params.URL
	} else {
		target = params.Path
	}

	// Use skratchdot/open-golang for cross-platform support
	err := open.Run(target)
	if err != nil {
		// Fallback for specific cases
		if runtime.GOOS == "windows" && params.Path != "" {
			// On Windows, try using cmd /c start for applications
			return NewTextErrorResponse(fmt.Sprintf("failed to open: %v (try providing a full path or URL)", err)), nil
		}
		return NewTextErrorResponse(fmt.Sprintf("failed to open: %v", err)), nil
	}

	var msg string
	if params.URL != "" {
		msg = fmt.Sprintf("Opened URL: %s", params.URL)
	} else {
		msg = fmt.Sprintf("Opened: %s", params.Path)
	}

	return NewTextResponse(msg), nil
}
