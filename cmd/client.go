package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
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

	// A network listener has to exist before anything points at it, so it is
	// started first and the window attaches to whatever URL came back.
	var (
		url    string
		stop   func()
		served bool
	)

	if opts.serveAddr != "" {
		token := opts.serveToken
		if token == "" {
			token, err = gui.ResolveServeToken(opts.serveAddr)
			if err != nil {
				return err
			}
		}

		url, stop, err = gui.ServeRemote(ctx, conn, core, coreErr, gui.ServeConfig{
			Addr:  opts.serveAddr,
			Port:  opts.servePort,
			Token: token,
		})
		if err != nil {
			return err
		}
		served = true
		defer stop()
	} else {
		// Loopback only: nothing else on the machine can reach it, so it needs
		// no credential and can pick a free port without asking.
		url, stop, err = gui.ServeLocal(ctx, conn, core, coreErr)
		if err != nil {
			return err
		}
		defer stop()
	}

	if opts.window {
		// The window is a WebView over the same handler, so it simply opens
		// whichever URL the bridge came up on. A wildcard bind is not
		// navigable, so the local one is used instead.
		return gui.RunWindow(workingDir, debug, localURL(url, opts.serveToken))
	}

	if !served {
		// A loopback bridge with no window and no device is not useful.
		return fmt.Errorf("nothing to show: pass --gui for the window, or --serve to expose the bridge")
	}

	printServeBanner(url, opts)
	waitForInterrupt(ctx)
	return nil
}

// localURL rewrites a wildcard bind into one the local machine can open, and
// attaches the token when there is one.
func localURL(url, token string) string {
	host, port, err := net.SplitHostPort(trimScheme(url))
	if err != nil {
		return url
	}
	// 0.0.0.0 and :: are the "every interface" addresses, not destinations.
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

// printServeBanner tells the user where to point the device, including the port
// that was actually bound rather than the one that was asked for.
func printServeBanner(url string, opts clientOptions) {
	_, port, err := net.SplitHostPort(trimScheme(url))
	if err != nil {
		port = strconv.Itoa(opts.servePort)
	}

	fmt.Println()
	fmt.Println("  SVPC AI is serving.")
	fmt.Println("  On the phone, open the app and enter this machine's address in the LAN,")
	fmt.Println("  for example 192.168.1.20, with the port and token below.")
	fmt.Println()
	fmt.Printf("    port   %s\n", port)
	fmt.Printf("    token  %s\n", opts.serveToken)
	fmt.Println()
	fmt.Println("  The token is also saved, so this is the only time it is shown.")
	fmt.Println("  Press Ctrl+C to stop.")
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

// compile-time assurance that the bridge signature stays what cmd expects.
var _ = func(ctx context.Context, conn *sql.DB, a *app.App, err error) {}
