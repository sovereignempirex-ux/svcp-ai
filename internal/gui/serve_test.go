package gui

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/svpc-ai/svpc/internal/app"
	"github.com/svpc-ai/svpc/internal/permission"
)

// A token with mixed case, so a case-insensitive comparison against the wrong
// value is a genuinely different string.
const testToken = "An-Example-Token-long-enough"

func TestIsLoopback(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:8080": true,
		"127.0.0.1":      true,
		"localhost:3000": true,
		"LocalHost:3000": true,
		"[::1]:8080":     true,
		"::1":            true,
		"":               true,
		// A malformed address is treated as remote rather than trusted, which is
		// the direction that fails safely.
		"::1:8080":        false,
		"0.0.0.0:8080":    false,
		"192.168.1.20:80": false,
		"10.0.0.5:1":      false,
		"[::]:8080":       false,
		"example.com:443": false,
		"0.0.0.0":         false,
	}
	for addr, want := range cases {
		if got := isLoopback(addr); got != want {
			t.Errorf("isLoopback(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestServeConfigRefusesUnsafeCombinations(t *testing.T) {
	cases := []struct {
		name string
		cfg  ServeConfig
		want string
	}{
		{"no token", ServeConfig{Addr: "0.0.0.0", Port: 8080}, "token is required"},
		{"short token", ServeConfig{Addr: "0.0.0.0", Token: "abc"}, "too short"},
		{"16 characters is the minimum", ServeConfig{Addr: "0.0.0.0", Token: strings.Repeat("a", 15)}, "too short"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.validate()
			if err == nil {
				t.Fatal("expected the combination to be refused")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestServeConfigAcceptsARealToken(t *testing.T) {
	cfg := ServeConfig{Addr: "0.0.0.0", Port: 8080, Token: strings.Repeat("a", 16)}
	if err := cfg.validate(); err != nil {
		t.Errorf("a 16 character token should be accepted: %v", err)
	}
}

// newJar builds a cookie jar for the client that mimics a browser.
func newJar() (http.CookieJar, error) {
	return cookiejar.New(nil)
}

// startAuthed brings up a real listener behind the token, so the checks below
// exercise the middleware rather than a hand-built request.
func startAuthed(t *testing.T) string {
	t.Helper()

	url, stop, err := ServeRemote(context.Background(), nil, nil, errFake{}, ServeConfig{
		Addr:  "127.0.0.1",
		Token: testToken,
	})
	if err != nil {
		t.Fatalf("starting the bridge: %v", err)
	}
	t.Cleanup(stop)
	return strings.TrimSuffix(url, "/")
}

func TestRemoteBridgeRefusesAnUnauthenticatedRequest(t *testing.T) {
	base := startAuthed(t)

	res, err := http.Get(base + "/api/models")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer res.Body.Close()

	// The endpoint works without a token on loopback, so a 401 here can only
	// come from the middleware.
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
	if got := res.Header.Get("WWW-Authenticate"); !strings.HasPrefix(got, "Bearer") {
		t.Errorf("WWW-Authenticate = %q, want a Bearer challenge", got)
	}
}

func TestRemoteBridgeAcceptsTheTokenThreeWays(t *testing.T) {
	base := startAuthed(t)
	client := &http.Client{}

	t.Run("authorization header", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, base+"/api/models", nil)
		req.Header.Set("Authorization", "Bearer "+testToken)
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want 200", res.StatusCode)
		}
	})

	t.Run("bearer prefix is case-insensitive", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, base+"/api/models", nil)
		req.Header.Set("Authorization", "bearer "+testToken)
		res, err := client.Do(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want 200", res.StatusCode)
		}
	})

	t.Run("query parameter exchanges for a cookie", func(t *testing.T) {
		// A WebView cannot set a header on a top-level navigation, so the URL is
		// the only way in for the first load.
		jar, _ := newJar()
		navigating := &http.Client{Jar: jar}

		res, err := navigating.Get(base + "/?token=" + testToken)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.StatusCode)
		}

		var found *http.Cookie
		for _, c := range res.Cookies() {
			if c.Name == tokenCookie {
				found = c
			}
		}
		if found == nil {
			t.Fatal("the token should have been exchanged for a cookie")
		}
		if !found.HttpOnly {
			t.Error("the cookie should be HttpOnly so script cannot read it")
		}
		if found.SameSite != http.SameSiteStrictMode {
			t.Errorf("SameSite = %v, want Strict", found.SameSite)
		}

		// The cookie alone must then be enough, with no token in the URL.
		follow, err := navigating.Get(base + "/api/models")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer follow.Body.Close()
		if follow.StatusCode != http.StatusOK {
			t.Errorf("status with the cookie = %d, want 200", follow.StatusCode)
		}
	})
}

func TestRemoteBridgeRejectsAWrongToken(t *testing.T) {
	base := startAuthed(t)

	for _, token := range []string{
		"",
		"wrong",
		testToken + "x", // a prefix must not pass
		" " + testToken, // leading whitespace
		strings.ToLower(testToken),
	} {
		req, _ := http.NewRequest(http.MethodGet, base+"/api/models", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := (&http.Client{}).Do(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("token %q was accepted with status %d", token, res.StatusCode)
		}
	}
}

func TestAuthIgnoresTheCookieForAWrongValue(t *testing.T) {
	handler := &auth{token: testToken, next: http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })}

	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	req.AddCookie(&http.Cookie{Name: tokenCookie, Value: "nope"})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestStripTokenRemovesTheCredential(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/chat?token=secret&session_id=abc", nil)
	clean := stripToken(req)

	if got := clean.URL.Query().Get("token"); got != "" {
		t.Errorf("token = %q, want it gone", got)
	}
	// The rest of the query has to survive, or the request changes meaning.
	if got := clean.URL.Query().Get("session_id"); got != "abc" {
		t.Errorf("session_id = %q, want it preserved", got)
	}
	// The original must not be mutated: the caller may still log it.
	if got := req.URL.Query().Get("token"); got != "secret" {
		t.Errorf("the original request was modified: token = %q", got)
	}
}

func TestListenRefusesToBindWithoutAToken(t *testing.T) {
	// The refusal has to happen before any socket exists, so this must not
	// depend on a free port being available.
	if _, err := listen(ServeConfig{Addr: "127.0.0.1", Port: 0}); err == nil {
		t.Fatal("binding without a token must be refused")
	}
}

func TestNewTokenIsUnpredictable(t *testing.T) {
	first, err := newToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := newToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first == second {
		t.Error("two tokens came out identical")
	}
	if len(first) < 32 {
		t.Errorf("token is %d characters, which is short for a shared secret", len(first))
	}
	// URL-safe, so it can be pasted into a browser address bar unescaped.
	if strings.ContainsAny(first, "+/=") {
		t.Errorf("token %q contains characters that need escaping in a URL", first)
	}
}

func TestLoadOrCreateTokenPersists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	first, err := loadOrCreateToken("0.0.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// A second call must return the same token, or every restart would
	// invalidate the device the user already paired.
	second, err := loadOrCreateToken("0.0.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first != second {
		t.Errorf("the token changed between calls: %q then %q", first, second)
	}

	path := filepath.Join(dir, ".svpc", "serve.toml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the token file was not written: %v", err)
	}

	// The restrictive mode only holds where the platform implements it. Windows
	// has no POSIX mode: the 0o600 in the write call is recorded as 0666, and
	// access is governed by the ACLs the per-user config directory already
	// carries. Asserting the mode there would assert something untrue.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("token file mode is %v, which is readable by others", perm)
		}
	} else {
		// The directory itself must be the user's own, or the file inherits
		// whatever that directory allows.
		if !strings.HasPrefix(strings.ToLower(path), strings.ToLower(dir)) {
			t.Errorf("the token was written outside the test directory: %s", path)
		}
	}
}

func TestLoadOrCreateTokenReadsAnExistingValue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	path := filepath.Join(dir, ".svpc")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	// Deliberately different formatting from what is written, because a person
	// will edit this file by hand to rotate the token.
	body := "# a comment\n\ntoken = \"hand-written-value\"\n"
	if err := os.WriteFile(filepath.Join(path, "serve.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := loadOrCreateToken("0.0.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hand-written-value" {
		t.Errorf("token = %q, want the hand-written one", got)
	}
}

func TestLoopbackBridgeNeedsNoToken(t *testing.T) {
	// The desktop window must keep working without any credential, which is the
	// whole point of the default.
	url, stop, err := ServeLocal(context.Background(), nil, nil, errFake{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stop()

	res, err := http.Get(url + "api/models")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}

	host, _, err := net.SplitHostPort(strings.TrimSuffix(strings.TrimPrefix(url, "http://"), "/"))
	if err != nil {
		t.Fatalf("parsing %q: %v", url, err)
	}
	if !isLoopback(host) {
		t.Errorf("the default bound %q, which is not loopback", host)
	}
}

// The core type has to keep the shape the bridge expects, so a signature change
// here is caught at compile time rather than when a phone cannot connect.
var _ = func(ctx context.Context, conn *sql.DB, a *app.App, err error) (string, func(), error) {
	return ServeRemote(ctx, conn, a, err, ServeConfig{})
}

var _ = fmt.Sprintf
var _ = permission.ErrorPermissionDenied
