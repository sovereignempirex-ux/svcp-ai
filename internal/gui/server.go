package gui

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/svpc-ai/svpc/internal/app"
	"github.com/svpc-ai/svpc/internal/config"
	"github.com/svpc-ai/svpc/internal/llm/agent"
	"github.com/svpc-ai/svpc/internal/llm/models"
	"github.com/svpc-ai/svpc/internal/llm/tools"
	"github.com/svpc-ai/svpc/internal/logging"
	"github.com/svpc-ai/svpc/internal/message"
)

//go:embed all:assets
var assets embed.FS

// Icon holds the application mark as an .ico, so the window and taskbar show
// the SVPC AI logo without any runtime file lookups.
var Icon []byte

func init() {
	if b, err := assets.ReadFile("assets/svpc.ico"); err == nil {
		Icon = b
	}
}

// ---------------------------------------------------------------------------
// Wire types
// ---------------------------------------------------------------------------

// Message is one turn of the conversation as the front-end sees it.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ToolCall describes a tool invocation, surfaced in the UI while it runs.
type ToolCall struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input string `json:"input,omitempty"`
	State string `json:"state"` // running | done | error
	Error string `json:"error,omitempty"`
}

// Config is the provider selection the UI sends with each request. Empty
// values fall back to the CLI configuration, so a user only sets what differs.
type Config struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
	BaseURL  string `json:"base_url"`
}

// ChatRequest is the payload posted by the UI for one turn.
type ChatRequest struct {
	SessionID string    `json:"session_id"`
	Content   string    `json:"content"`
	Config    Config    `json:"config"`
	History   []Message `json:"history"`
}

// ModelInfo describes one selectable model.
type ModelInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Context  int64  `json:"context_window"`
}

// ---------------------------------------------------------------------------
// Bridge
// ---------------------------------------------------------------------------

// bridge owns the application core and translates HTTP requests into agent
// runs, streaming progress back as server-sent events.
//
// The core can legitimately be nil: a first run has no provider configured
// yet, and the window still has to open so the user can supply one. In that
// case every endpoint reports the reason instead of failing opaquely.
type bridge struct {
	// ctx and db are kept so the core can be started later, once a provider has
	// been supplied from the window.
	ctx context.Context
	db  *sql.DB

	app *app.App
	// setupErr records why the core is unavailable, for display in the UI.
	setupErr string
	// coreReady flips once the agent is usable.
	coreReady bool

	mu       sync.Mutex
	sessions map[string]string // UI session id -> persisted session id
}

func newBridge(ctx context.Context, conn *sql.DB, a *app.App, setupErr error) *bridge {
	b := &bridge{ctx: ctx, db: conn, app: a, coreReady: a != nil, sessions: map[string]string{}}
	if setupErr != nil {
		b.setupErr = setupErr.Error()
	}
	return b
}

// ready reports whether a chat turn can be served.
func (b *bridge) ready() error {
	if b.app != nil {
		return nil
	}
	if b.setupErr == "" {
		b.setupErr = "no AI provider is configured"
	}
	return errors.New(b.setupErr)
}

// sessionID resolves (and lazily creates) the persisted session backing a UI
// conversation, so history survives a window reload.
func (b *bridge) sessionID(ctx context.Context, uiID, title string) (string, error) {
	if uiID == "" {
		uiID = "default"
	}

	b.mu.Lock()
	if id, ok := b.sessions[uiID]; ok {
		b.mu.Unlock()
		return id, nil
	}
	b.mu.Unlock()

	// Reuse the most recent session with the same UI id if the process was
	// restarted, otherwise start a fresh one.
	if list, err := b.app.Sessions.List(ctx); err == nil {
		for i := len(list) - 1; i >= 0; i-- {
			if strings.HasPrefix(list[i].Title, titlePrefix(uiID)) {
				b.mu.Lock()
				b.sessions[uiID] = list[i].ID
				b.mu.Unlock()
				return list[i].ID, nil
			}
		}
	}

	sess, err := b.app.Sessions.Create(ctx, titlePrefix(uiID)+" "+truncate(title, 60))
	if err != nil {
		return "", err
	}

	b.mu.Lock()
	b.sessions[uiID] = sess.ID
	b.mu.Unlock()
	return sess.ID, nil
}

func titlePrefix(uiID string) string { return "[gui:" + uiID + "]" }

// truncate shortens s to at most n runes, marking the cut with an ellipsis.
// The result never exceeds n, so callers can rely on it for layout.
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// ---------------------------------------------------------------------------
// HTTP handlers
// ---------------------------------------------------------------------------

// handleChat runs one turn through the real agent and streams the transcript:
// tool activity first, then the answer as it is produced.
func (b *bridge) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		http.Error(w, "empty prompt", http.StatusBadRequest)
		return
	}

	// Start streaming before anything can fail, so the UI always receives a
	// well-formed event stream rather than a bare HTTP error.
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flush(w, flusher)

	// Apply any provider override coming from the UI. The agent keeps using the
	// CLI configuration otherwise, which is what makes this a merge rather
	// than a second, competing client.
	if err := applyOverrides(req.Config); err != nil {
		writeEvent(w, map[string]any{"error": err.Error()})
		return
	}

	// A provider may have just been configured from the window, so the core is
	// booted lazily on the first successful turn.
	if b.app == nil {
		if err := bootCore(b); err != nil {
			writeEvent(w, map[string]any{"error": err.Error()})
			return
		}
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	sessionID, err := b.sessionID(ctx, req.SessionID, req.Content)
	if err != nil {
		writeEvent(w, map[string]any{"error": "session error: " + err.Error()})
		return
	}

	writeEvent(w, map[string]any{"session_id": sessionID})

	done, err := b.app.CoderAgent.Run(ctx, sessionID, req.Content)
	if err != nil {
		writeEvent(w, map[string]any{"error": err.Error()})
		return
	}

	b.pump(ctx, w, flusher, done, sessionID)
	writeEvent(w, map[string]any{"done": true})
}

// bootCore brings up the application core after the user has configured a
// provider from the window. It is safe to call repeatedly.
func bootCore(b *bridge) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.app != nil {
		return nil
	}

	// The core needs the session store; without it there is nothing to boot
	// against and app.New would dereference a nil handle.
	if b.db == nil {
		b.setupErr = "the session store is unavailable"
		return errors.New(b.setupErr)
	}
	if b.ctx == nil {
		b.ctx = context.Background()
	}

	core, err := app.New(b.ctx, b.db)
	if err != nil {
		b.setupErr = err.Error()
		return err
	}

	b.app = core
	b.setupErr = ""
	b.coreReady = true
	return nil
}

// pump translates agent messages into UI events until the run finishes.
func (b *bridge) pump(ctx context.Context, w http.ResponseWriter, flusher http.Flusher,
	done <-chan agent.AgentEvent, sessionID string) {

	emitted := map[string]bool{}

	for {
		select {
		case <-ctx.Done():
			return

		case ev, ok := <-done:
			if !ok {
				return
			}
			if ev.Error != nil {
				writeEvent(w, map[string]any{"error": ev.Error.Error()})
				return
			}

			// Tool activity is reported as it arrives so the UI can show what
			// the agent is actually doing.
			for _, call := range ev.Message.ToolCalls() {
				if emitted[call.ID] {
					continue
				}
				emitted[call.ID] = true
				state := "running"
				if call.Finished {
					state = "done"
				}
				writeEvent(w, map[string]any{
					"tool": ToolCall{ID: call.ID, Name: prettyToolName(call.Name),
						Input: toolSummary(call), State: state},
				})
				flush(w, flusher)
			}

			// A tool result can fail; surface it without killing the run.
			for _, res := range ev.Message.ToolResults() {
				if res.IsError {
					writeEvent(w, map[string]any{
						"tool_error": ToolCall{
							ID: res.ToolCallID, State: "error",
							Error: truncate(firstLine(res.Content), 200),
						},
					})
					flush(w, flusher)
				}
			}

			if text := ev.Message.Content().String(); text != "" {
				writeEvent(w, map[string]any{"text": text})
				flush(w, flusher)
			}
		}
	}
}

// handleModels reports the model the agent is running, plus whether the core
// has been started. The UI uses it to show setup state instead of guessing.
func (b *bridge) handleModels(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"ready": b.coreReady}
	if b.coreReady {
		if m := b.app.CoderAgent.Model(); m.ID != "" {
			out["current"] = ModelInfo{
				ID:       string(m.ID),
				Name:     m.Name,
				Provider: string(m.Provider),
				Context:  m.ContextWindow,
			}
		}
	}
	if b.setupErr != "" {
		out["setup_error"] = b.setupErr
	}
	_ = json.NewEncoder(w).Encode(out)
}

// handleSessions lists the stored conversations so the UI can offer a picker.
func (b *bridge) handleSessions(w http.ResponseWriter, r *http.Request) {
	if !b.coreReady {
		_ = json.NewEncoder(w).Encode(map[string]any{"sessions": []any{}})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	list, err := b.app.Sessions.List(ctx)
	if err != nil {
		http.Error(w, "cannot list sessions", http.StatusInternalServerError)
		return
	}

	type item struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		CreatedAt int64  `json:"created_at"`
	}

	items := make([]item, 0, len(list))
	for _, s := range list {
		items = append(items, item{ID: s.ID, Title: s.Title, CreatedAt: s.CreatedAt})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"sessions": items})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// applyOverrides writes provider selection into the shared configuration so the
// agent picks it up. Anything left empty is preserved, which is what lets the
// desktop client reuse whatever the terminal is already configured with.
func applyOverrides(c Config) error {
	if key := strings.TrimSpace(c.APIKey); key != "" {
		provider := strings.ToLower(strings.TrimSpace(c.Provider))
		if provider == "" {
			provider = "anthropic"
		}
		// A missing provider entry is not fatal: the key is still remembered for
		// the next start, and the agent reports the real problem when it runs.
		if err := config.UpdateProviderAPIKey(provider, key); err != nil {
			logging.Warn("Could not store provider key", "provider", provider, "error", err)
		}
	}

	if model := strings.TrimSpace(c.Model); model != "" {
		if err := config.UpdateAgentModel(config.AgentCoder, models.ModelID(model)); err != nil {
			return err
		}
	}
	return nil
}

// prettyToolName maps a tool identifier to the label shown in the UI.
func prettyToolName(name string) string {
	switch name {
	case tools.BashToolName:
		return "Bash"
	case tools.EditToolName:
		return "Edit"
	case tools.WriteToolName:
		return "Write"
	case tools.PatchToolName:
		return "Patch"
	case tools.ViewToolName:
		return "View"
	case tools.GlobToolName:
		return "Glob"
	case tools.GrepToolName:
		return "Grep"
	case tools.LSToolName:
		return "List"
	case tools.FetchToolName:
		return "Fetch"
	case tools.BuildToolName:
		return "Build"
	case tools.HostingToolName:
		return "Hosting"
	case tools.ImageGenToolName:
		return "Image"
	case tools.CloudToolName:
		return "Cloud"
	case tools.DockerToolName:
		return "Docker"
	case tools.KubernetesToolName:
		return "Kubernetes"
	case tools.CICDToolName:
		return "CI/CD"
	case tools.OpenToolName:
		return "Open"
	case agent.AgentToolName:
		return "Task"
	default:
		return name
	}
}

// toolSummary renders a short, human-readable hint about what a tool was asked
// to do, derived from its raw JSON input.
func toolSummary(call message.ToolCall) string {
	if strings.TrimSpace(call.Input) == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(call.Input), &payload); err != nil {
		return truncate(firstLine(call.Input), 80)
	}

	str := func(key string) string {
		if v, ok := payload[key].(string); ok {
			return strings.TrimSpace(v)
		}
		return ""
	}

	// Tools that act on a provider are summarised as "<provider> <action>":
	// either half alone loses the context.
	if provider, action := str("provider"), str("action"); provider != "" && action != "" {
		return truncate(provider+" "+action, 80)
	}

	// Otherwise the first field that describes the intent wins. Order matters:
	// a glob pattern says more than the directory it is rooted at.
	for _, key := range []string{
		"command", "file_path", "pattern", "path", "url", "query", "prompt", "action",
	} {
		if v := str(key); v != "" {
			return truncate(v, 80)
		}
	}
	return ""
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

func writeEvent(w http.ResponseWriter, payload any) {
	b, _ := json.Marshal(payload)
	fmt.Fprintf(w, "data: %s\n\n", b)
}

func flush(w http.ResponseWriter, f http.Flusher) {
	if f != nil {
		f.Flush()
	}
}

// serve starts the loopback bridge and returns the base URL plus a shutdown func.
//
// a may be nil when the agent core could not start; the bridge then serves a
// setup-only UI so the user can configure a provider from the window.
func serve(ctx context.Context, conn *sql.DB, a *app.App, setupErr error) (string, func(), error) {
	b := newBridge(ctx, conn, a, setupErr)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", b.handleChat)
	mux.HandleFunc("/api/models", b.handleModels)
	mux.HandleFunc("/api/sessions", b.handleSessions)

	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		return "", nil, err
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)

	url := "http://" + ln.Addr().String() + "/"
	shutdown := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}
	return url, shutdown, nil
}
