package gui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/svpc-ai/svpc/internal/app"
	"github.com/svpc-ai/svpc/internal/llm/tools"
	"github.com/svpc-ai/svpc/internal/message"
	"github.com/svpc-ai/svpc/internal/permission"
)

func TestToolSummaryPicksTheUsefulField(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"command", `{"command":"go build ./...","timeout":10}`, "go build ./..."},
		{"file", `{"file_path":"internal/tui/tui.go"}`, "internal/tui/tui.go"},
		{"pattern before path", `{"pattern":"**/*.go","path":"internal"}`, "**/*.go"},
		{"url", `{"url":"https://example.com"}`, "https://example.com"},
		{"provider and action", `{"provider":"github","action":"create_issue"}`, "github create_issue"},
		{"empty", `{}`, ""},
		{"invalid", `not json`, "not json"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toolSummary(message.ToolCall{Name: "x", Input: tc.input})
			if got != tc.want {
				t.Errorf("toolSummary = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestToolSummaryTruncates(t *testing.T) {
	long := strings.Repeat("a", 300)
	got := toolSummary(message.ToolCall{Name: "bash", Input: `{"command":"` + long + `"}`})
	if len([]rune(got)) > 80 {
		t.Errorf("summary not truncated: %d runes", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected an ellipsis, got %q", got)
	}
}

func TestPrettyToolName(t *testing.T) {
	cases := map[string]string{
		tools.BashToolName:    "Bash",
		tools.EditToolName:    "Edit",
		tools.ViewToolName:    "View",
		tools.BuildToolName:   "Build",
		tools.HostingToolName: "Hosting",
		"something_new":       "something_new",
	}
	for in, want := range cases {
		if got := prettyToolName(in); got != want {
			t.Errorf("prettyToolName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("a\nb\r\nc"); got != "a" {
		t.Errorf("firstLine = %q, want %q", got, "a")
	}
	if got := firstLine("single"); got != "single" {
		t.Errorf("firstLine = %q, want %q", got, "single")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("got %q", got)
	}
	got := truncate(strings.Repeat("x", 50), 10)
	if len([]rune(got)) != 10 {
		t.Errorf("length = %d runes, want 10", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected an ellipsis, got %q", got)
	}
	// Multi-byte input must not be split mid-rune.
	if got := truncate("αααααααααα", 5); len([]rune(got)) != 5 {
		t.Errorf("rune-aware length = %d, want 5", len([]rune(got)))
	}
}

func TestNewBridgeWithoutCore(t *testing.T) {
	b := newBridge(nil, nil, nil, errFake{})
	if err := b.ready(); err == nil {
		t.Fatal("expected ready() to fail when the core is missing")
	}
	if b.setupErr == "" {
		t.Error("setupErr should record why the core is unavailable")
	}
}

type errFake struct{}

func (errFake) Error() string { return "no provider configured" }

// ---------------------------------------------------------------------------
// End-to-end HTTP
// ---------------------------------------------------------------------------

// startServer brings up the loopback bridge in setup mode — the state a first
// run lands in, with no provider configured — and returns its base URL.
func startServer(t *testing.T) string {
	t.Helper()

	base, shutdown, err := serve(context.Background(), nil, nil, errFake{})
	if err != nil {
		t.Fatalf("starting the bridge: %v", err)
	}
	t.Cleanup(shutdown)
	return base
}

// get performs a request against the bridge and returns status and body.
func get(t *testing.T, method, url, body string) (int, string) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}

	// A fresh client per call: the bridge streams, so a shared one would hold
	// the connection open.
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response: %v", err)
	}
	return resp.StatusCode, string(raw)
}

func TestServeServesTheWindow(t *testing.T) {
	base := startServer(t)

	for _, asset := range []string{"", "index.html", "app.css", "app.js", "svpc.ico"} {
		status, body := get(t, http.MethodGet, base+asset, "")
		if status != http.StatusOK {
			t.Errorf("GET %q = %d, want 200", asset, status)
		}
		if len(body) == 0 {
			t.Errorf("GET %q returned nothing", asset)
		}
	}
}

func TestIndexReferencesItsAssets(t *testing.T) {
	base := startServer(t)

	_, body := get(t, http.MethodGet, base+"index.html", "")
	for _, want := range []string{"app.css", "app.js", "svpc.ico"} {
		if !strings.Contains(body, want) {
			t.Errorf("index.html does not reference %s", want)
		}
	}
}

func TestModelsReportsSetupState(t *testing.T) {
	base := startServer(t)

	status, body := get(t, http.MethodGet, base+"api/models", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decoding %q: %v", body, err)
	}
	// A first run must be able to tell the window it is not ready yet.
	if ready, _ := payload["ready"].(bool); ready {
		t.Error("ready should be false before a provider is configured")
	}
	if payload["setup_error"] == nil {
		t.Error("the reason the core is unavailable should be reported")
	}
}

func TestSessionsEmptyBeforeSetup(t *testing.T) {
	base := startServer(t)

	status, body := get(t, http.MethodGet, base+"api/sessions", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !strings.Contains(body, `"sessions":[]`) {
		t.Errorf("expected an empty list, got %q", body)
	}
}

func TestChatStreamsAnErrorEvent(t *testing.T) {
	base := startServer(t)

	// The stream is opened before anything can fail, so a misconfigured core
	// still produces a well-formed event rather than a bare HTTP error.
	status, body := get(t, http.MethodPost, base+"api/chat", `{"content":"hello"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !strings.HasPrefix(body, "data: ") {
		t.Errorf("expected a server-sent event, got %q", body)
	}

	var payload map[string]any
	line := strings.TrimPrefix(strings.SplitN(body, "\n", 2)[0], "data: ")
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		t.Fatalf("decoding %q: %v", line, err)
	}
	if payload["error"] == nil {
		t.Errorf("expected an error event, got %v", payload)
	}
}

func TestChatRejectsBadRequests(t *testing.T) {
	base := startServer(t)

	cases := []struct {
		name   string
		method string
		body   string
		want   int
	}{
		{"wrong method", http.MethodGet, "", http.StatusMethodNotAllowed},
		{"invalid json", http.MethodPost, `{invalid`, http.StatusBadRequest},
		{"empty prompt", http.MethodPost, `{"content":"   "}`, http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, _ := get(t, tc.method, base+"api/chat", tc.body)
			if status != tc.want {
				t.Errorf("status = %d, want %d", status, tc.want)
			}
		})
	}
}

func TestEmbeddedIconIsAValidICO(t *testing.T) {
	if len(Icon) == 0 {
		t.Fatal("the application icon is not embedded")
	}
	// Reserved zero, type 1 (icon), then the image count.
	if Icon[0] != 0 || Icon[1] != 0 || Icon[2] != 1 || Icon[3] != 0 {
		t.Errorf("bad ICONDIR header: %v", Icon[:4])
	}
	if count := int(Icon[4]) | int(Icon[5])<<8; count == 0 {
		t.Error("the icon declares no images")
	}
}

// ---------------------------------------------------------------------------
// Approvals
// ---------------------------------------------------------------------------

func TestPermissionRejectsBadRequests(t *testing.T) {
	base := startServer(t)

	cases := []struct {
		name   string
		method string
		body   string
		want   int
	}{
		{"wrong method", http.MethodGet, "", http.StatusMethodNotAllowed},
		{"invalid json", http.MethodPost, `{invalid`, http.StatusBadRequest},
		{"missing id", http.MethodPost, `{"action":"allow"}`, http.StatusBadRequest},
		// The core is not running in setup mode, so nothing can be approved.
		{"no core", http.MethodPost, `{"id":"x","action":"allow"}`, http.StatusServiceUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, _ := get(t, tc.method, base+"api/permission", tc.body)
			if status != tc.want {
				t.Errorf("status = %d, want %d", status, tc.want)
			}
		})
	}
}

func TestToPermissionRequest(t *testing.T) {
	t.Run("carries the fields the window needs", func(t *testing.T) {
		got, ok := toPermissionRequest(permission.PermissionRequest{
			ID: "id-1", ToolName: "bash", Action: "execute",
			Description: "go test ./...", Path: "C:/repo",
		})
		if !ok {
			t.Fatal("a request with an id should be forwarded")
		}
		if got.ID != "id-1" || got.ToolName != "bash" || got.Action != "execute" {
			t.Errorf("fields were not carried over: %+v", got)
		}
		// The description is what the card shows, so it has to survive intact.
		if got.Description != "go test ./..." {
			t.Errorf("description = %q", got.Description)
		}
		if got.Diff != "" {
			t.Errorf("no file tool was involved, so there is no diff: %q", got.Diff)
		}
	})

	t.Run("pulls the diff out of the file tools", func(t *testing.T) {
		got, _ := toPermissionRequest(permission.PermissionRequest{
			ID: "id-2", Params: tools.WritePermissionsParams{
				FilePath: "a.go", Diff: "--- a\n+++ b\n",
			},
		})
		if got.Diff != "--- a\n+++ b\n" {
			t.Errorf("diff = %q", got.Diff)
		}
	})

	t.Run("a request without an id is dropped", func(t *testing.T) {
		if _, ok := toPermissionRequest(permission.PermissionRequest{ToolName: "bash"}); ok {
			t.Error("a request with no id cannot be answered, so it should be dropped")
		}
	})
}

func TestPendingApprovalRoundTrip(t *testing.T) {
	b := newBridge(context.Background(), nil, nil, errFake{})
	b.rememberPending(permission.PermissionRequest{
		ID: "abc", SessionID: "s1", ToolName: "bash", Action: "execute", Path: "C:/repo",
	})

	got, ok := b.takePending("abc")
	if !ok {
		t.Fatal("the stored request should be found")
	}
	// The permission service matches on these fields when granting for a
	// session, so losing any of them would silently change behaviour.
	if got.ID != "abc" || got.SessionID != "s1" || got.ToolName != "bash" ||
		got.Action != "execute" || got.Path != "C:/repo" {
		t.Errorf("the original request was not preserved: %+v", got)
	}

	// It is consumed, so a second answer is rejected rather than double-granted.
	if _, ok := b.takePending("abc"); ok {
		t.Error("a request should only be answerable once")
	}
}

func TestOnlyOneTurnAtATime(t *testing.T) {
	b := newBridge(context.Background(), nil, nil, errFake{})

	if !b.beginTurn() {
		t.Fatal("the first turn should be admitted")
	}
	// A second concurrent turn would leave approval requests with nowhere to go.
	if b.beginTurn() {
		t.Error("a second concurrent turn should be refused")
	}
	b.endTurn()
	if !b.beginTurn() {
		t.Error("the slot should be free once the turn finishes")
	}
}

func TestPermissionEventIsForwardedToTheWindow(t *testing.T) {
	// The bridge reads from b.perm, so a request published to the broker has to
	// arrive there. This is the path that used to be missing entirely, which
	// left the agent blocked forever.
	perms := permission.NewPermissionService()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := newBridge(ctx, nil, nil, nil)
	b.app = &app.App{Permissions: perms}
	b.watchPermissions()

	// Give the goroutine a moment to subscribe before publishing.
	time.Sleep(50 * time.Millisecond)

	released := make(chan bool, 1)
	go func() {
		released <- perms.Request(permission.CreatePermissionRequest{
			SessionID: "s1", ToolName: "bash", Description: "go build ./...",
		})
	}()

	var forwarded PermissionRequest
	select {
	case forwarded = <-b.perm:
		if forwarded.ToolName != "bash" {
			t.Errorf("tool name = %q", forwarded.ToolName)
		}
		if forwarded.Description != "go build ./..." {
			t.Errorf("description = %q", forwarded.Description)
		}
		if forwarded.ID == "" {
			t.Fatal("the request needs an id so the window can answer it")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no approval reached the window")
	}

	// The window answers with the id it was shown, exactly as the HTTP handler
	// resolves it back to the original request.
	pending, ok := b.takePending(forwarded.ID)
	if !ok {
		t.Fatal("the request the window was shown must be answerable")
	}
	perms.Grant(pending)

	select {
	case granted := <-released:
		if !granted {
			t.Error("the tool should have been allowed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the tool is still blocked after the window answered")
	}
}

func TestPermissionRequestTimesOutRatherThanBlocking(t *testing.T) {
	// requestTimeout is minutes long, so the wait is exercised through the
	// shutdown path instead: an abandoned request must not wedge the tool.
	perms := permission.NewPermissionService()

	released := make(chan bool, 1)
	go func() {
		released <- perms.Request(permission.CreatePermissionRequest{
			SessionID: "s1", ToolName: "bash",
		})
	}()

	// Nobody has answered; shutting down must release the wait.
	time.Sleep(50 * time.Millisecond)
	perms.Shutdown()

	select {
	case granted := <-released:
		if granted {
			t.Error("an unanswered request must not be granted")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the tool stayed blocked after shutdown")
	}
}
