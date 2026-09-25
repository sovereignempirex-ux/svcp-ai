package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/svpc-ai/svpc/internal/permission"
	"github.com/svpc-ai/svpc/internal/pubsub"
)

func TestHostingTool_Info(t *testing.T) {
	info := NewHostingTool(nil).Info()

	if info.Name != HostingToolName {
		t.Errorf("expected name %q, got %q", HostingToolName, info.Name)
	}
	if info.Description == "" {
		t.Error("description should not be empty")
	}
	if info.Parameters == nil {
		t.Fatal("parameters should not be nil")
	}

	props := info.Parameters["properties"].(map[string]any)
	for _, want := range []string{
		"provider", "action", "owner", "repo", "title", "body", "number", "labels", "assignees",
	} {
		if props[want] == nil {
			t.Errorf("missing property: %s", want)
		}
	}
}

func TestHostingTool_Run_Validation(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"invalid json", `{invalid json`},
		{"missing owner", `{"provider":"github","action":"list_issues","repo":"r"}`},
		{"missing repo", `{"provider":"github","action":"list_issues","owner":"o"}`},
		{"unknown action", `{"provider":"github","action":"launch_missiles","owner":"o","repo":"r","token":"t"}`},
	}

	tool := NewHostingTool(nil)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tool.Run(context.Background(), ToolCall{
				ID: "test", Name: HostingToolName, Input: tc.input,
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

func TestHostingTool_Run_MissingToken(t *testing.T) {
	// Clearing the environment proves the tool refuses rather than sending an
	// unauthenticated request.
	t.Setenv("GITHUB_TOKEN", "")

	res, err := NewHostingTool(nil).Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  HostingToolName,
		Input: `{"provider":"github","action":"list_issues","owner":"o","repo":"r"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected an error response when no token is configured")
	}
	if !strings.Contains(res.Content, "token") {
		t.Errorf("the error should name the missing credential, got %q", res.Content)
	}
}

func TestMutatingHostingAction(t *testing.T) {
	// A new action must be denied by default, so this is an allow-list.
	for _, action := range []string{
		"create_pr", "create_issue", "merge_pr", "close_issue", "add_labels", "assign_issue",
	} {
		if !mutatingHostingAction(action) {
			t.Errorf("%q changes state and must ask first", action)
		}
	}
	for _, action := range []string{
		"list_issues", "list_prs", "get_pr", "get_issue", "something_new",
	} {
		if mutatingHostingAction(action) {
			t.Errorf("%q should not ask", action)
		}
	}
}

func TestDescribeHostingAction(t *testing.T) {
	cases := []struct {
		action string
		params HostingParams
		want   []string
	}{
		{"create_pr", HostingParams{Owner: "acme", Repo: "app", Title: "Fix the parser"},
			[]string{"pull request", "acme/app", "Fix the parser"}},
		{"create_issue", HostingParams{Owner: "acme", Repo: "app", Title: "Crash on start"},
			[]string{"issue", "acme/app", "Crash on start"}},
		{"merge_pr", HostingParams{Owner: "acme", Repo: "app", Number: 42},
			[]string{"Merge", "#42"}},
		{"close_issue", HostingParams{Owner: "acme", Repo: "app", Number: 7},
			[]string{"Close", "#7"}},
		{"add_labels", HostingParams{Owner: "acme", Repo: "app", Number: 3, Labels: []string{"bug", "p1"}},
			[]string{"bug", "p1"}},
		{"assign_issue", HostingParams{Owner: "acme", Repo: "app", Number: 3, Assignees: []string{"ana"}},
			[]string{"ana"}},
	}

	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			got := describeHostingAction(ProviderGitHub, tc.action, tc.params)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("describeHostingAction = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

func TestDescribeHostingActionHandlesAnEmptyTitle(t *testing.T) {
	got := describeHostingAction(ProviderGitHub, "create_issue", HostingParams{Owner: "o", Repo: "r"})
	if !strings.Contains(got, "no title") {
		t.Errorf("an untitled issue should say so, got %q", got)
	}
}

func TestRedactHostingHidesTheToken(t *testing.T) {
	got := redactHosting(HostingParams{
		Provider: "github", Action: "create_issue",
		Owner: "acme", Repo: "app", Title: "Bug", Token: "ghp_secret",
	})
	if got["token"] != "***" {
		t.Errorf("the token must never be shown, got %v", got["token"])
	}
	// The non-secret fields are what make the prompt informative.
	if got["owner"] != "acme" || got["repo"] != "app" || got["title"] != "Bug" {
		t.Errorf("context was lost from the prompt: %v", got)
	}
}

// TestHostingAsksBeforeMutating proves a denied request never reaches the API.
func TestHostingAsksBeforeMutating(t *testing.T) {
	reached := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		_ = json.NewEncoder(w).Encode(map[string]any{"number": 1})
	}))
	defer server.Close()

	denied := NewHostingTool(denyPermissions{})
	res, err := denied.Run(context.Background(), ToolCall{
		ID:   "test",
		Name: HostingToolName,
		Input: `{"provider":"github","action":"create_issue","owner":"o","repo":"r",` +
			`"title":"T","token":"t","api_url":` + quote(server.URL) + `}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("a denied action must report an error")
	}
	if reached {
		t.Error("the request reached the API despite being denied")
	}
}

func TestHostingDoesNotAskForReads(t *testing.T) {
	// The read is allowed, so a recorded request proves no prompt was needed.
	var asked bool
	recorder := &recordingPermissions{asked: &asked}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"number": 1, "title": "T"}})
	}))
	defer server.Close()

	res, err := NewHostingTool(recorder).Run(context.Background(), ToolCall{
		ID:   "test",
		Name: HostingToolName,
		Input: `{"provider":"github","action":"list_issues","owner":"o","repo":"r",` +
			`"token":"t","api_url":` + quote(server.URL) + `}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error response: %s", res.Content)
	}
	if asked {
		t.Error("listing issues is a read and should not interrupt the user")
	}
}

func TestHostingSendsTheTokenAsBearer(t *testing.T) {
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode([]map[string]any{{"number": 1}})
	}))
	defer server.Close()

	_, err := NewHostingTool(nil).Run(context.Background(), ToolCall{
		ID:   "test",
		Name: HostingToolName,
		Input: `{"provider":"github","action":"list_issues","owner":"o","repo":"r",` +
			`"token":"secret-token","api_url":` + quote(server.URL) + `}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if auth != "Bearer secret-token" {
		t.Errorf("Authorization = %q", auth)
	}
}

// TestDoRequestWithNoBody guards the read path. A nil *strings.Reader handed to
// http.NewRequest satisfies the io.Reader type switch and then panics on Len,
// which is what made every read through this client crash the agent.
func TestDoRequestWithNoBody(t *testing.T) {
	var gotMethod, gotBody, gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := &hostingClient{
		httpClient: server.Client(),
		baseURL:    server.URL,
		token:      "t",
		provider:   ProviderGitHub,
	}

	raw, err := client.doRequest(context.Background(), http.MethodGet, "/repos/o/r/issues", nil)
	if err != nil {
		t.Fatalf("a body-less request must not fail: %v", err)
	}
	if !strings.Contains(string(raw), `"ok"`) {
		t.Errorf("response = %q", raw)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q", gotMethod)
	}
	if gotBody != "" {
		t.Errorf("a GET must not send a body, got %q", gotBody)
	}
	if gotContentType != "" {
		t.Errorf("a body-less request must not claim a content type, got %q", gotContentType)
	}
}

func TestDoRequestSendsAJSONBody(t *testing.T) {
	var gotBody, gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := &hostingClient{
		httpClient: server.Client(),
		baseURL:    server.URL,
		token:      "t",
		provider:   ProviderGitHub,
	}

	if _, err := client.doRequest(context.Background(), http.MethodPost, "/repos/o/r/issues",
		map[string]any{"title": "Bug"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q", gotContentType)
	}
	if !strings.Contains(gotBody, `"title":"Bug"`) {
		t.Errorf("body = %q", gotBody)
	}
}

func TestDoRequestReportsTheProvidersError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer server.Close()

	client := &hostingClient{
		httpClient: server.Client(),
		baseURL:    server.URL,
		token:      "t",
		provider:   ProviderGitHub,
	}

	_, err := client.doRequest(context.Background(), http.MethodGet, "/repos/o/r/issues", nil)
	if err == nil {
		t.Fatal("a 404 must be reported as an error")
	}
	// The provider's own message is far more useful than a bare status code.
	if !strings.Contains(err.Error(), "Not Found") {
		t.Errorf("error = %q, want the provider message", err)
	}
}

// Permission doubles
// ---------------------------------------------------------------------------

// denyPermissions refuses everything, so a test can prove that a refusal
// actually stops the work.
type denyPermissions struct{}

func (denyPermissions) GrantPersistant(permission.PermissionRequest) {}
func (denyPermissions) Grant(permission.PermissionRequest)           {}
func (denyPermissions) Deny(permission.PermissionRequest)            {}
func (denyPermissions) Shutdown()                                    {}
func (denyPermissions) AutoApproveSession(string)                    {}

func (denyPermissions) Subscribe(context.Context) <-chan pubsub.Event[permission.PermissionRequest] {
	return nil
}

func (denyPermissions) Request(permission.CreatePermissionRequest) bool { return false }

// recordingPermissions allows everything but notes that it was asked.
type recordingPermissions struct {
	asked *bool
}

func (r *recordingPermissions) GrantPersistant(permission.PermissionRequest) {}
func (r *recordingPermissions) Grant(permission.PermissionRequest)           {}
func (r *recordingPermissions) Deny(permission.PermissionRequest)            {}
func (r *recordingPermissions) Shutdown()                                    {}
func (r *recordingPermissions) AutoApproveSession(string)                    {}

func (r *recordingPermissions) Subscribe(context.Context) <-chan pubsub.Event[permission.PermissionRequest] {
	return nil
}

func (r *recordingPermissions) Request(permission.CreatePermissionRequest) bool {
	*r.asked = true
	return true
}
