package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/svpc-ai/svpc/internal/permission"
)

const (
	GitHubToolName    = "github"
	GitLabToolName    = "gitlab"
	BitbucketToolName = "bitbucket"

	// HostingToolName is the unified provider-agnostic tool that fronts the
	// three platforms above.
	HostingToolName = "hosting"
)

type HostingProvider string

const (
	ProviderGitHub    HostingProvider = "github"
	ProviderGitLab    HostingProvider = "gitlab"
	ProviderBitbucket HostingProvider = "bitbucket"
)

type HostingParams struct {
	Provider  string   `json:"provider"`
	Action    string   `json:"action"`
	Owner     string   `json:"owner,omitempty"`
	Repo      string   `json:"repo,omitempty"`
	Title     string   `json:"title,omitempty"`
	Body      string   `json:"body,omitempty"`
	Head      string   `json:"head,omitempty"`
	Base      string   `json:"base,omitempty"`
	Number    int      `json:"number,omitempty"`
	State     string   `json:"state,omitempty"`
	Labels    []string `json:"labels,omitempty"`
	Assignees []string `json:"assignees,omitempty"`
	Token     string   `json:"token,omitempty"`
	APIURL    string   `json:"api_url,omitempty"`
}

type HostingTool struct {
	httpClient  *http.Client
	permissions permission.Service
	runner      *runner
}

func NewHostingTool(permissions permission.Service) BaseTool {
	return &HostingTool{
		// A provider that stalls must not hold the turn open indefinitely.
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		permissions: permissions,
		runner:      newRunner(permissions),
	}
}

// mutatingHostingAction reports whether an action changes state. Reads are
// listed explicitly so a newly added action is denied by default rather than
// silently allowed.
func mutatingHostingAction(action string) bool {
	switch action {
	case "create_pr", "create_issue", "merge_pr", "close_issue", "add_labels", "assign_issue":
		return true
	default:
		return false
	}
}

// describeHostingAction states the change in the user's terms, including the
// title or body, because "create_issue" alone does not say what was written.
func describeHostingAction(provider HostingProvider, action string, p HostingParams) string {
	repo := p.Owner + "/" + p.Repo
	switch action {
	case "create_pr":
		return fmt.Sprintf("Open pull request on %s %s: %s", provider, repo, quoteOr(p.Title, "no title"))
	case "create_issue":
		return fmt.Sprintf("Open issue on %s %s: %s", provider, repo, quoteOr(p.Title, "no title"))
	case "merge_pr":
		return fmt.Sprintf("Merge pull request #%d on %s %s", p.Number, provider, repo)
	case "close_issue":
		return fmt.Sprintf("Close issue #%d on %s %s", p.Number, provider, repo)
	case "add_labels":
		return fmt.Sprintf("Add labels %s to #%d on %s %s",
			strings.Join(p.Labels, ", "), p.Number, provider, repo)
	case "assign_issue":
		return fmt.Sprintf("Assign #%d on %s %s to %s",
			p.Number, provider, repo, strings.Join(p.Assignees, ", "))
	default:
		return action + " on " + string(provider) + " " + repo
	}
}

func quoteOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return "\"" + truncateRunes(value, 120) + "\""
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// redactHosting keeps the token out of the prompt and the transcript.
func redactHosting(p HostingParams) map[string]any {
	out := map[string]any{
		"provider": p.Provider,
		"action":   p.Action,
		"owner":    p.Owner,
		"repo":     p.Repo,
	}
	if p.Title != "" {
		out["title"] = p.Title
	}
	if p.Number != 0 {
		out["number"] = p.Number
	}
	if len(p.Labels) > 0 {
		out["labels"] = p.Labels
	}
	if len(p.Assignees) > 0 {
		out["assignees"] = p.Assignees
	}
	if p.Token != "" {
		out["token"] = "***"
	}
	return out
}

func (t *HostingTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "hosting",
		Description: "Interact with code hosting platforms (GitHub, GitLab, Bitbucket). Actions: create_pr, create_issue, list_issues, list_prs, get_pr, get_issue, merge_pr, close_issue, add_labels, assign_issue.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type":        "string",
					"description": "Hosting provider: github, gitlab, or bitbucket",
					"enum":        []string{"github", "gitlab", "bitbucket"},
				},
				"action": map[string]any{
					"type":        "string",
					"description": "Action to perform: create_pr, create_issue, list_issues, list_prs, get_pr, get_issue, merge_pr, close_issue, add_labels, assign_issue",
					"enum":        []string{"create_pr", "create_issue", "list_issues", "list_prs", "get_pr", "get_issue", "merge_pr", "close_issue", "add_labels", "assign_issue"},
				},
				"owner": map[string]any{
					"type":        "string",
					"description": "Repository owner/organization",
				},
				"repo": map[string]any{
					"type":        "string",
					"description": "Repository name",
				},
				"title": map[string]any{
					"type":        "string",
					"description": "Title for PR or issue",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Body/description for PR or issue",
				},
				"head": map[string]any{
					"type":        "string",
					"description": "Source branch for PR",
				},
				"base": map[string]any{
					"type":        "string",
					"description": "Target branch for PR (default: main)",
				},
				"number": map[string]any{
					"type":        "integer",
					"description": "PR or issue number",
				},
				"state": map[string]any{
					"type":        "string",
					"description": "State filter: open, closed, all",
					"enum":        []string{"open", "closed", "all"},
				},
				"labels": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Labels to add",
				},
				"assignees": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Assignees to assign",
				},
				"token": map[string]any{
					"type":        "string",
					"description": "API token (optional, uses env var if not provided)",
				},
				"api_url": map[string]any{
					"type":        "string",
					"description": "Custom API URL (for self-hosted GitLab/Bitbucket)",
				},
			},
			"required": []string{"provider", "action"},
		},
	}
}

func (t *HostingTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params HostingParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	provider := HostingProvider(strings.ToLower(params.Provider))
	action := strings.ToLower(params.Action)

	// Validate required fields
	if params.Owner == "" || params.Repo == "" {
		return NewTextErrorResponse("owner and repo are required"), nil
	}

	// Get token from params or environment
	token := params.Token
	if token == "" {
		switch provider {
		case ProviderGitHub:
			token = getEnvOrDefault("GITHUB_TOKEN", "")
		case ProviderGitLab:
			token = getEnvOrDefault("GITLAB_TOKEN", "")
		case ProviderBitbucket:
			token = getEnvOrDefault("BITBUCKET_TOKEN", "")
		}
	}
	if token == "" {
		return NewTextErrorResponse(fmt.Sprintf("no token provided for %s (set %s_TOKEN env var or provide token parameter)", provider, strings.ToUpper(string(provider)))), nil
	}

	// Determine base API URL
	baseURL := params.APIURL
	if baseURL == "" {
		switch provider {
		case ProviderGitHub:
			baseURL = "https://api.github.com"
		case ProviderGitLab:
			baseURL = "https://gitlab.com/api/v4"
		case ProviderBitbucket:
			baseURL = "https://api.bitbucket.org/2.0"
		}
	}

	client := &hostingClient{
		httpClient: t.httpClient,
		baseURL:    baseURL,
		token:      token,
		provider:   provider,
	}

	// Reads are harmless, but anything that changes state on a real repository
	// asks first, the same as a local file write does.
	if mutatingHostingAction(action) {
		sessionID, _ := sessionContext(ctx)
		if err := t.runner.ask(ctx, sessionID, "", HostingToolName,
			describeHostingAction(provider, action, params), action,
			redactHosting(params)); err != nil {
			return NewTextErrorResponse(err.Error()), nil
		}
	}

	var result string
	var err error

	switch action {
	case "create_pr":
		result, err = client.createPR(ctx, params)
	case "create_issue":
		result, err = client.createIssue(ctx, params)
	case "list_issues":
		result, err = client.listIssues(ctx, params)
	case "list_prs":
		result, err = client.listPRs(ctx, params)
	case "get_pr":
		result, err = client.getPR(ctx, params)
	case "get_issue":
		result, err = client.getIssue(ctx, params)
	case "merge_pr":
		result, err = client.mergePR(ctx, params)
	case "close_issue":
		result, err = client.closeIssue(ctx, params)
	case "add_labels":
		result, err = client.addLabels(ctx, params)
	case "assign_issue":
		result, err = client.assignIssue(ctx, params)
	default:
		return NewTextErrorResponse(fmt.Sprintf("unknown action: %s", action)), nil
	}

	if err != nil {
		return NewTextErrorResponse(fmt.Sprintf("%s %s failed: %v", provider, action, err)), nil
	}

	return NewTextResponse(result), nil
}

type hostingClient struct {
	httpClient *http.Client
	baseURL    string
	token      string
	provider   HostingProvider
}

func (c *hostingClient) doRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
	// A nil io.Reader is what "no body" means here. A nil *strings.Reader is not:
	// http.NewRequest type-switches on the concrete type, calls Len on it and
	// panics, which is how every read through this client used to fail.
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	// A GET with no body must not claim to send one.
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, apiError(resp.Status, raw)
	}
	return raw, nil
}

func (c *hostingClient) createPR(ctx context.Context, params HostingParams) (string, error) {
	if params.Title == "" || params.Head == "" {
		return "", fmt.Errorf("title and head (source branch) are required for create_pr")
	}
	base := params.Base
	if base == "" {
		base = "main"
	}

	var payload map[string]any
	switch c.provider {
	case ProviderGitHub:
		payload = map[string]any{
			"title": params.Title,
			"body":  params.Body,
			"head":  params.Head,
			"base":  base,
		}
		data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/repos/%s/%s/pulls", params.Owner, params.Repo), payload)
		return string(data), err
	case ProviderGitLab:
		payload = map[string]any{
			"title":         params.Title,
			"description":   params.Body,
			"source_branch": params.Head,
			"target_branch": base,
		}
		data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/projects/%s%%2F%s/merge_requests", params.Owner, params.Repo), payload)
		return string(data), err
	case ProviderBitbucket:
		payload = map[string]any{
			"title":       params.Title,
			"description": params.Body,
			"source":      map[string]any{"branch": map[string]any{"name": params.Head}},
			"destination": map[string]any{"branch": map[string]any{"name": base}},
		}
		data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/repositories/%s/%s/pullrequests", params.Owner, params.Repo), payload)
		return string(data), err
	}
	return "", fmt.Errorf("unsupported provider for create_pr: %s", c.provider)
}

func (c *hostingClient) createIssue(ctx context.Context, params HostingParams) (string, error) {
	if params.Title == "" {
		return "", fmt.Errorf("title is required for create_issue")
	}

	var payload map[string]any
	switch c.provider {
	case ProviderGitHub:
		payload = map[string]any{
			"title":     params.Title,
			"body":      params.Body,
			"labels":    params.Labels,
			"assignees": params.Assignees,
		}
		data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/repos/%s/%s/issues", params.Owner, params.Repo), payload)
		return string(data), err
	case ProviderGitLab:
		payload = map[string]any{
			"title":       params.Title,
			"description": params.Body,
			"labels":      strings.Join(params.Labels, ","),
		}
		data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/projects/%s%%2F%s/issues", params.Owner, params.Repo), payload)
		return string(data), err
	case ProviderBitbucket:
		payload = map[string]any{
			"title":   params.Title,
			"content": map[string]any{"raw": params.Body},
		}
		data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/repositories/%s/%s/issues", params.Owner, params.Repo), payload)
		return string(data), err
	}
	return "", fmt.Errorf("unsupported provider for create_issue: %s", c.provider)
}

func (c *hostingClient) listIssues(ctx context.Context, params HostingParams) (string, error) {
	state := params.State
	if state == "" {
		state = "open"
	}

	path := fmt.Sprintf("/repos/%s/%s/issues?state=%s", params.Owner, params.Repo, state)
	if c.provider == ProviderGitLab {
		path = fmt.Sprintf("/projects/%s%%2F%s/issues?state=%s", params.Owner, params.Repo, state)
	} else if c.provider == ProviderBitbucket {
		path = fmt.Sprintf("/repositories/%s/%s/issues?state=%s", params.Owner, params.Repo, state)
	}

	data, err := c.doRequest(ctx, "GET", path, nil)
	return string(data), err
}

func (c *hostingClient) listPRs(ctx context.Context, params HostingParams) (string, error) {
	state := params.State
	if state == "" {
		state = "open"
	}

	var path string
	switch c.provider {
	case ProviderGitHub:
		path = fmt.Sprintf("/repos/%s/%s/pulls?state=%s", params.Owner, params.Repo, state)
	case ProviderGitLab:
		path = fmt.Sprintf("/projects/%s%%2F%s/merge_requests?state=%s", params.Owner, params.Repo, state)
	case ProviderBitbucket:
		path = fmt.Sprintf("/repositories/%s/%s/pullrequests?state=%s", params.Owner, params.Repo, state)
	}

	data, err := c.doRequest(ctx, "GET", path, nil)
	return string(data), err
}

func (c *hostingClient) getPR(ctx context.Context, params HostingParams) (string, error) {
	if params.Number == 0 {
		return "", fmt.Errorf("number is required for get_pr")
	}

	var path string
	switch c.provider {
	case ProviderGitHub:
		path = fmt.Sprintf("/repos/%s/%s/pulls/%d", params.Owner, params.Repo, params.Number)
	case ProviderGitLab:
		path = fmt.Sprintf("/projects/%s%%2F%s/merge_requests/%d", params.Owner, params.Repo, params.Number)
	case ProviderBitbucket:
		path = fmt.Sprintf("/repositories/%s/%s/pullrequests/%d", params.Owner, params.Repo, params.Number)
	}

	data, err := c.doRequest(ctx, "GET", path, nil)
	return string(data), err
}

func (c *hostingClient) getIssue(ctx context.Context, params HostingParams) (string, error) {
	if params.Number == 0 {
		return "", fmt.Errorf("number is required for get_issue")
	}

	var path string
	switch c.provider {
	case ProviderGitHub:
		path = fmt.Sprintf("/repos/%s/%s/issues/%d", params.Owner, params.Repo, params.Number)
	case ProviderGitLab:
		path = fmt.Sprintf("/projects/%s%%2F%s/issues/%d", params.Owner, params.Repo, params.Number)
	case ProviderBitbucket:
		path = fmt.Sprintf("/repositories/%s/%s/issues/%d", params.Owner, params.Repo, params.Number)
	}

	data, err := c.doRequest(ctx, "GET", path, nil)
	return string(data), err
}

func (c *hostingClient) mergePR(ctx context.Context, params HostingParams) (string, error) {
	if params.Number == 0 {
		return "", fmt.Errorf("number is required for merge_pr")
	}

	var path string
	var payload map[string]any
	switch c.provider {
	case ProviderGitHub:
		path = fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", params.Owner, params.Repo, params.Number)
		payload = map[string]any{
			"commit_title": params.Title,
			"merge_method": "merge",
		}
	case ProviderGitLab:
		path = fmt.Sprintf("/projects/%s%%2F%s/merge_requests/%d/merge", params.Owner, params.Repo, params.Number)
		payload = map[string]any{
			"merge_commit_message": params.Title,
		}
	case ProviderBitbucket:
		path = fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/merge", params.Owner, params.Repo, params.Number)
		payload = map[string]any{
			"message": params.Title,
		}
	}

	data, err := c.doRequest(ctx, "PUT", path, payload)
	return string(data), err
}

func (c *hostingClient) closeIssue(ctx context.Context, params HostingParams) (string, error) {
	if params.Number == 0 {
		return "", fmt.Errorf("number is required for close_issue")
	}

	var path string
	var payload map[string]any
	switch c.provider {
	case ProviderGitHub:
		path = fmt.Sprintf("/repos/%s/%s/issues/%d", params.Owner, params.Repo, params.Number)
		payload = map[string]any{"state": "closed"}
	case ProviderGitLab:
		path = fmt.Sprintf("/projects/%s%%2F%s/issues/%d", params.Owner, params.Repo, params.Number)
		payload = map[string]any{"state_event": "close"}
	case ProviderBitbucket:
		path = fmt.Sprintf("/repositories/%s/%s/issues/%d", params.Owner, params.Repo, params.Number)
		payload = map[string]any{"state": "resolved"}
	}

	data, err := c.doRequest(ctx, "PATCH", path, payload)
	return string(data), err
}

func (c *hostingClient) addLabels(ctx context.Context, params HostingParams) (string, error) {
	if params.Number == 0 {
		return "", fmt.Errorf("number is required for add_labels")
	}
	if len(params.Labels) == 0 {
		return "", fmt.Errorf("labels are required for add_labels")
	}

	var path string
	var payload map[string]any
	switch c.provider {
	case ProviderGitHub:
		path = fmt.Sprintf("/repos/%s/%s/issues/%d/labels", params.Owner, params.Repo, params.Number)
		payload = map[string]any{"labels": params.Labels}
	case ProviderGitLab:
		path = fmt.Sprintf("/projects/%s%%2F%s/issues/%d", params.Owner, params.Repo, params.Number)
		payload = map[string]any{"labels": strings.Join(params.Labels, ",")}
	case ProviderBitbucket:
		return "", fmt.Errorf("add_labels not supported for Bitbucket")
	}

	data, err := c.doRequest(ctx, "POST", path, payload)
	return string(data), err
}

func (c *hostingClient) assignIssue(ctx context.Context, params HostingParams) (string, error) {
	if params.Number == 0 {
		return "", fmt.Errorf("number is required for assign_issue")
	}
	if len(params.Assignees) == 0 {
		return "", fmt.Errorf("assignees are required for assign_issue")
	}

	var path string
	var payload map[string]any
	switch c.provider {
	case ProviderGitHub:
		path = fmt.Sprintf("/repos/%s/%s/issues/%d/assignees", params.Owner, params.Repo, params.Number)
		payload = map[string]any{"assignees": params.Assignees}
	case ProviderGitLab:
		path = fmt.Sprintf("/projects/%s%%2F%s/issues/%d", params.Owner, params.Repo, params.Number)
		payload = map[string]any{"assignee_ids": params.Assignees}
	case ProviderBitbucket:
		return "", fmt.Errorf("assign_issue not supported for Bitbucket")
	}

	data, err := c.doRequest(ctx, "POST", path, payload)
	return string(data), err
}
