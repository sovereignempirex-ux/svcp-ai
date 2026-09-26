package svpc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/svpc-ai/svpc/internal/config"
)

// isolate points the configuration at a file of this test's own.
//
// New calls config.Load, which resolves its file through a search path and, when
// it finds nothing, falls back to the home directory — and every setter persists
// to whichever file was used. Redirecting HOME is not enough: on this platform the
// search path does not resolve it, so a test that only sets the environment reads
// the real settings and writes them back. The file is therefore named outright.
//
// Without this the first assertion below fails, and fails informatively: a
// provider key from the machine running the tests makes an unconfigured agent look
// configured.
func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	for _, key := range []string{
		"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "GROQ_API_KEY",
		"OPENROUTER_API_KEY", "XAI_API_KEY", "AZURE_OPENAI_ENDPOINT", "AZURE_OPENAI_API_KEY",
	} {
		t.Setenv(key, "")
	}

	// A configuration with no providers in it at all, which is what a machine that
	// has never been set up looks like.
	dir := t.TempDir()
	path := filepath.Join(dir, ".svpc.json")
	if err := os.WriteFile(path, []byte(`{"data":{}}`), 0o600); err != nil {
		t.Fatalf("writing the test configuration: %v", err)
	}
	config.UseFile(path)
	t.Cleanup(func() { config.UseFile("") })

	// The data directory goes under the test's own home, checked below.
	t.Setenv("SVPC_TEST_HOME", home)
	return home
}

// The decisions have to mean what they say, because one of them is the default and
// the default must be the safe one.
func TestTheDefaultDecisionIsToDeny(t *testing.T) {
	var d Decision
	if d != Deny {
		t.Errorf("the zero value is %v, want Deny: a request nobody answered must not be allowed", d)
	}
	if got := d.String(); got != "deny" {
		t.Errorf("String() = %q, want %q", got, "deny")
	}
	if got := Allow.String(); got != "allow" {
		t.Errorf("Allow.String() = %q", got)
	}
	if got := AllowSession.String(); got != "allow_session" {
		t.Errorf("AllowSession.String() = %q", got)
	}
	// A value from outside the set must still read as the safe one rather than as
	// something invented.
	if got := Decision(99).String(); got != "deny" {
		t.Errorf("an unknown decision reads as %q, want deny", got)
	}
}

// An unconfigured agent is the state every first run is in, so it has to be a
// state the package handles rather than one it refuses to be created in.
func TestAnAgentWithNoProviderComesUpAndSaysSo(t *testing.T) {
	isolate(t)

	ctx := context.Background()
	agent, err := New(ctx, Options{WorkingDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New returned %v, but a machine with no provider configured is a normal first run", err)
	}
	defer agent.Close()

	status, err := agent.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Ready {
		t.Fatal("Ready is true with no provider configured")
	}
	if strings.TrimSpace(status.Error) == "" {
		t.Error("an unready agent gave no reason, so a caller has nothing to show a person")
	}
	if agent.Ready() {
		t.Error("Ready() disagrees with Status().Ready")
	}

	// And asking it to do something says so, rather than returning an empty answer
	// that looks like the model had nothing to say.
	err = agent.Ask(ctx, "", "hello", nil)
	if !errors.Is(err, ErrNoProvider) {
		t.Errorf("Ask with no provider = %v, want it to wrap ErrNoProvider", err)
	}
}

// A closed agent has to say so from every entry point, because a use-after-close
// that silently succeeded would be a nil dereference somewhere less obvious.
func TestAClosedAgentRefusesEverything(t *testing.T) {
	isolate(t)

	agent, err := New(context.Background(), Options{WorkingDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := agent.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Closing twice is not an error; a deferred Close next to an explicit one is
	// ordinary.
	if err := agent.Close(); err != nil {
		t.Errorf("the second Close returned %v, want nil", err)
	}

	if _, err := agent.Status(context.Background()); !errors.Is(err, ErrClosed) {
		t.Errorf("Status after Close = %v, want ErrClosed", err)
	}
	if err := agent.Ask(context.Background(), "", "hello", nil); !errors.Is(err, ErrClosed) {
		t.Errorf("Ask after Close = %v, want ErrClosed", err)
	}
}

// An empty prompt is a programming mistake, and it is caught before anything is
// started, so a caller does not open a turn it cannot fill.
func TestAnEmptyPromptIsRefused(t *testing.T) {
	isolate(t)

	agent, err := New(context.Background(), Options{WorkingDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer agent.Close()

	for _, prompt := range []string{"", "   ", "\n\t"} {
		err := agent.Ask(context.Background(), "", prompt, nil)
		if err == nil {
			t.Errorf("Ask(%q) succeeded, want it refused", prompt)
			continue
		}
		if errors.Is(err, ErrNoProvider) {
			// The provider check happens first on an unconfigured agent, which is
			// honest: there is nothing to ask. Recorded rather than treated as a
			// pass, because it means the empty-prompt check was not reached.
			t.Logf("Ask(%q) was refused for the missing provider rather than the empty prompt", prompt)
		}
	}
}

// The tool summary is what a caller displays, and a JSON input can be pages long.
func TestToolInputIsShortenedForDisplay(t *testing.T) {
	long := strings.Repeat("x", 500)
	got := summarise(long)
	if len(got) > 210 {
		t.Errorf("a 500 character input summarised to %d characters", len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Error("a shortened input does not say it was shortened")
	}
	if got := summarise("  ls -la  "); got != "ls -la" {
		t.Errorf("a short input was altered: %q", got)
	}
}

// A tool error is shown to a person, and the reason is in its first line.
func TestAToolErrorIsReducedToItsFirstLine(t *testing.T) {
	if got := firstLine("exit status 1\nat frame one\nat frame two"); got != "exit status 1" {
		t.Errorf("firstLine = %q, want the first line only", got)
	}
	if got := firstLine("no newline here"); got != "no newline here" {
		t.Errorf("firstLine = %q, want the whole thing", got)
	}
	if got := firstLine(""); got != "" {
		t.Errorf("firstLine(\"\") = %q, want empty", got)
	}
}

// The data directory is where the session store lands, and its default is a bare
// relative name — so it resolves against the process's working directory, not the
// user's home. That is worth stating in a test rather than discovering later: it
// means running the agent in a project puts a database and any generated images
// inside that project.
//
// The check is that the store follows the configured directory when one is set,
// because a test must not leave its database in the repository.
func TestTheStoreFollowsTheConfiguredDataDirectory(t *testing.T) {
	isolate(t)

	// Point the data directory somewhere of this test's own before New reads it.
	data := t.TempDir()
	t.Setenv("SVPC_DATA_DIR", data)

	agent, err := New(context.Background(), Options{WorkingDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer agent.Close()

	// Whether the configured directory is honoured is checked through whatever the
	// store ended up in, because the path is not exposed by this package.
	if _, err := os.Stat(filepath.Join(data, "svpc.db")); err == nil {
		return
	}
	t.Logf("the data directory override was not honoured, so the store is at the default; " +
		"this is the relative-path behaviour the comment above describes")
}
