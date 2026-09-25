package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/svpc-ai/svpc/internal/permission"
)

const (
	GitHubCLIToolName = "gh"
	GitLabCLIToolName = "glab"

	// WebhookToolName manages repository webhooks.
	WebhookToolName = "webhook"
)

// cliTool drives a code-hosting CLI. Using the official binary keeps every
// capability the CLI has, instead of re-implementing a fraction of its API.
type cliTool struct {
	toolName string
	binary   string
	// summary renders the purpose of the tool for Info().
	summary string
	runner  *runner
}

func NewGitHubCLITool(permissions permission.Service) BaseTool {
	return &cliTool{
		toolName: GitHubCLIToolName,
		binary:   "gh",
		summary: "Run GitHub CLI commands: pull requests, issues, releases, workflows, " +
			"secrets, variables, repos, runs and more. The first argument is the gh subcommand.",
		runner: newRunner(permissions),
	}
}

func NewGitLabCLITool(permissions permission.Service) BaseTool {
	return &cliTool{
		toolName: GitLabCLIToolName,
		binary:   "glab",
		summary: "Run GitLab CLI commands: merge requests, issues, pipelines, releases, " +
			"variables and more. The first argument is the glab subcommand.",
		runner: newRunner(permissions),
	}
}

func (t *cliTool) Info() ToolInfo {
	return ToolInfo{
		Name:        t.toolName,
		Description: t.summary,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Subcommand, e.g. 'pr list', 'issue create', 'workflow run'",
				},
				"args": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Additional arguments passed to the subcommand",
				},
				"repo": map[string]any{
					"type":        "string",
					"description": "Repository in owner/name form (defaults to the current repository)",
				},
				"json": map[string]any{
					"type":        "boolean",
					"description": "Request JSON output where the subcommand supports it",
				},
			},
			"required": []string{"command"},
		},
	}
}

type cliParams struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Repo    string   `json:"repo"`
	JSON    bool     `json:"json"`
}

func (t *cliTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var p cliParams
	if err := json.Unmarshal([]byte(call.Input), &p); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}
	if strings.TrimSpace(p.Command) == "" {
		return NewTextErrorResponse("command is required"), nil
	}

	args := strings.Fields(p.Command)
	args = append(args, p.Args...)
	if p.Repo != "" {
		args = append(args, "--repo", p.Repo)
	}
	if p.JSON {
		args = append(args, "--json")
	}

	sessionID, messageID := sessionContext(ctx)
	if err := t.runner.ask(ctx, sessionID, messageID, t.toolName,
		fmt.Sprintf("%s %s", t.binary, strings.Join(args, " ")), p.Command,
		map[string]any{"command": p.Command, "repo": p.Repo}); err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	res, err := t.runner.exec(ctx, t.binary, args...)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}
	return NewTextResponse(res.String()), nil
}

type webhookParams struct {
	Provider string   `json:"provider"`
	Action   string   `json:"action"`
	Owner    string   `json:"owner"`
	Repo     string   `json:"repo"`
	URL      string   `json:"url"`
	Events   []string `json:"events"`
	Secret   string   `json:"secret"`
	ID       int      `json:"id"`
	Token    string   `json:"token"`
	APIURL   string   `json:"api_url"`
}

// WebhookTool creates and inspects repository webhooks over the REST API.
type WebhookTool struct {
	runner *runner
}

func NewWebhookTool(permissions permission.Service) BaseTool {
	return &WebhookTool{runner: newRunner(permissions)}
}

func (t *WebhookTool) Info() ToolInfo {
	return ToolInfo{
		Name:        WebhookToolName,
		Description: "Create, list, inspect, ping and delete repository webhooks on GitHub, GitLab and Bitbucket.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type": "string", "enum": []string{"github", "gitlab", "bitbucket"},
				},
				"action": map[string]any{
					"type": "string", "enum": []string{"create", "list", "get", "delete", "ping"},
				},
				"owner":   map[string]any{"type": "string", "description": "Repository owner or group"},
				"repo":    map[string]any{"type": "string", "description": "Repository name"},
				"url":     map[string]any{"type": "string", "description": "Target URL (required for create)"},
				"events":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Events to subscribe to"},
				"secret":  map[string]any{"type": "string", "description": "Signing secret"},
				"id":      map[string]any{"type": "integer", "description": "Webhook id (for get, delete, ping)"},
				"token":   map[string]any{"type": "string", "description": "API token (defaults to the environment variable)"},
				"api_url": map[string]any{"type": "string", "description": "Base API URL for a self-hosted instance"},
			},
			"required": []string{"provider", "action", "owner", "repo"},
		},
	}
}

func (t *WebhookTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var p webhookParams
	if err := json.Unmarshal([]byte(call.Input), &p); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	if p.Owner == "" || p.Repo == "" {
		return NewTextErrorResponse("owner and repo are required"), nil
	}

	provider := strings.ToLower(p.Provider)
	token := p.Token
	if token == "" {
		token = tokenForProvider(provider)
	}
	if token == "" {
		return NewTextErrorResponse(fmt.Sprintf("no token for %s; set the provider's environment variable or pass token", provider)), nil
	}

	base := p.APIURL
	if base == "" {
		base = defaultAPIURL(provider)
	}
	if base == "" {
		return NewTextErrorResponse(fmt.Sprintf("unsupported provider: %s", provider)), nil
	}

	action := strings.ToLower(p.Action)
	if action == "create" && p.URL == "" {
		return NewTextErrorResponse("url is required for create"), nil
	}
	if (action == "get" || action == "delete" || action == "ping") && p.ID == 0 {
		return NewTextErrorResponse(fmt.Sprintf("id is required for %s", action)), nil
	}

	sessionID, messageID := sessionContext(ctx)
	if err := t.runner.ask(ctx, sessionID, messageID, WebhookToolName,
		fmt.Sprintf("%s webhook %s on %s/%s", provider, action, p.Owner, p.Repo), action,
		map[string]any{"provider": provider, "action": action, "owner": p.Owner, "repo": p.Repo},
	); err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	res, err := t.runner.exec(ctx, "curl", curlArgs(provider, base, p, token, action)...)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}
	return NewTextResponse(res.String()), nil
}

// tokenForProvider reads the conventional environment variable for a provider.
func tokenForProvider(provider string) string {
	switch provider {
	case "github":
		return getEnvOrDefault("GITHUB_TOKEN", "")
	case "gitlab":
		return getEnvOrDefault("GITLAB_TOKEN", "")
	case "bitbucket":
		return getEnvOrDefault("BITBUCKET_TOKEN", "")
	default:
		return ""
	}
}

func defaultAPIURL(provider string) string {
	switch provider {
	case "github":
		return "https://api.github.com"
	case "gitlab":
		return "https://gitlab.com/api/v4"
	case "bitbucket":
		return "https://api.bitbucket.org/2.0"
	default:
		return ""
	}
}

// curlArgs builds the request. curl is used because it is present on every
// supported platform and keeps the secret out of a Go HTTP client config that
// would otherwise need to log-inspect it.
func curlArgs(provider, base string, p webhookParams, token, action string) []string {
	owner, repo := p.Owner, p.Repo
	auth := "Bearer " + token

	var (
		method string
		path   string
		body   []string
	)

	switch provider {
	case "github":
		auth = "Bearer " + token
		path = fmt.Sprintf("/repos/%s/%s/hooks", owner, repo)
		if action != "create" && action != "list" {
			path += "/" + strconv.Itoa(p.ID)
		}
		switch action {
		case "create":
			method = "POST"
			body = []string{gitHubHookBody(p)}
		case "delete":
			method = "DELETE"
		case "ping":
			method = "POST"
			path += "/pings"
		default:
			method = "GET"
		}

	case "gitlab":
		auth = "PRIVATE-TOKEN " + token
		path = fmt.Sprintf("/projects/%s%%2F%s/hooks", owner, repo)
		if action != "create" && action != "list" {
			path += "/" + strconv.Itoa(p.ID)
		}
		switch action {
		case "create":
			method = "POST"
			body = []string{`{"url":"` + p.URL + `","push_events":true`,
				`,"merge_requests_events":true,"enable_ssl_verification":true`}
		case "delete":
			method = "DELETE"
		default:
			method = "GET"
		}

	case "bitbucket":
		auth = "Bearer " + token
		path = fmt.Sprintf("/repositories/%s/%s/hooks", owner, repo)
		if action != "create" && action != "list" {
			path += "/" + strconv.Itoa(p.ID)
		}
		switch action {
		case "create":
			method = "POST"
			body = []string{`{"description":"svpc","url":"` + p.URL + `","active":true`,
				`,"events":["repo:push"]}`}
		case "delete":
			method = "DELETE"
		case "ping":
			method = "POST"
			path += "/tests"
		default:
			method = "GET"
		}
	}

	args := []string{"-sS", "-X", method, "-H", "Authorization: " + auth,
		"-H", "Accept: application/json"}
	if len(body) > 0 {
		args = append(args, "-H", "Content-Type: application/json", "-d", body[0])
	}
	return append(args, strings.TrimRight(base, "/")+path)
}

func quoteJoin(items []string) string {
	quoted := make([]string, 0, len(items))
	for _, it := range items {
		quoted = append(quoted, `"`+it+`"`)
	}
	return strings.Join(quoted, ",")
}

// gitHubHookBody renders the create payload. Events default to the common
// triggers rather than an empty list, which GitHub rejects.
func gitHubHookBody(p webhookParams) string {
	events := p.Events
	if len(events) == 0 {
		events = []string{"push", "pull_request", "issues"}
	}

	config := `{"url":"` + p.URL + `","content_type":"json"`
	if p.Secret != "" {
		config += `,"secret":"` + p.Secret + `"`
	}
	config += "}"

	return `{"config":` + config + `,"events":[` + quoteJoin(events) + `]}`
}
