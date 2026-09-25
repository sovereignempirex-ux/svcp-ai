package gui

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/svpc-ai/svpc/internal/logging"
)

// The bridge starts on loopback with no credential, which is right for a window
// on the same machine and wrong the moment it is reachable from a phone.
//
// ServeConfig is the opt-in that changes that: it binds a chosen address and
// requires a bearer token on every request. Nothing is implicit. A non-loopback
// bind without a token is refused rather than warned about, because the failure
// mode is an unauthenticated agent that can run commands, write files and
// deploy.

// ServeConfig describes how to expose the bridge beyond this machine.
type ServeConfig struct {
	// Addr is the interface to bind. "127.0.0.1" keeps it local.
	Addr string
	// Port is the TCP port; 0 picks a free one.
	Port int
	// Token is the shared secret every request must present. Empty means "use
	// the saved one", which is what a repeat run should do.
	Token string
}

// isLoopback reports whether an address only accepts connections from this host.
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.TrimSpace(host)
	if host == "" || strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	// A name that does not resolve to a literal address cannot be assumed local.
	return false
}

// validate refuses the combinations that would expose an unauthenticated agent.
func (c ServeConfig) validate() error {
	if c.Token == "" {
		return fmt.Errorf(
			"a token is required to serve beyond this machine: " +
				"anyone who can reach the port could run commands as you")
	}
	if len(c.Token) < 16 {
		return fmt.Errorf("the token is too short to be safe; use at least 16 characters")
	}
	return nil
}

// describe renders the bind address for logs and messages.
func (c ServeConfig) describe() string {
	return net.JoinHostPort(c.Addr, strconv.Itoa(c.Port))
}

// ---------------------------------------------------------------------------
// Tokens
// ---------------------------------------------------------------------------

// newToken returns a URL-safe random token, or an error rather than a fallback.
// Silently generating something weak would defeat the point of having one.
func newToken() (string, error) {
	buf := make([]byte, 24) // 192 bits, far past what is guessable
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating a token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// ResolveServeToken returns the saved token for this machine, creating one on
// first use. It is separate from ServeRemote so the caller can show the token
// before any listener exists.
func ResolveServeToken(host string) (string, error) {
	return loadOrCreateToken(host)
}

// loadOrCreateToken reads the saved token, creating one when there is none.
//
// The token is per machine rather than per project, so rotating it does not mean
// visiting every repository.
func loadOrCreateToken(host string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating the config directory: %w", err)
	}
	path := filepath.Join(dir, ".svpc", "serve.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}

	// The file is ours and holds one key, so a full parser would be overkill.
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
			if ok && strings.TrimSpace(key) == "token" {
				if token := strings.Trim(strings.TrimSpace(value), `"`); token != "" {
					return token, nil
				}
			}
		}
	}

	token, err := newToken()
	if err != nil {
		return "", err
	}
	body := fmt.Sprintf(
		"# SVPC AI remote access. Keep this file private.\n"+
			"# Delete it to revoke every device, or change the value to issue a new one.\n"+
			"token = %q\n", token)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return "", fmt.Errorf("saving the token: %w", err)
	}
	return token, nil
}

// ---------------------------------------------------------------------------
// Authentication
// ---------------------------------------------------------------------------

// tokenCookie carries the credential after the first navigation.
const tokenCookie = "svpc_token"

// auth wraps a handler so every request must present the shared token.
//
// A WebView cannot attach a header to a top-level navigation, so a token in the
// query string is accepted and then exchanged for a cookie. That matters because
// the URL lands in the WebView's history, where a header would not.
type auth struct {
	token string
	next  http.Handler
}

// authorized checks a request, in constant time so the token cannot be guessed
// by measuring how long a rejection takes.
func (a *auth) authorized(r *http.Request) bool {
	if a.token == "" {
		return false
	}

	supplied := ""
	if header := r.Header.Get("Authorization"); header != "" {
		if value, ok := cutPrefixFold(header, "Bearer "); ok {
			supplied = value
		}
	}
	if supplied == "" {
		supplied = r.URL.Query().Get("token")
	}
	if supplied == "" {
		if cookie, err := r.Cookie(tokenCookie); err == nil {
			supplied = cookie.Value
		}
	}

	return a.matches(supplied)
}

func (a *auth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// A correct token in the query string is exchanged for a cookie and the
	// request is replayed, because a 401 page is all a WebView can show. This
	// has to happen before the plain-accept path below, or a successful
	// navigation would never set the cookie and every later request would have
	// to carry the secret in its URL.
	if inURL := r.URL.Query().Get("token"); inURL != "" {
		if a.matches(inURL) {
			http.SetCookie(w, &http.Cookie{
				Name:     tokenCookie,
				Value:    a.token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
			a.next.ServeHTTP(w, stripToken(r))
			return
		}
	}

	if a.authorized(r) {
		a.next.ServeHTTP(w, r)
		return
	}

	w.Header().Set("WWW-Authenticate", `Bearer realm="SVPC AI"`)
	http.Error(w, "unauthorised: this bridge requires a token", http.StatusUnauthorized)
}

// matches compares a candidate against the token in constant time.
func (a *auth) matches(candidate string) bool {
	if a.token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(a.token)) == 1
}

// cutPrefixFold is strings.CutPrefix with a case-insensitive prefix, which the
// standard library does not provide.
func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return "", false
	}
	return s[len(prefix):], true
}

// stripToken returns a copy of the request with the credential removed from the
// URL, so it cannot be echoed into a log or a trace.
func stripToken(r *http.Request) *http.Request {
	clean := r.Clone(r.Context())
	query := clean.URL.Query()
	query.Del("token")
	clean.URL.RawQuery = query.Encode()
	return clean
}

// ---------------------------------------------------------------------------
// Binding
// ---------------------------------------------------------------------------

// listen binds the serve address, reporting the one actually in use.
//
// Binding is the last point at which a bad combination can be refused, so a
// listener that exists has already passed validate.
func listen(cfg ServeConfig) (net.Listener, error) {
	if cfg.Addr == "" {
		cfg.Addr = "0.0.0.0"
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	ln, err := net.Listen("tcp", cfg.describe())
	if err != nil {
		return nil, fmt.Errorf("binding %s: %w", cfg.describe(), err)
	}
	if !isLoopback(ln.Addr().String()) {
		logging.Warn("The bridge is reachable from other machines",
			"address", ln.Addr().String(),
			"note", "anyone holding the token can run commands as this user")
	}
	return ln, nil
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
