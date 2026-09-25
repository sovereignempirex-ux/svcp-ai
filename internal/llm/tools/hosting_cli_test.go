package tools

import (
	"context"
	"testing"
)

func TestGitHubCLITool_Info(t *testing.T) {
	tool := NewGitHubCLITool()
	info := tool.Info()

	if info.Name != GitHubCLIToolName {
		t.Errorf("expected name %q, got %q", GitHubCLIToolName, info.Name)
	}

	if info.Description == "" {
		t.Error("description should not be empty")
	}
}

func TestGitLabCLITool_Info(t *testing.T) {
	tool := NewGitLabCLITool()
	info := tool.Info()

	if info.Name != GitLabCLIToolName {
		t.Errorf("expected name %q, got %q", GitLabCLIToolName, info.Name)
	}
}

func TestGitHubWorkflowTool_Info(t *testing.T) {
	tool := NewGitHubWorkflowTool()
	info := tool.Info()

	if info.Name != "github_workflow" {
		t.Errorf("expected name %q, got %q", "github_workflow", info.Name)
	}

	props := info.Parameters["properties"].(map[string]any)
	required := info.Parameters["required"].([]string)
	
	expectedRequired := []string{"owner", "repo", "name", "on", "jobs"}
	for _, r := range expectedRequired {
		found := false
		for _, req := range required {
			if req == r {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing required parameter: %s", r)
		}
	}
	
	for _, r := range expectedRequired {
		if props[r] == nil {
			t.Errorf("missing property: %s", r)
		}
	}
}

func TestGitHubWorkflowTool_Run(t *testing.T) {
	tool := NewGitHubWorkflowTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID: "test",
		Name: "github_workflow",
		Input: `{
			"owner": "test",
			"repo": "test",
			"name": "ci.yml",
			"on": {"push": {"branches": ["main"]}},
			"jobs": {"build": {"runs-on": "ubuntu-latest", "steps": [{"uses": "actions/checkout@v3"}]}}
		}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if response.IsError {
		t.Errorf("expected success, got error: %s", response.Content)
	}
}

func TestWebhookTool_Info(t *testing.T) {
	tool := NewWebhookTool()
	info := tool.Info()

	if info.Name != "webhook" {
		t.Errorf("expected name %q, got %q", "webhook", info.Name)
	}

	props := info.Parameters["properties"].(map[string]any)
	if props["provider"] == nil {
		t.Error("provider parameter missing")
	}
	if props["action"] == nil {
		t.Error("action parameter missing")
	}
}