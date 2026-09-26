package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/svpc-ai/svpc/internal/app"
	"github.com/svpc-ai/svpc/internal/config"
	"github.com/svpc-ai/svpc/internal/db"
	"github.com/svpc-ai/svpc/internal/gui"
	"github.com/svpc-ai/svpc/internal/logging"
)

// clientOptions says which face of the client to start.
type clientOptions struct {
	// window opens the native desktop window.
	window bool
	// serveAddr is a network address to serve on; empty means loopback only.
	serveAddr string
	servePort int
	// serveToken overrides the saved one.
	serveToken string
}

// resolved is what runClient worked out, once the token has been settled.
//
// It exists because the token is easy to lose: it is usually generated rather
// than passed, and a copy that is still the empty flag value is what reaches
// the window and the banner. That produced a window full of 401s and a blank
// token on screen, so the two values the rest of this file needs are carried
// together and cannot drift apart.
type resolved struct {
	url   string
	token string
	// exposed is true when a network address is being served.
	exposed bool
	stop    func()
}

// runClient starts the window, the network bridge, or both together.
//
// They are one program: the window is a WebView pointed at the bridge, so
// serving as well costs a second listener on a handler that already exists.
func runClient(workingDir string, debug bool, opts clientOptions) error {
	// Load configuration first: the agent validates its provider and model while
	// being constructed. A failure is not fatal here, because the interface has
	// its own setup path for a first run.
	if _, err := config.Load(workingDir, debug); err != nil {
		logging.Warn("Configuration incomplete, starting in setup mode", "error", err)
	}

	conn, err := db.Connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	core, coreErr := app.New(ctx, conn)
	if coreErr != nil {
		logging.Warn("Agent core unavailable, starting in setup mode", "error", coreErr)
		// The agent's own error is often a bare "not found"; the configuration
		// validator names the missing provider, which is far more useful in a
		// window than in a log.
		if cerr := config.Validate(); cerr != nil {
			coreErr = cerr
		}
		core = nil
	}
	if core != nil {
		defer core.Shutdown()
	}

	// A listener has to exist before anything points at it, so it starts first
	// and the window attaches to whatever URL came back.
	live, err := startBridge(ctx, conn, core, coreErr, opts)
	if err != nil {
		return err
	}
	defer live.stop()

	if opts.window {
		// The window is a WebView over the same handler, so it opens whichever
		// URL the bridge came up on. A wildcard bind is not a destination, so
		// the loopback form is used instead.
		return gui.RunWindow(workingDir, debug, localURL(live.url, live.token))
	}

	if !live.exposed {
		// A loopback bridge with no window and no device is not useful.
		return fmt.Errorf("nothing to show: pass --gui for the window, or --serve to expose the bridge")
	}

	printServeBanner(live)
	waitForInterrupt(ctx)
	return nil
}

// startBridge brings up the listener and settles the token in one place, so
// every consumer below sees the same value.
func startBridge(ctx context.Context, conn *sql.DB, core *app.App, coreErr error,
	opts clientOptions) (resolved, error) {

	if opts.serveAddr == "" {
		// Loopback only: nothing else on the machine can reach it, so it needs
		// no credential and can pick a free port without asking.
		url, stop, err := gui.ServeLocal(ctx, conn, core, coreErr)
		if err != nil {
			return resolved{}, err
		}
		return resolved{url: url, stop: stop}, nil
	}

	token := opts.serveToken
	if token == "" {
		// Reuse the saved one, so a device does not need pairing twice.
		generated, err := gui.ResolveServeToken(opts.serveAddr)
		if err != nil {
			return resolved{}, err
		}
		token = generated
	}

	url, stop, err := gui.ServeRemote(ctx, conn, core, coreErr, gui.ServeConfig{
		Addr:  opts.serveAddr,
		Port:  opts.servePort,
		Token: token,
	})
	if err != nil {
		return resolved{}, err
	}
	return resolved{url: url, token: token, exposed: true, stop: stop}, nil
}

// localURL rewrites a wildcard bind into one the local machine can open, and
// attaches the token when there is one.
func localURL(bind, token string) string {
	// The bridge reports "http://host:port/" and a caller may pass either that
	// or a bare "host:port". Splitting first and joining afterwards keeps the
	// separator out of the port, which would otherwise yield "host:port//" and
	// a path the file server does not serve.
	host, port, err := net.SplitHostPort(trimSlash(trimScheme(bind)))
	if err != nil {
		return bind
	}
	// 0.0.0.0 and :: name every interface; they are not destinations.
	if host == "0.0.0.0" || host == "::" || host == "" {
		host = "127.0.0.1"
	}

	local := "http://" + net.JoinHostPort(host, port) + "/"
	if token != "" {
		local += "?token=" + token
	}
	return local
}

func trimScheme(url string) string {
	for _, prefix := range []string{"http://", "https://"} {
		if len(url) > len(prefix) && url[:len(prefix)] == prefix {
			return url[len(prefix):]
		}
	}
	return url
}

// trimSlash drops a trailing slash so a host:port pair can be split without the
// separator ending up inside the port.
func trimSlash(s string) string {
	return strings.TrimSuffix(s, "/")
}

// serveBanner is the text shown once the bridge is up, as a value so it can be
// checked rather than only looked at.
func serveBanner(live resolved) string {
	var b strings.Builder

	port := "?"
	if _, bound, err := net.SplitHostPort(trimSlash(trimScheme(live.url))); err == nil {
		port = bound
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "  SVPC AI is serving.")
	fmt.Fprintln(&b)

	if live.token == "" {
		// An empty value under a "token" heading reads as a real token that was
		// lost, so the reason has to be stated instead of printing a blank.
		fmt.Fprintln(&b, "  The bridge is on loopback, so nothing else can reach it.")
		fmt.Fprintln(&b, "  To pair a device, stop this and run:  svpc --serve 0.0.0.0")
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "  Press Ctrl+C to stop.")
		return b.String()
	}

	fmt.Fprintln(&b, "  On the phone, open the app and enter this machine's address in the")
	fmt.Fprintln(&b, "  local network, for example 192.168.1.20, with the port and token below.")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "    port   %s\n", port)
	fmt.Fprintf(&b, "    token  %s\n", live.token)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "  The token is also saved, so this is the only time it is shown.")
	fmt.Fprintln(&b, "  To see it again, read serve.toml in your config directory.")
	fmt.Fprintln(&b, "  Press Ctrl+C to stop.")
	return b.String()
}

func printServeBanner(live resolved) {
	fmt.Print(serveBanner(live))
}

// waitForInterrupt blocks until the user stops the process.
func waitForInterrupt(ctx context.Context) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	select {
	case <-signals:
	case <-ctx.Done():
	}
}
