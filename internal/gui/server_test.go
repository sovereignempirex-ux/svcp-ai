package gui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/svpc-ai/svpc/internal/llm/tools"
	"github.com/svpc-ai/svpc/internal/message"
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
