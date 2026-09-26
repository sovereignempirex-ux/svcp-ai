// Package svpc embeds SVPC AI in a Go program.
//
// The command line tool, the desktop window and the HTTP bridge are three faces
// of one agent, and this is the agent on its own: create it, send a message, read
// what comes back. Nothing else in this repository is required to use it.
//
//	agent, err := svpc.New(ctx, svpc.Options{WorkingDir: "."})
//	if err != nil {
//		return err
//	}
//	defer agent.Close()
//
//	if err := agent.Ask(ctx, "what does this repository do?", func(ev svpc.Event) bool {
//		if ev.Text != "" {
//			fmt.Print(ev.Text)
//		}
//		return true // keep going
//	}); err != nil {
//		return err
//	}
//
// A turn may ask for permission before it acts, and that is the one thing a
// caller has to decide: see Options.OnPermission. Everything else can be ignored.
//
// The configuration is the same one the command line tool reads, so an embedded
// agent and the terminal share credentials, model and history rather than each
// keeping their own.
package svpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/svpc-ai/svpc/internal/app"
	"github.com/svpc-ai/svpc/internal/config"
	"github.com/svpc-ai/svpc/internal/db"
	"github.com/svpc-ai/svpc/internal/permission"
)

// Errors an embedder is most likely to meet. They are wrapped, so compare with
// errors.Is rather than by string.
var (
	// ErrClosed is returned by every method after Close.
	ErrClosed = errors.New("svpc: the agent is closed")
	// ErrNoProvider means no provider is configured, so there is nothing to talk
	// to. It is returned by Ask and not by New: a first run has no provider, and
	// refusing to start would leave a new user with nothing to configure with.
	ErrNoProvider = errors.New("svpc: no provider is configured")
)

// Options configures an agent. The zero value is usable: it reads the same
// configuration the command line tool reads, for the process's own directory.
type Options struct {
	// WorkingDir is the directory the agent treats as the project. Empty means the
	// process's own directory.
	WorkingDir string
	// Debug turns on the debug log, which is noisy and off by default.
	Debug bool
	// OnPermission is called when the agent wants to do something that needs
	// approval, such as run a command or write outside the project.
	//
	// It is called on a goroutine of the agent's own and must return promptly: the
	// turn is waiting, and the request is denied if the answer takes longer than
	// five minutes. A nil function denies everything, which is the right default
	// for a program nobody is watching.
	OnPermission func(Request) Decision
}

// Agent is one running instance. Safe for concurrent use.
type Agent struct {
	opts   Options
	core   *app.App
	store  *sql.DB
	cancel context.CancelFunc
	// startErr is why the agent came up without a provider, kept so Ask can say so
	// rather than reporting something vaguer.
	startErr error

	mu     sync.Mutex
	closed bool
}

// New starts an agent.
//
// A configuration that cannot be read is not fatal. The agent comes up, Status
// says what is missing, and Ask explains it too: a first run has no provider, and
// an error from here would mean a new user only ever saw an error. What is
// returned is a failure that leaves nothing usable behind it, such as a session
// store that will not open.
func New(ctx context.Context, opts Options) (*Agent, error) {
	// The same file the command line tool reads. Loading it again is free — the
	// configuration is a singleton, so this is the first load in a process and a
	// no-op afterwards — and it is what makes an embedded agent and the terminal
	// agree about the model instead of each keeping their own.
	if _, err := config.Load(opts.WorkingDir, opts.Debug); err != nil {
		// Recorded and reported later rather than returned, for the reason above.
		_ = err
	}

	conn, err := db.Connect()
	if err != nil {
		return nil, fmt.Errorf("svpc: opening the session store: %w", err)
	}

	runCtx, cancel := context.WithCancel(ctx)

	core, coreErr := app.New(runCtx, conn)
	if coreErr != nil {
		// Not fatal either. The object is still useful: Status describes the
		// problem, and a caller can write a provider key and start again.
		core = nil
	}

	a := &Agent{opts: opts, core: core, store: conn, cancel: cancel, startErr: coreErr}

	if core != nil && opts.OnPermission != nil {
		// Started here rather than on first use, because a permission request can
		// arrive at any point in a turn and a subscriber added mid-turn would miss
		// the one that is already waiting.
		a.watchPermissions(runCtx)
	}
	return a, nil
}

// Status describes what the agent can do right now. It is the first thing to ask,
// because it separates "not set up yet" from "set up and broken", which need
// different handling.
type Status struct {
	// Ready is false when no provider is configured. The agent is still running and
	// still explains itself.
	Ready bool
	// Provider, Model and ModelName identify what a ready agent is using.
	Provider  string
	Model     string
	ModelName string
	// Context is the model's context window in tokens, or 0 when unknown.
	Context int64
	// WorkingDir is the project directory the agent was given.
	WorkingDir string
	// Error explains an unready agent in terms a person can act on.
	Error string
}

// Status reports what the agent can do.
func (a *Agent) Status(ctx context.Context) (Status, error) {
	if err := a.check(); err != nil {
		return Status{}, err
	}
	if a.core == nil {
		msg := "no provider is configured; set one and start the agent again"
		if a.startErr != nil {
			// The configuration validator names the missing piece, which is more
			// use than the sentence above.
			if verr := config.Validate(); verr != nil {
				msg = verr.Error()
			} else {
				msg = a.startErr.Error()
			}
		}
		return Status{WorkingDir: a.opts.WorkingDir, Error: msg}, nil
	}
	m := a.core.CoderAgent.Model()
	return Status{
		Ready:      true,
		Provider:   string(m.Provider),
		Model:      string(m.ID),
		ModelName:  m.Name,
		Context:    m.ContextWindow,
		WorkingDir: a.opts.WorkingDir,
	}, nil
}

// Ready reports whether the agent has a provider, for a caller that does not need
// the rest.
func (a *Agent) Ready() bool {
	s, err := a.Status(context.Background())
	return err == nil && s.Ready
}

// Close releases everything the agent holds: the session store, the goroutines,
// and anything waiting on an approval, which is answered rather than left hanging.
// Safe to call more than once.
func (a *Agent) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	cancel, core, store := a.cancel, a.core, a.store
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if core != nil {
		// Released first, so a tool waiting on an approval is told no rather than
		// being cut off mid-decision. The service denies everything outstanding.
		core.Permissions.Shutdown()
		core.Shutdown()
	}
	if store != nil {
		return store.Close()
	}
	return nil
}

func (a *Agent) check() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return ErrClosed
	}
	return nil
}

// permissionService is the interface this package needs from the agent's
// permission service, named here so the field can be set in a test without the
// whole app.
type permissionService = permission.Service
