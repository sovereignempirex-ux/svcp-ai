package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHubCLITool_Info(t *testing.T) {
	info := NewGitHubCLITool(nil).Info()

	if info.Name != GitHubCLIToolName {
		t.Errorf("expected name %q, got %q", GitHubCLIToolName, info.Name)
	}
	if info.Description == "" {
		t.Error("description should not be empty")
	}
	if !containsString(info.Parameters["required"].([]string), "command") {
		t.Error("command should be required")
	}
}

func TestGitLabCLITool_Info(t *testing.T) {
	info := NewGitLabCLITool(nil).Info()
	if info.Name != GitLabCLIToolName {
		t.Errorf("expected name %q, got %q", GitLabCLIToolName, info.Name)
	}
}

func TestCLITool_Info(t *testing.T) {
	// Both tools share one implementation, so a table covers them both.
	cases := []struct {
		name string
		tool BaseTool
	}{
		{"github", NewGitHubCLITool(nil)},
		{"gitlab", NewGitLabCLITool(nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			props := tc.tool.Info().Parameters["properties"].(map[string]any)
			for _, want := range []string{"command", "args", "repo"} {
				if props[want] == nil {
					t.Errorf("missing property: %s", want)
				}
			}
		})
	}
}

func TestCLITool_Run_Validation(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"invalid json", `{invalid json`},
		{"missing command", `{}`},
		{"blank command", `{"command": "  "}`},
	}

	tool := NewGitHubCLITool(nil)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tool.Run(context.Background(), ToolCall{
				ID: "test", Name: GitHubCLIToolName, Input: tc.input,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.IsError {
				t.Error("expected an error response")
			}
		})
	}
}

func TestWebhookTool_Info(t *testing.T) {
	info := NewWebhookTool(nil).Info()

	if info.Name != WebhookToolName {
		t.Errorf("expected name %q, got %q", WebhookToolName, info.Name)
	}
	props := info.Parameters["properties"].(map[string]any)
	for _, want := range []string{"provider", "action", "owner", "repo", "url", "events", "secret", "id"} {
		if props[want] == nil {
			t.Errorf("missing property: %s", want)
		}
	}
	enum := props["provider"].(map[string]any)["enum"].([]string)
	for _, want := range []string{"github", "gitlab", "bitbucket"} {
		if !containsString(enum, want) {
			t.Errorf("missing provider in enum: %s", want)
		}
	}
}

func TestWebhookTool_Run_Validation(t *testing.T) {
	// Without a token the tool must refuse before touching the network, so
	// these cases are safe to run offline.
	cases := []struct {
		name  string
		input string
	}{
		{"invalid json", `{invalid json`},
		{"missing owner", `{"provider":"github","action":"list","repo":"r"}`},
		{"missing repo", `{"provider":"github","action":"list","owner":"o"}`},
		{"create without a url", `{"provider":"github","action":"create","owner":"o","repo":"r","token":"t"}`},
		{"delete without an id", `{"provider":"github","action":"delete","owner":"o","repo":"r","token":"t"}`},
	}

	tool := NewWebhookTool(nil)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tool.Run(context.Background(), ToolCall{
				ID: "test", Name: WebhookToolName, Input: tc.input,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.IsError {
				t.Errorf("expected an error response, got: %s", res.Content)
			}
		})
	}
}

func TestDefaultAPIURL(t *testing.T) {
	cases := map[string]string{
		"github":    "https://api.github.com",
		"gitlab":    "https://gitlab.com/api/v4",
		"bitbucket": "https://api.bitbucket.org/2.0",
	}
	for provider, want := range cases {
		if got := defaultAPIURL(provider); got != want {
			t.Errorf("defaultAPIURL(%q) = %q, want %q", provider, got, want)
		}
	}
	if defaultAPIURL("sourcehut") != "" {
		t.Error("an unknown provider should have no default endpoint")
	}
}

func TestTokenForProviderReadsEnvironment(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "gh-token")
	if got := tokenForProvider("github"); got != "gh-token" {
		t.Errorf("tokenForProvider(github) = %q", got)
	}

	t.Setenv("GITLAB_TOKEN", "gl-token")
	if got := tokenForProvider("gitlab"); got != "gl-token" {
		t.Errorf("tokenForProvider(gitlab) = %q", got)
	}

	if got := tokenForProvider("nonsense"); got != "" {
		t.Errorf("an unknown provider should have no token, got %q", got)
	}
}

func TestCurlArgs(t *testing.T) {
	p := webhookParams{
		Owner: "acme", Repo: "app",
		URL:    "https://example.com/hook",
		Events: []string{"push", "pull_request"},
		Secret: "s3cret",
		ID:     42,
	}

	t.Run("github create", func(t *testing.T) {
		args := curlArgs("github", "https://api.github.com", p, "tok", "create")
		assertCurl(t, args, "POST", "https://api.github.com/repos/acme/app/hooks")

		body := curlBody(args)
		if !strings.Contains(body, "https://example.com/hook") {
			t.Errorf("body missing the target url: %s", body)
		}
		if !strings.Contains(body, `"push"`) || !strings.Contains(body, `"pull_request"`) {
			t.Errorf("body missing events: %s", body)
		}
		if !strings.Contains(body, "s3cret") {
			t.Errorf("body missing the signing secret: %s", body)
		}
		if !containsString(args, "Authorization: Bearer tok") {
			t.Errorf("missing bearer auth: %v", args)
		}
	})

	t.Run("github list", func(t *testing.T) {
		args := curlArgs("github", "https://api.github.com", p, "tok", "list")
		if !containsString(args, "GET") {
			t.Errorf("list should be a GET: %v", args)
		}
		if args[len(args)-1] != "https://api.github.com/repos/acme/app/hooks" {
			t.Errorf("collection endpoint expected, got %q", args[len(args)-1])
		}
	})

	t.Run("github ping targets the pings subresource", func(t *testing.T) {
		args := curlArgs("github", "https://api.github.com", p, "tok", "ping")
		if !strings.HasSuffix(args[len(args)-1], "/hooks/42/pings") {
			t.Errorf("unexpected endpoint %q", args[len(args)-1])
		}
	})

	t.Run("github delete", func(t *testing.T) {
		args := curlArgs("github", "https://api.github.com", p, "tok", "delete")
		if !containsString(args, "DELETE") {
			t.Errorf("delete should be a DELETE: %v", args)
		}
	})

	t.Run("gitlab uses its own auth header and escaped path", func(t *testing.T) {
		args := curlArgs("gitlab", "https://gitlab.com/api/v4", p, "tok", "list")
		if !containsString(args, "Authorization: PRIVATE-TOKEN tok") {
			t.Errorf("gitlab needs a PRIVATE-TOKEN header: %v", args)
		}
		// The slash in the project path has to be percent-encoded.
		if args[len(args)-1] != "https://gitlab.com/api/v4/projects/acme%2Fapp/hooks" {
			t.Errorf("unexpected endpoint %q", args[len(args)-1])
		}
	})

	t.Run("bitbucket", func(t *testing.T) {
		args := curlArgs("bitbucket", "https://api.bitbucket.org/2.0", p, "tok", "delete")
		if args[len(args)-1] != "https://api.bitbucket.org/2.0/repositories/acme/app/hooks/42" {
			t.Errorf("unexpected endpoint %q", args[len(args)-1])
		}
	})

	t.Run("a self-hosted endpoint is respected", func(t *testing.T) {
		p.APIURL = "https://git.example.com/api/v4/"
		args := curlArgs("gitlab", p.APIURL, p, "tok", "list")
		if !strings.HasPrefix(args[len(args)-1], "https://git.example.com/api/v4/projects/") {
			t.Errorf("the custom endpoint should be used, got %q", args[len(args)-1])
		}
		// A trailing slash must not produce a double slash.
		if strings.Contains(args[len(args)-1], "//projects") {
			t.Errorf("double slash in %q", args[len(args)-1])
		}
	})
}

func TestGitHubHookBodyDefaultsEvents(t *testing.T) {
	// GitHub rejects an empty events array, so a default is required.
	body := gitHubHookBody(webhookParams{URL: "https://x/y"})
	if !strings.Contains(body, `"push"`) {
		t.Errorf("expected a default event list, got %s", body)
	}

	// Without a secret the config object must still be valid.
	body = gitHubHookBody(webhookParams{URL: "https://x/y", Events: []string{"release"}})
	if strings.Contains(body, "secret") {
		t.Errorf("no secret was given, so none should appear: %s", body)
	}
	if strings.Count(body, `"config"`) != 1 {
		t.Errorf("the config object appears more than once: %s", body)
	}
}

// assertCurl checks the method and the endpoint a curl invocation targets.
func assertCurl(t *testing.T, args []string, method, endpoint string) {
	t.Helper()
	if !containsString(args, method) {
		t.Errorf("expected method %s in %v", method, args)
	}
	if args[len(args)-1] != endpoint {
		t.Errorf("endpoint = %q, want %q", args[len(args)-1], endpoint)
	}
}

// curlBody extracts the -d payload from a curl argument list.
func curlBody(args []string) string {
	for i, a := range args {
		if a == "-d" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestQuoteJoin(t *testing.T) {
	got := quoteJoin([]string{"push", "issues"})
	if got != `"push","issues"` {
		t.Errorf("quoteJoin = %q", got)
	}
	if quoteJoin(nil) != "" {
		t.Error("an empty list should render as an empty string")
	}
}

func TestGitHubWorkflowTool_InfoRequiresTriggers(t *testing.T) {
	info := NewGitHubWorkflowTool(nil).Info()
	for _, want := range []string{"name", "on", "jobs"} {
		if !containsString(info.Parameters["required"].([]string), want) {
			t.Errorf("missing required parameter: %s", want)
		}
	}
	if info.Parameters["properties"].(map[string]any)["on"] == nil {
		t.Error("the trigger property is missing")
	}
}

func TestGitHubWorkflowTool_RejectsPathTraversal(t *testing.T) {
	tool := NewGitHubWorkflowTool(nil)

	for _, name := range []string{`../../evil.yml`, `sub/dir.yml`, `..\\evil.yml`} {
		res, err := tool.Run(context.Background(), ToolCall{
			ID:    "test",
			Name:  GitHubWorkflowToolName,
			Input: `{"name":` + quote(name) + `,"on":{"push":{}},"jobs":{"b":{"runs-on":"x"}}}`,
		})
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", name, err)
		}
		if !res.IsError {
			t.Errorf("name %q should have been rejected", name)
		}
	}
}

func TestGitHubWorkflowTool_RequiresName(t *testing.T) {
	res, err := NewGitHubWorkflowTool(nil).Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  GitHubWorkflowToolName,
		Input: `{"on":{"push":{}},"jobs":{}}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("expected an error response when the name is missing")
	}
}

func TestGitHubWorkflowTool_WritesIntoTheRepository(t *testing.T) {
	// A real temporary git repository, so the tool's root detection is
	// exercised rather than stubbed.
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")

	tool := NewGitHubWorkflowTool(nil)
	res, err := tool.Run(context.Background(), ToolCall{
		ID:   "test",
		Name: GitHubWorkflowToolName,
		Input: `{"project_path":` + quote(dir) + `,"name":"ci.yml",
			"on":{"push":{"branches":["main"]}},
			"jobs":{"build":{"runs-on":"ubuntu-latest"}}}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error response: %s", res.Content)
	}

	written := filepath.Join(dir, ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(written)
	if err != nil {
		t.Fatalf("reading the workflow: %v", err)
	}
	if !strings.Contains(string(data), "runs-on") {
		t.Errorf("workflow body missing:\n%s", data)
	}
}

// runGit runs a git command and fails the test if it does not succeed.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if _, err := lookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	res, err := newRunner(nil).execIn(context.Background(), dir, "git", args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("git %v exited %d: %s", args, res.ExitCode, res.String())
	}
}
