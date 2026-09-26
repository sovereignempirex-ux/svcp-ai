package cmd

import (
	"net/url"
	"strings"
	"testing"
)

// localURL must be navigable by the window and must carry the credential when
// one is in force. Getting this wrong shows the user a 401 page with no
// explanation, which is exactly what happened when the generated token was not
// passed through.
func TestLocalURL(t *testing.T) {
	cases := []struct {
		name  string
		bind  string
		token string
		want  string
	}{
		{"a wildcard bind is not a destination",
			"http://0.0.0.0:8080/", "t", "http://127.0.0.1:8080/?token=t"},
		{"the ipv6 wildcard likewise",
			"http://[::]:8080/", "t", "http://127.0.0.1:8080/?token=t"},
		{"loopback stays loopback",
			"http://127.0.0.1:53514/", "t", "http://127.0.0.1:53514/?token=t"},
		{"no token means no query",
			"http://127.0.0.1:8080/", "", "http://127.0.0.1:8080/"},
		{"a scheme is optional",
			"0.0.0.0:9000", "t", "http://127.0.0.1:9000/?token=t"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := localURL(tc.bind, tc.token)
			if got != tc.want {
				t.Errorf("localURL(%q, %q) = %q, want %q", tc.bind, tc.token, got, tc.want)
			}
		})
	}
}

// The URL the window opens has to be one the bridge would accept, so the
// credential is checked the way the server will read it.
func TestLocalURLIsAcceptedByTheServer(t *testing.T) {
	got := localURL("http://0.0.0.0:8080/", "s3cret-token")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("the window would fail to navigate to %q: %v", got, err)
	}
	if parsed.Host != "127.0.0.1:8080" {
		t.Errorf("host = %q, want the loopback address", parsed.Host)
	}
	// The server reads the token from this exact parameter, and drops it before
	// serving, so a mismatch here means a blank window.
	if parsed.Query().Get("token") != "s3cret-token" {
		t.Errorf("token = %q, want the one that is in force", parsed.Query().Get("token"))
	}
}

func TestTrimScheme(t *testing.T) {
	cases := map[string]string{
		"http://0.0.0.0:1": "0.0.0.0:1",
		"https://a.b:2":    "a.b:2",
		"0.0.0.0:3":        "0.0.0.0:3",
		"":                 "",
	}
	for in, want := range cases {
		if got := trimScheme(in); got != want {
			t.Errorf("trimScheme(%q) = %q, want %q", in, got, want)
		}
	}
}

// The banner is the only place the user learns the port and the token, so an
// empty token there makes pairing a phone impossible.
func TestServeBannerShowsThePortAndToken(t *testing.T) {
	banner := serveBanner(resolved{
		url:     "http://0.0.0.0:8080/",
		token:   "s3cret-token",
		exposed: true,
	})

	if !strings.Contains(banner, "8080") {
		t.Errorf("the port is missing from the banner:\n%s", banner)
	}
	if !strings.Contains(banner, "s3cret-token") {
		t.Errorf("the token is missing from the banner:\n%s", banner)
	}
	// The port that is printed has to be the one that was bound, which is not
	// the one that was asked for when the flag said 0.
	if strings.Contains(banner, "  port   0\n") {
		t.Errorf("the requested port was printed rather than the bound one:\n%s", banner)
	}
}

func TestServeBannerWithoutAToken(t *testing.T) {
	banner := serveBanner(resolved{url: "http://127.0.0.1:1234/"})

	// An empty value under a "token" heading reads as a real token that was
	// lost, so the absence of a credential has to be stated instead.
	if strings.Contains(banner, "token  \n") {
		t.Errorf("an empty token is printed as though it were a value:\n%s", banner)
	}
	if !strings.Contains(banner, "--serve") {
		t.Errorf("the banner should say how to make the bridge reachable:\n%s", banner)
	}
	if !strings.Contains(banner, "loopback") {
		t.Errorf("the banner should explain why there is no token:\n%s", banner)
	}
}

// resolved exists so the token cannot drift between the window and the banner.
// These two are the only consumers, and they have to agree.
func TestWindowAndBannerAgreeOnTheToken(t *testing.T) {
	live := resolved{url: "http://0.0.0.0:8080/", token: "one-token", exposed: true}

	windowURL := localURL(live.url, live.token)
	banner := serveBanner(live)

	parsed, err := url.Parse(windowURL)
	if err != nil {
		t.Fatalf("bad window url %q: %v", windowURL, err)
	}
	if !strings.Contains(banner, parsed.Query().Get("token")) {
		t.Errorf("the window opens with %q but the banner shows a different token:\n%s",
			parsed.Query().Get("token"), banner)
	}
}
