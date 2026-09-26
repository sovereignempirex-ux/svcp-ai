package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/svpc-ai/svpc/internal/llm/models"
)

// The configuration is a package-level singleton: Load returns the first one it
// ever built and every setter writes through it. That is right for a program and
// wrong for a test suite, because the first test to load decides where every
// later one reads and writes. These helpers give each test its own.
//
// Reset is deliberately unexported: nothing outside this package has any business
// discarding a loaded configuration, and a test in another package should not be
// able to either.
func reset() {
	cfg = nil
	configFileOverride = ""
	viper.Reset()
}

// load points the configuration at a file of its own and loads it, then returns
// that file's path.
//
// The providers are given by name, because a configuration with only one provider
// cannot hold a disabled one: Load refuses a configuration in which no provider is
// available, so testing that a disabled provider stays disabled needs a second one
// to carry the load.
//
// The file is named explicitly rather than reached through the search path. The
// setters persist through viper's record of which file was used, and fall back to
// the user's home directory when there is none, so a test that relies on the
// search path resolving $HOME somewhere harmless will sooner or later rewrite
// ~/.svpc.json on the machine running it. That is not hypothetical: an earlier
// version of this file redirected HOME and still wrote to the real one, and put
// a gateway address into it.
//
// Every test checks where the file ended up, so a change that breaks the isolation
// is a failing test rather than a rewritten configuration.
func load(t *testing.T, providers map[string]map[string]any) string {
	t.Helper()
	reset()
	t.Cleanup(reset)

	dir := t.TempDir()
	path := filepath.Join(dir, ".svpc.json")
	raw, err := json.Marshal(map[string]any{"providers": providers})
	if err != nil {
		t.Fatalf("encoding the seed configuration: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("writing the seed configuration: %v", err)
	}

	// Credentials in the environment are copied into the configuration, so without
	// this the test could not tell its own values from a real machine's.
	for _, key := range []string{
		"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "GROQ_API_KEY",
		"OPENROUTER_API_KEY", "XAI_API_KEY", "AZURE_OPENAI_ENDPOINT", "AZURE_OPENAI_API_KEY",
	} {
		t.Setenv(key, "")
	}

	// Named through the package's own seam rather than viper directly, because
	// viper clears an explicitly named file whenever the config name is set, and
	// Load sets the name after every reset.
	configFileOverride = path
	if _, err := Load(t.TempDir(), false); err != nil {
		t.Fatalf("loading configuration: %v", err)
	}

	used := viper.ConfigFileUsed()
	if used == "" {
		t.Fatal("no configuration file was used, so a setter would fall back to the home directory")
	}
	if filepath.Dir(used) != dir {
		t.Fatalf("the configuration was read from %s, which is not this test's own directory %s", used, dir)
	}
	return path
}

// usable is the provider most tests need: present, with a key, and enabled.
func usable() map[string]map[string]any {
	return map[string]map[string]any{"openai": {"apiKey": "sk-test"}}
}

const openaiName = models.ModelProvider("openai")

// TestEndpointIsReadFromConfiguration covers the reading half. The chat request
// had carried a base_url for the whole life of the desktop client and nothing
// looked at it, which is how a field ends up in a published contract that does
// nothing.
func TestEndpointIsReadFromConfiguration(t *testing.T) {
	load(t, map[string]map[string]any{
		"openai": {"apiKey": "sk-test", "baseUrl": "https://gateway.example.internal/v1"},
	})

	if got := Get().Providers[openaiName].BaseURL; got != "https://gateway.example.internal/v1" {
		t.Errorf("BaseURL = %q, want the value in the file", got)
	}
}

// A base URL pasted with a trailing slash would otherwise produce a path with a
// doubled separator, which most gateways reject.
func TestEndpointIsStoredWithoutATrailingSlash(t *testing.T) {
	load(t, usable())

	if err := UpdateProviderBaseURL("openai", "https://gateway.example.internal/v1/"); err != nil {
		t.Fatalf("setting the endpoint: %v", err)
	}
	if got := Get().Providers[openaiName].BaseURL; got != "https://gateway.example.internal/v1" {
		t.Errorf("BaseURL = %q, want the trailing slash removed", got)
	}

	// Clearing it has to restore the default rather than store an empty override,
	// so a cleared field is indistinguishable from one that was never set.
	if err := UpdateProviderBaseURL("openai", "  "); err != nil {
		t.Fatalf("clearing the endpoint: %v", err)
	}
	if got := Get().Providers[openaiName].BaseURL; got != "" {
		t.Errorf("BaseURL = %q after clearing, want empty", got)
	}
}

// Choosing an endpoint is not a statement about credentials, so it must not switch
// on a provider the user turned off. The key setter does re-enable one, and
// copying that here would quietly undo a deliberate choice.
func TestSettingAnEndpointLeavesADisabledProviderDisabled(t *testing.T) {
	path := load(t, map[string]map[string]any{"openai": {"apiKey": "sk-test", "disabled": true}, "anthropic": {"apiKey": "sk-ant"}})

	before := Get().Providers[openaiName].Disabled
	if !before {
		t.Fatal("the seed did not take: the provider came back enabled, so there is nothing to preserve")
	}

	if err := UpdateProviderBaseURL("openai", "https://gateway.example.internal"); err != nil {
		t.Fatalf("setting the endpoint: %v", err)
	}

	if after := Get().Providers[openaiName].Disabled; after != before {
		t.Errorf("Disabled = %v in memory after setting an endpoint, want %v", after, before)
	}

	// The file is checked as well, not just the value in memory. The setter writes
	// the entry twice — once into the loaded struct and once into the file — and
	// only the second survives a restart. A change that re-enabled the provider on
	// disk alone would pass a check that only looked at memory, and would then
	// switch the provider back on at the next start.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the configuration file: %v", err)
	}
	var onDisk struct {
		Providers map[string]struct {
			Disabled bool `json:"disabled"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("the configuration file is not readable JSON: %v", err)
	}
	if onDisk.Providers["openai"].Disabled != before {
		t.Errorf("disabled in the file = %v after setting an endpoint, want %v",
			onDisk.Providers["openai"].Disabled, before)
	}
}

// The reverse also has to hold: the endpoint setter must not disable a provider
// that is in use, which it would if it replaced the entry wholesale.
func TestSettingAnEndpointLeavesAnEnabledProviderEnabled(t *testing.T) {
	load(t, usable())

	if err := UpdateProviderBaseURL("openai", "https://gateway.example.internal"); err != nil {
		t.Fatalf("setting the endpoint: %v", err)
	}

	if Get().Providers[openaiName].Disabled {
		t.Error("Disabled = true after setting an endpoint on an enabled provider")
	}
	if got := Get().Providers[openaiName].APIKey; got != "sk-test" {
		t.Errorf("APIKey = %q, want it left alone", got)
	}
}

// A provider that is not in the configuration has nothing to point anywhere, and
// saying so is better than quietly creating an entry nothing reads.
func TestSettingAnEndpointOnAnUnknownProviderIsRefused(t *testing.T) {
	load(t, usable())

	if err := UpdateProviderBaseURL("not-a-provider", "https://gateway.example.internal"); err == nil {
		t.Error("an unknown provider was accepted, so a typo would create a configuration entry nothing reads")
	}
	if _, present := Get().Providers[models.ModelProvider("not-a-provider")]; present {
		t.Error("an unknown provider was added to the configuration")
	}
}

// The file the user actually keeps has to come out right, because that is what
// survives a restart. Reading the in-memory value only proves the setter touched
// a struct.
func TestEndpointIsPersistedToTheFile(t *testing.T) {
	path := load(t, usable())

	if err := UpdateProviderBaseURL("openai", "https://gateway.example.internal/v1"); err != nil {
		t.Fatalf("setting the endpoint: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the configuration file: %v", err)
	}
	var onDisk struct {
		Providers map[string]struct {
			BaseURL string `json:"baseUrl"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("the configuration file is not readable JSON: %v", err)
	}
	if got := onDisk.Providers["openai"].BaseURL; got != "https://gateway.example.internal/v1" {
		t.Errorf("baseUrl in the file = %q, want the value that was set", got)
	}
}

// The provider name arrives from a chat request typed by a person, so it is
// matched without regard to case.
func TestProviderNameIsMatchedCaseInsensitively(t *testing.T) {
	load(t, usable())

	if err := UpdateProviderBaseURL("OpenAI", "https://gateway.example.internal"); err != nil {
		t.Fatalf("setting the endpoint with mixed case: %v", err)
	}
	if got := Get().Providers[openaiName].BaseURL; got != "https://gateway.example.internal" {
		t.Errorf("BaseURL = %q, want the value that was set", got)
	}
}
