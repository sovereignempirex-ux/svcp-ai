package tools

import (
	"context"
	"testing"
)

func TestCloudTool_Info(t *testing.T) {
	tool := NewCloudTool()
	info := tool.Info()

	if info.Name != "cloud" {
		t.Errorf("expected name %q, got %q", "cloud", info.Name)
	}

	props := info.Parameters["properties"].(map[string]any)
	for _, prop := range []string{"provider", "action", "region", "params", "token", "api_url"} {
		if props[prop] == nil {
			t.Errorf("missing property: %s", prop)
		}
	}

	// Check provider enum
	providerProp := props["provider"].(map[string]any)
	enum := providerProp["enum"].([]string)
	expectedProviders := []string{"aws", "gcp", "azure", "cloudflare", "vercel", "netlify", "heroku"}
	for _, ep := range expectedProviders {
		found := false
		for _, e := range enum {
			if e == ep {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing provider in enum: %s", ep)
		}
	}
}

func TestCloudTool_Run_InvalidParams(t *testing.T) {
	tool := NewCloudTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  "cloud",
		Input: `{invalid json`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for invalid JSON")
	}
}

func TestCloudTool_Run_MissingRequired(t *testing.T) {
	tool := NewCloudTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  "cloud",
		Input: `{}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for missing provider/action")
	}
}

func TestCloudTool_Run_UnknownProvider(t *testing.T) {
	tool := NewCloudTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  "cloud",
		Input: `{"provider": "unknown", "action": "test"}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for unknown provider")
	}
}

func TestCloudTool_Run_UnknownAction(t *testing.T) {
	tool := NewCloudTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  "cloud",
		Input: `{"provider": "aws", "action": "unknown_action"}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for unknown action")
	}
}

func TestDockerTool_Info(t *testing.T) {
	tool := NewDockerTool()
	info := tool.Info()

	if info.Name != DockerToolName {
		t.Errorf("expected name %q, got %q", DockerToolName, info.Name)
	}

	props := info.Parameters["properties"].(map[string]any)
	actionProp := props["action"].(map[string]any)
	enum := actionProp["enum"].([]string)
	expectedActions := []string{"build", "run", "push", "pull", "compose_up", "compose_down", "images_list", "containers_list", "container_logs", "container_exec", "prune", "tag", "login", "logout"}
	for _, ea := range expectedActions {
		found := false
		for _, e := range enum {
			if e == ea {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing action in enum: %s", ea)
		}
	}
}

func TestDockerTool_Run_MissingAction(t *testing.T) {
	tool := NewDockerTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  DockerToolName,
		Input: `{}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for missing action")
	}
}

func TestKubernetesTool_Info(t *testing.T) {
	tool := NewKubernetesTool()
	info := tool.Info()

	if info.Name != KubernetesToolName {
		t.Errorf("expected name %q, got %q", KubernetesToolName, info.Name)
	}

	props := info.Parameters["properties"].(map[string]any)
	actionProp := props["action"].(map[string]any)
	enum := actionProp["enum"].([]string)
	expectedActions := []string{"apply", "delete", "get", "describe", "logs", "exec", "port_forward", "scale", "rollout_restart", "rollout_status", "rollout_undo", "helm_install", "helm_upgrade", "helm_uninstall", "helm_list", "context_list", "context_use"}
	for _, ea := range expectedActions {
		found := false
		for _, e := range enum {
			if e == ea {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing action in enum: %s", ea)
		}
	}
}

func TestKubernetesTool_Run_MissingAction(t *testing.T) {
	tool := NewKubernetesTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  KubernetesToolName,
		Input: `{}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for missing action")
	}
}

func TestCICDTool_Info(t *testing.T) {
	tool := NewCICDTool()
	info := tool.Info()

	if info.Name != "cicd" {
		t.Errorf("expected name %q, got %q", "cicd", info.Name)
	}

	props := info.Parameters["properties"].(map[string]any)
	providerProp := props["provider"].(map[string]any)
	enum := providerProp["enum"].([]string)
	expectedProviders := []string{"github_actions", "gitlab_ci", "circleci", "buildkite", "jenkins", "teamcity", "drone", "woodpecker"}
	for _, ep := range expectedProviders {
		found := false
		for _, e := range enum {
			if e == ep {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing provider in enum: %s", ep)
		}
	}
}

func TestCICDTool_Run_InvalidParams(t *testing.T) {
	tool := NewCICDTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  "cicd",
		Input: `{invalid json`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for invalid JSON")
	}
}

func TestCICDTool_Run_MissingRequired(t *testing.T) {
	tool := NewCICDTool()

	response, err := tool.Run(context.Background(), ToolCall{
		ID:    "test",
		Name:  "cicd",
		Input: `{"provider": "github_actions"}`,
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !response.IsError {
		t.Error("expected error response for missing action")
	}
}