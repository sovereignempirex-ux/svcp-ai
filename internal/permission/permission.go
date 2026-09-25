package permission

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/svpc-ai/svpc/internal/config"
	"github.com/svpc-ai/svpc/internal/pubsub"
)

var ErrorPermissionDenied = errors.New("permission denied")

type CreatePermissionRequest struct {
	SessionID   string `json:"session_id"`
	ToolName    string `json:"tool_name"`
	Description string `json:"description"`
	Action      string `json:"action"`
	Params      any    `json:"params"`
	Path        string `json:"path"`
}

type PermissionRequest struct {
	ID          string `json:"id"`
	SessionID   string `json:"session_id"`
	ToolName    string `json:"tool_name"`
	Description string `json:"description"`
	Action      string `json:"action"`
	Params      any    `json:"params"`
	Path        string `json:"path"`
}

type Service interface {
	pubsub.Suscriber[PermissionRequest]
	GrantPersistant(permission PermissionRequest)
	Grant(permission PermissionRequest)
	Deny(permission PermissionRequest)
	Request(opts CreatePermissionRequest) bool
	AutoApproveSession(sessionID string)
	// Shutdown denies everything still waiting for an answer.
	Shutdown()
}

type permissionService struct {
	*pubsub.Broker[PermissionRequest]

	sessionPermissions  []PermissionRequest
	pendingRequests     sync.Map
	autoApproveSessions []string

	// done is closed when the service shuts down, releasing anything waiting on
	// an approval that will now never come.
	done chan struct{}
}

// requestTimeout bounds how long a tool waits for a person to answer. It is long
// enough to read a diff and decide, and short enough that an abandoned request
// cannot wedge the agent.
const requestTimeout = 5 * time.Minute

func (s *permissionService) GrantPersistant(permission PermissionRequest) {
	respCh, ok := s.pendingRequests.Load(permission.ID)
	if ok {
		respCh.(chan bool) <- true
	}
	s.sessionPermissions = append(s.sessionPermissions, permission)
}

func (s *permissionService) Grant(permission PermissionRequest) {
	respCh, ok := s.pendingRequests.Load(permission.ID)
	if ok {
		respCh.(chan bool) <- true
	}
}

func (s *permissionService) Deny(permission PermissionRequest) {
	respCh, ok := s.pendingRequests.Load(permission.ID)
	if ok {
		respCh.(chan bool) <- false
	}
}

func (s *permissionService) Request(opts CreatePermissionRequest) bool {
	if slices.Contains(s.autoApproveSessions, opts.SessionID) {
		return true
	}
	dir := filepath.Dir(opts.Path)
	if dir == "." || dir == "" {
		dir = workingDirectory()
	}
	permission := PermissionRequest{
		ID:          uuid.New().String(),
		Path:        dir,
		SessionID:   opts.SessionID,
		ToolName:    opts.ToolName,
		Description: opts.Description,
		Action:      opts.Action,
		Params:      opts.Params,
	}

	for _, p := range s.sessionPermissions {
		if p.ToolName == permission.ToolName && p.Action == permission.Action && p.SessionID == permission.SessionID && p.Path == permission.Path {
			return true
		}
	}

	respCh := make(chan bool, 1)

	s.pendingRequests.Store(permission.ID, respCh)
	defer s.pendingRequests.Delete(permission.ID)

	s.Publish(pubsub.CreatedEvent, permission)

	// Wait for a response, but not forever. A request that is never answered —
	// because the window was closed mid-turn, or because no UI is subscribed at
	// all — would otherwise block the tool, and the agent, permanently. Timing
	// out denies the request, which is the safe direction: the model is told the
	// action was refused and can ask again or explain itself.
	select {
	case resp := <-respCh:
		return resp
	case <-time.After(requestTimeout):
		return false
	case <-s.done:
		return false
	}
}

func (s *permissionService) AutoApproveSession(sessionID string) {
	s.autoApproveSessions = append(s.autoApproveSessions, sessionID)
}

// workingDirectory is the directory an approval is scoped to when the caller
// gave no path of its own. config.WorkingDirectory panics when nothing has been
// loaded yet, which would turn a missing configuration into a crash inside a
// tool, so the process directory is the fallback.
func workingDirectory() string {
	if cfg := config.Get(); cfg != nil && cfg.WorkingDir != "" {
		return cfg.WorkingDir
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

func NewPermissionService() Service {
	return &permissionService{
		Broker:             pubsub.NewBroker[PermissionRequest](),
		sessionPermissions: make([]PermissionRequest, 0),
		done:               make(chan struct{}),
	}
}

// Shutdown releases every request still waiting for an answer. Without it a tool
// blocked on an approval would outlive the process's owner.
func (s *permissionService) Shutdown() {
	select {
	case <-s.done:
		return
	default:
		close(s.done)
	}
	s.Broker.Shutdown()
}
