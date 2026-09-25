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
	"github.com/svpc-ai/svpc/internal/permission"
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

// PermissionRequest is an approval the agent is waiting on, as the UI sees it.
type PermissionRequest struct {
	ID          string `json:"id"`
	ToolName    string `json:"tool_name"`
	Action      string `json:"action"`
	Description string `json:"description"`
	Path        string `json:"path,omitempty"`
	// Detail carries the tool's own summary — a command, a file path — so the
	// card can say what is about to happen rather than only naming a tool.
	Detail string `json:"detail,omitempty"`
	// Diff is set for the file tools, matching what the terminal dialog shows.
	Diff string `json:"diff,omitempty"`
}

// PermissionResponse is the user's answer to a PermissionRequest.
type PermissionResponse struct {
	ID     string `json:"id"`
	Action string `json:"action"` // allow | allow_session | deny
}

// The three answers a user can give, mirroring the terminal dialog.
const (
	PermissionAllow           = "allow"
	PermissionAllowForSession = "allow_session"
	PermissionDeny            = "deny"
)

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

	// perm carries approval requests from the agent to the window. Without a
	// consumer the permission service blocks forever, so a chat turn that needs
	// approval would hang: the window has to be able to answer.
	perm chan PermissionRequest

	// active is set while a turn is streaming. Only one turn runs at a time, so
	// every approval request has exactly one stream to travel to.
	active bool

	// awaiting holds the approval requests the window has been shown and not yet
	// answered, keyed by id.
	awaiting map[string]pendingPermission

	// watching guards the one-time subscription to the permission broker.
	watching bool
}

// pendingPermission keeps the request exactly as the permission service built
// it, so an answer can be handed straight back without reconstructing fields.
type pendingPermission struct {
	original permission.PermissionRequest
}

func newBridge(ctx context.Context, conn *sql.DB, a *app.App, setupErr error) *bridge {
	b := &bridge{
		ctx:       ctx,
		db:        conn,
		app:       a,
		coreReady: a != nil,
		sessions:  map[string]string{},
		awaiting:  map[string]pendingPermission{},
		perm:      make(chan PermissionRequest, 8),
	}
	if setupErr != nil {
		b.setupErr = setupErr.Error()
	}
	return b
}

// watchPermissions forwards every approval the agent asks for to the window.
//
// The terminal dialog subscribes to the same broker; without an equivalent
// subscriber here the permission service would block on an unanswered request
// and the turn would never finish.
//
// It is safe to call on every turn: the subscription is made once, and only
// after the core exists.
func (b *bridge) watchPermissions() {
	b.mu.Lock()
	core := b.app
	already := b.watching
	b.watching = true
	b.mu.Unlock()

	if core == nil || already {
		return
	}

	ctx := b.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	go func() {
		for ev := range core.Permissions.Subscribe(ctx) {
			b.rememberPending(ev.Payload)

			req, ok := toPermissionRequest(ev.Payload)
			if !ok {
				continue
			}
			select {
			case b.perm <- req:
			case <-ctx.Done():
				return
			}
		}
	}()
}

// toPermissionRequest converts a broker payload into the wire form, pulling out
// the diff the file tools attach so the window can show the change.
func toPermissionRequest(p permission.PermissionRequest) (PermissionRequest, bool) {
	out := PermissionRequest{
		ID:          p.ID,
		ToolName:    p.ToolName,
		Action:      p.Action,
		Description: p.Description,
		Path:        p.Path,
	}
	if p.ID == "" {
		return out, false
	}

	// The file tools send a typed params struct with a rendered diff; anything
	// else contributes no diff. A type switch keeps those types in their own
	// package instead of forcing a shared interface on them.
	switch params := p.Params.(type) {
	case tools.WritePermissionsParams:
		out.Diff = params.Diff
	case tools.EditPermissionsParams:
		out.Diff = params.Diff
	}
	return out, true
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

	// Only one turn streams at a time: every approval request the agent makes
	// has to reach the single open stream, so a second turn would either steal
	// them or deadlock the first.
	if !b.beginTurn() {
		writeEvent(w, map[string]any{"error": "a turn is already running"})
		return
	}
	defer b.endTurn()

	// The core may have just booted, in which case there was no subscriber when
	// the bridge was created.
	b.watchPermissions()

	done, err := b.app.CoderAgent.Run(ctx, sessionID, req.Content)
	if err != nil {
		writeEvent(w, map[string]any{"error": err.Error()})
		return
	}

	b.pump(ctx, w, flusher, done, sessionID)
	writeEvent(w, map[string]any{"done": true})
}

// beginTurn claims the single turn slot, reporting false when it is taken.
func (b *bridge) beginTurn() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active {
		return false
	}
	b.active = true
	return true
}

func (b *bridge) endTurn() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.active = false
}

// handlePermission records the user's answer to an approval request.
func (b *bridge) handlePermission(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PermissionResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if b.app == nil {
		http.Error(w, "the agent core is not running", http.StatusServiceUnavailable)
		return
	}

	// The window only knows the fields it was shown, so the original payload is
	// kept alongside them: the permission service matches on session, tool,
	// action and path, and GrantPersistant needs the exact request back.
	pending, ok := b.takePending(req.ID)
	if !ok {
		http.Error(w, "unknown or already answered request", http.StatusNotFound)
		return
	}

	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case PermissionAllow:
		b.app.Permissions.Grant(pending)
	case PermissionAllowForSession:
		b.app.Permissions.GrantPersistant(pending)
	case PermissionDeny:
		b.app.Permissions.Deny(pending)
	default:
		http.Error(w, "action must be allow, allow_session or deny", http.StatusBadRequest)
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// takePending removes and returns the stored request for an approval.
func (b *bridge) takePending(id string) (permission.PermissionRequest, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	stored, ok := b.awaiting[id]
	if !ok {
		return permission.PermissionRequest{}, false
	}
	delete(b.awaiting, id)
	return stored.original, true
}

// rememberPending keeps the request so a later response can be matched to it.
func (b *bridge) rememberPending(p permission.PermissionRequest) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.awaiting == nil {
		b.awaiting = map[string]pendingPermission{}
	}
	b.awaiting[p.ID] = pendingPermission{original: p}
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

		case req := <-b.perm:
			// The agent is blocked until the window answers, so this has to be
			// surfaced immediately and flushed before anything else can be
			// written to the same response.
			writeEvent(w, map[string]any{"permission": req})
			flush(w, flusher)

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
	mux.HandleFunc("/api/permission", b.handlePermission)
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
