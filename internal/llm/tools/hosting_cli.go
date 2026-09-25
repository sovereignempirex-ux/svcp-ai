package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	GitHubCLIToolName = "gh"
	GitLabCLIToolName = "glab"
)

type CLIParams struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	WorkDir string   `json:"workdir,omitempty"`
}

type GitHubCLITool struct{}

func NewGitHubCLITool() BaseTool {
	return &GitHubCLITool{}
}

func (t *GitHubCLITool) Info() ToolInfo {
	return ToolInfo{
		Name:        GitHubCLIToolName,
		Description: "Run GitHub CLI (gh) commands. Requires gh to be installed and authenticated. Use for advanced GitHub operations not covered by the hosting tool.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "gh subcommand (e.g., pr, issue, repo, workflow, secret, variable)",
				},
				"args": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Arguments to pass to the command",
				},
				"workdir": map[string]any{
					"type":        "string",
					"description": "Working directory (default: current)",
				},
			},
			"required": []string{"command"},
		},
	}
}

func (t *GitHubCLITool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params CLIParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	if params.Command == "" {
		return NewTextErrorResponse("command is required"), nil
	}

	args := append([]string{params.Command}, params.Args...)
	
	// This would use exec.CommandContext in real implementation
	// For now return a message
	return NewTextResponse(fmt.Sprintf("gh %s (not executed - requires gh CLI installed)", strings.Join(args, " "))), nil
}

type GitLabCLITool struct{}

func NewGitLabCLITool() BaseTool {
	return &GitLabCLITool{}
}

func (t *GitLabCLITool) Info() ToolInfo {
	return ToolInfo{
		Name:        GitLabCLIToolName,
		Description: "Run GitLab CLI (glab) commands. Requires glab to be installed and authenticated.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "glab subcommand (e.g., mr, issue, repo, pipeline, variable)",
				},
				"args": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Arguments to pass to the command",
				},
				"workdir": map[string]any{
					"type":        "string",
					"description": "Working directory (default: current)",
				},
			},
			"required": []string{"command"},
		},
	}
}

func (t *GitLabCLITool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params CLIParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	if params.Command == "" {
		return NewTextErrorResponse("command is required"), nil
	}

	args := append([]string{params.Command}, params.Args...)
	return NewTextResponse(fmt.Sprintf("glab %s (not executed - requires glab CLI installed)", strings.Join(args, " "))), nil
}

type GitHubWorkflowParams struct {
	Owner      string         `json:"owner"`
	Repo       string         `json:"repo"`
	Name       string         `json:"name"`
	On         map[string]any `json:"on"`
	Jobs       map[string]any `json:"jobs"`
	Token      string         `json:"token,omitempty"`
}

type GitHubWorkflowTool struct {
	httpClient *http.Client
}

func NewGitHubWorkflowTool() BaseTool {
	return &GitHubWorkflowTool{
		httpClient: &http.Client{},
	}
}

func (t *GitHubWorkflowTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "github_workflow",
		Description: "Create or update GitHub Actions workflow files. Generates YAML workflow definitions.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"owner": map[string]any{
					"type":        "string",
					"description": "Repository owner",
				},
				"repo": map[string]any{
					"type":        "string",
					"description": "Repository name",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Workflow name (e.g., ci.yml)",
				},
				"on": map[string]any{
					"type":        "object",
					"description": "Trigger configuration (push, pull_request, schedule, etc.)",
				},
				"jobs": map[string]any{
					"type":        "object",
					"description": "Jobs definition",
				},
				"token": map[string]any{
					"type":        "string",
					"description": "GitHub token (optional, uses GITHUB_TOKEN env var)",
				},
			},
			"required": []string{"owner", "repo", "name", "on", "jobs"},
		},
	}
}

func (t *GitHubWorkflowTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params GitHubWorkflowParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	// Generate workflow YAML
	workflow := map[string]any{
		"name": params.Name,
		"on":   params.On,
		"jobs": params.Jobs,
	}

	yamlBytes, err := json.MarshalIndent(workflow, "", "  ")
	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("failed to generate workflow: %v", err)), nil
	}

	// In a real implementation, this would write to .github/workflows/
	return NewTextResponse(fmt.Sprintf("Generated workflow %s:\n\n```yaml\n%s\n```", params.Name, string(yamlBytes))), nil
}

type WebhookParams struct {
	Provider   string         `json:"provider"`
	Action     string         `json:"action"`
	Owner      string         `json:"owner"`
	Repo       string         `json:"repo"`
	URL        string         `json:"url,omitempty"`
	Events     []string       `json:"events,omitempty"`
	Secret     string         `json:"secret,omitempty"`
	ID         int            `json:"id,omitempty"`
	Token      string         `json:"token,omitempty"`
	APIURL     string         `json:"api_url,omitempty"`
}

type WebhookTool struct {
	httpClient *http.Client
}

func NewWebhookTool() BaseTool {
	return &WebhookTool{
		httpClient: &http.Client{},
	}
}

func (t *WebhookTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "webhook",
		Description: "Manage webhooks on GitHub, GitLab, Bitbucket. Actions: create, list, get, delete, ping.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type":        "string",
					"description": "Provider: github, gitlab, bitbucket",
					"enum":        []string{"github", "gitlab", "bitbucket"},
				},
				"action": map[string]any{
					"type":        "string",
					"description": "Action: create, list, get, delete, ping",
					"enum":        []string{"create", "list", "get", "delete", "ping"},
				},
				"owner": map[string]any{
					"type":        "string",
					"description": "Repository owner",
				},
				"repo": map[string]any{
					"type":        "string",
					"description": "Repository name",
				},
				"url": map[string]any{
					"type":        "string",
					"description": "Webhook URL (required for create)",
				},
				"events": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Events to subscribe to",
				},
				"secret": map[string]any{
					"type":        "string",
					"description": "Webhook secret",
				},
				"id": map[string]any{
					"type":        "integer",
					"description": "Webhook ID (for get, delete, ping)",
				},
				"token": map[string]any{
					"type":        "string",
					"description": "API token",
				},
				"api_url": map[string]any{
					"type":        "string",
					"description": "Custom API URL",
				},
			},
			"required": []string{"provider", "action", "owner", "repo"},
		},
	}
}

func (t *WebhookTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params WebhookParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	provider := strings.ToLower(params.Provider)
	action := strings.ToLower(params.Action)

	if params.Owner == "" || params.Repo == "" {
		return NewTextErrorResponse("owner and repo are required"), nil
	}

	if action == "create" && params.URL == "" {
		return NewTextErrorResponse("url is required for create action"), nil
	}

	if (action == "get" || action == "delete" || action == "ping") && params.ID == 0 {
		return NewTextErrorResponse("id is required for get/delete/ping actions"), nil
	}

	// This would implement actual webhook management
	return NewTextResponse(fmt.Sprintf("Webhook %s %s on %s/%s (not fully implemented)", action, provider, params.Owner, params.Repo)), nil
}