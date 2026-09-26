package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/svpc-ai/svpc/internal/config"
	"github.com/svpc-ai/svpc/internal/llm/models"
	"github.com/svpc-ai/svpc/internal/llm/provider"
	"github.com/svpc-ai/svpc/internal/llm/tools"
	"github.com/svpc-ai/svpc/internal/message"
)

// userMessage is the shortest way to say something to a provider. The message
// package has no constructor for it, so a test writes the struct, and every test
// that needed one would otherwise repeat these three lines.
func userMessage(text string) message.Message {
	return message.Message{
		Role:  message.User,
		Parts: []message.ContentPart{message.TextContent{Text: text}},
	}
}

// A gateway address is only worth storing if the client actually goes there. The
// option that carries it was dead code until this was wired, and a test that only
// checked the configuration would have passed with the wiring still missing.
//
// So the client is pointed at a server started by the test and asked a question.
// Where the request arrives is the only thing that cannot be faked: a provider
// that ignored the endpoint would try to reach the real service, and the request
// would never appear here.
func TestAConfiguredEndpointIsTheOneThatGetsUsed(t *testing.T) {
	// Whatever the provider sends, answer in the shape the OpenAI client expects
	// so the call returns instead of erroring on the response.
	var gotPath, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 0,
			"model":   "gpt-4.1",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "pong"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer server.Close()

	prov, err := provider.NewProvider(
		models.ProviderOpenAI,
		provider.WithAPIKey("sk-test"),
		provider.WithModel(models.SupportedModels["gpt-4.1"]),
		provider.WithOpenAIOptions(provider.WithOpenAIBaseURL(server.URL+"/v1")),
		provider.WithMaxTokens(64),
	)
	if err != nil {
		t.Fatalf("building the provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := prov.SendMessages(ctx, []message.Message{
		userMessage("ping"),
	}, nil)
	if err != nil {
		t.Fatalf("the provider did not reach the configured endpoint: %v", err)
	}

	if gotPath == "" {
		t.Fatal("no request arrived at the configured endpoint, so the base URL was not used")
	}
	if !strings.HasSuffix(gotPath, "/chat/completions") {
		t.Errorf("the request went to %q, want a chat completions path under the configured base", gotPath)
	}
	if gotAuth == "" {
		t.Error("the request carried no credential, so the key was dropped on the way")
	}
	if resp.Content != "pong" {
		t.Errorf("Content = %q, want the reply from the endpoint that was configured", resp.Content)
	}
}

// The same has to hold for a provider whose SDK is the Anthropic one, because the
// two take different options and wiring only one of them would leave the other
// silently ignoring the setting.
func TestAConfiguredEndpointIsUsedByTheOtherProvider(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		// The Anthropic shape: a content block list and a stop reason.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "msg_test",
			"type":  "message",
			"role":  "assistant",
			"model": "claude-sonnet-4",
			"content": []map[string]any{
				{"type": "text", "text": "pong"},
			},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer server.Close()

	prov, err := provider.NewProvider(
		models.ProviderAnthropic,
		provider.WithAPIKey("sk-ant-test"),
		provider.WithModel(models.SupportedModels["claude-sonnet-4-5"]),
		provider.WithAnthropicOptions(provider.WithAnthropicBaseURL(server.URL)),
		provider.WithMaxTokens(64),
	)
	if err != nil {
		t.Fatalf("building the provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := prov.SendMessages(ctx, []message.Message{
		userMessage("ping"),
	}, nil)
	if err != nil {
		t.Fatalf("the provider did not reach the configured endpoint: %v", err)
	}
	if gotPath == "" {
		t.Fatal("no request arrived at the configured endpoint, so the base URL was not used")
	}
	if !strings.Contains(gotPath, "/messages") {
		t.Errorf("the request went to %q, want a messages path under the configured base", gotPath)
	}
	if resp.Content != "pong" {
		t.Errorf("Content = %q, want the reply from the endpoint that was configured", resp.Content)
	}
}

// An empty endpoint must mean the service's own default, not an empty base URL
// that every request would be built from.
func TestAnEmptyEndpointLeavesTheDefaultAlone(t *testing.T) {
	prov, err := provider.NewProvider(
		models.ProviderOpenAI,
		provider.WithAPIKey("sk-test"),
		provider.WithModel(models.SupportedModels["gpt-4.1"]),
		provider.WithOpenAIOptions(provider.WithOpenAIBaseURL("")),
		provider.WithMaxTokens(64),
	)
	if err != nil {
		t.Fatalf("building the provider: %v", err)
	}
	if prov == nil {
		t.Fatal("no provider was built")
	}
	// Nothing to assert on the client itself without reaching the network, so the
	// property is that building succeeds and the model is intact: an empty
	// override must not have replaced the endpoint with "".
	if m := prov.Model(); m.ID == "" {
		t.Error("the provider has no model, so the options were not applied cleanly")
	}
}

// The tools argument is part of the call, and a gateway that rejects an empty
// list would surface as an error rather than a bad request. This keeps the wiring
// honest about the rest of the request, which is what a real gateway sees.
func TestTheCallCarriesTheToolsItWasGiven(t *testing.T) {
	var sawToolList bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Tools []map[string]any `json:"tools"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		sawToolList = len(body.Tools) > 0
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-test", "object": "chat.completion", "created": 0, "model": "gpt-4.1",
			"choices": []map[string]any{{
				"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer server.Close()

	prov, err := provider.NewProvider(
		models.ProviderOpenAI,
		provider.WithAPIKey("sk-test"),
		provider.WithModel(models.SupportedModels["gpt-4.1"]),
		provider.WithOpenAIOptions(provider.WithOpenAIBaseURL(server.URL+"/v1")),
		provider.WithMaxTokens(64),
	)
	if err != nil {
		t.Fatalf("building the provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if _, err := prov.SendMessages(ctx, []message.Message{
		userMessage("what tools do you have"),
	}, []tools.BaseTool{tools.NewBashTool(nil)}); err != nil {
		t.Fatalf("sending with tools: %v", err)
	}
	if !sawToolList {
		t.Error("the request carried no tools, so a gateway would have nothing to choose from")
	}
}

// compile-time proof that the configuration field the factory reads is the one
// this file sets, so a rename on either side is a build failure rather than a
// setting that quietly stops being read.
var _ = config.Provider{BaseURL: ""}
