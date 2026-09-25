package gui

import (
	"context"
	"database/sql"
	"os"

	"github.com/svpc-ai/svpc/internal/app"
	"github.com/svpc-ai/svpc/internal/config"
	"github.com/svpc-ai/svpc/internal/db"
	"github.com/svpc-ai/svpc/internal/logging"
)

// session is a booted agent core, which is allowed to be incomplete on a first
// run so the window can open in setup mode and let the user fix it.
type session struct {
	// Conn is the session store; it is never nil once bootstrap succeeds.
	Conn *sql.DB
	// Core is nil when no usable provider is configured yet.
	Core *app.App
	// SetupErr explains why Core is nil, in the user's terms.
	SetupErr error
}

// bootstrap loads the configuration, opens the database and starts the agent
// core. A validation failure is reported through SetupErr rather than returned:
// the window has to open either way so the user can supply a provider.
//
// It is deliberately separate from the window host so the same startup sequence
// is exercised on every platform, not just where WebView2 exists.
func bootstrap(ctx context.Context, workingDir string, debug bool) (*session, error) {
	// The agent validates its provider and model during construction, so the
	// configuration has to be loaded first.
	if _, err := config.Load(workingDir, debug); err != nil {
		logging.Warn("Configuration incomplete, starting in setup mode", "error", err)
	}

	conn, err := db.Connect()
	if err != nil {
		return nil, err
	}

	s := &session{Conn: conn}
	core, coreErr := app.New(ctx, conn)
	if coreErr == nil {
		s.Core = core
		return s, nil
	}

	logging.Warn("Agent core unavailable, starting in setup mode", "error", coreErr)

	// The agent's own error is often a bare "not found"; the configuration
	// validator names the missing provider, which is far more useful in a window.
	if cerr := config.Validate(); cerr != nil {
		coreErr = cerr
	}
	s.SetupErr = coreErr
	return s, nil
}

// Close releases the database handle and shuts the agent down.
func (s *session) Close() {
	if s == nil {
		return
	}
	if s.Core != nil {
		s.Core.Shutdown()
	}
	if s.Conn != nil {
		s.Conn.Close()
	}
}

// tempProfileDir returns a scratch directory for the WebView2 user data.
//
// The web view is given a throwaway profile rather than the user's real browser
// data: it keeps the application's storage separate and leaves nothing behind.
func tempProfileDir() string {
	dir, err := os.MkdirTemp("", "svpc-webview")
	if err != nil {
		return os.TempDir()
	}
	return dir
}
