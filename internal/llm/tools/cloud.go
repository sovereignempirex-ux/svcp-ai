package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	AWSToolName       = "aws"
	GCPToolName       = "gcp"
	AzureToolName     = "azure"
	CloudflareToolName = "cloudflare"
	VercelToolName    = "vercel"
	NetlifyToolName   = "netlify"
	HerokuToolName    = "heroku"
	DockerToolName    = "docker"
	KubernetesToolName = "k8s"
)

type CloudParams struct {
	Provider string         `json:"provider"`
	Action   string         `json:"action"`
	Region   string         `json:"region,omitempty"`
	Params   map[string]any `json:"params,omitempty"`
	Token    string         `json:"token,omitempty"`
	APIURL   string         `json:"api_url,omitempty"`
}

type CloudTool struct {
	httpClient *http.Client
}

func NewCloudTool() BaseTool {
	return &CloudTool{
		httpClient: &http.Client{},
	}
}

func (t *CloudTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "cloud",
		Description: "Interact with cloud providers (AWS, GCP, Azure, Cloudflare, Vercel, Netlify, Heroku). Actions vary by provider.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type":        "string",
					"description": "Cloud provider",
					"enum":        []string{"aws", "gcp", "azure", "cloudflare", "vercel", "netlify", "heroku"},
				},
				"action": map[string]any{
					"type":        "string",
					"description": "Action to perform (provider-specific)",
				},
				"region": map[string]any{
					"type":        "string",
					"description": "Region (for AWS, GCP, Azure)",
				},
				"params": map[string]any{
					"type":        "object",
					"description": "Action-specific parameters",
				},
				"token": map[string]any{
					"type":        "string",
					"description": "API token/credentials",
				},
				"api_url": map[string]any{
					"type":        "string",
					"description": "Custom API endpoint",
				},
			},
			"required": []string{"provider", "action"},
		},
	}
}

func (t *CloudTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params CloudParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	provider := strings.ToLower(params.Provider)
	action := strings.ToLower(params.Action)

	// Provider-specific actions
	validActions := map[string][]string{
		"aws":       {"lambda_invoke", "lambda_create", "lambda_update", "s3_upload", "s3_download", "s3_list", "dynamodb_put", "dynamodb_get", "dynamodb_query", "ecs_deploy", "cloudformation_deploy", "secrets_get", "secrets_put"},
		"gcp":       {"cloud_function_deploy", "cloud_function_invoke", "cloud_run_deploy", "cloud_run_invoke", "storage_upload", "storage_download", "firestore_write", "firestore_read", "secret_get", "secret_put"},
		"azure":     {"function_deploy", "function_invoke", "container_app_deploy", "storage_upload", "storage_download", "keyvault_get", "keyvault_set"},
		"cloudflare": {"worker_deploy", "worker_tail", "dns_create", "dns_list", "dns_delete", "pages_deploy", "kv_put", "kv_get", "r2_upload", "r2_download"},
		"vercel":    {"deploy", "deployments_list", "deployment_get", "logs_get", "domains_list", "env_get", "env_set"},
		"netlify":   {"deploy", "deploys_list", "deploy_get", "functions_invoke", "dns_list", "dns_create", "env_get", "env_set"},
		"heroku":    {"app_create", "app_deploy", "app_logs", "app_restart", "config_get", "config_set", "addons_list", "run_command"},
	}

	if actions, ok := validActions[provider]; ok {
		found := false
		for _, a := range actions {
			if a == action {
				found = true
				break
			}
		}
		if !found {
			return NewTextErrorResponse(fmt.Sprintf("unknown action %q for provider %q. Valid actions: %s", action, provider, strings.Join(actions, ", "))), nil
		}
	}

	// Get credentials from env if not provided
	token := params.Token
	if token == "" {
		switch provider {
		case "aws":
			token = getEnvOrDefault("AWS_ACCESS_KEY_ID", "") + ":" + getEnvOrDefault("AWS_SECRET_ACCESS_KEY", "")
		case "gcp":
			token = getEnvOrDefault("GCP_SERVICE_ACCOUNT_KEY", "")
		case "azure":
			token = getEnvOrDefault("AZURE_CLIENT_ID", "") + ":" + getEnvOrDefault("AZURE_CLIENT_SECRET", "") + ":" + getEnvOrDefault("AZURE_TENANT_ID", "")
		case "cloudflare":
			token = getEnvOrDefault("CLOUDFLARE_API_TOKEN", "")
		case "vercel":
			token = getEnvOrDefault("VERCEL_TOKEN", "")
		case "netlify":
			token = getEnvOrDefault("NETLIFY_AUTH_TOKEN", "")
		case "heroku":
			token = getEnvOrDefault("HEROKU_API_KEY", "")
		}
	}

	if token == "" && provider != "docker" && provider != "k8s" {
		return NewTextErrorResponse(fmt.Sprintf("no credentials for %s (set env vars or provide token)", provider)), nil
	}

	result := fmt.Sprintf(
		"Cloud operation queued:\n- Provider: %s\n- Action: %s\n- Region: %s\n- Params: %v\n\nNote: This is a stub. Real implementation would use provider SDKs/CLIs.",
		provider,
		action,
		defaultString(params.Region, "default"),
		params.Params,
	)

	return NewTextResponse(result), nil
}

type DockerTool struct{}

func NewDockerTool() BaseTool {
	return &DockerTool{}
}

func (t *DockerTool) Info() ToolInfo {
	return ToolInfo{
		Name:        DockerToolName,
		Description: "Docker operations: build, run, push, pull, compose, image management, container management.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "Docker action",
					"enum":        []string{"build", "run", "push", "pull", "compose_up", "compose_down", "images_list", "containers_list", "container_logs", "container_exec", "prune", "tag", "login", "logout"},
				},
				"image": map[string]any{
					"type":        "string",
					"description": "Image name/tag",
				},
				"dockerfile": map[string]any{
					"type":        "string",
					"description": "Path to Dockerfile",
				},
				"context": map[string]any{
					"type":        "string",
					"description": "Build context path",
				},
				"tag": map[string]any{
					"type":        "string",
					"description": "Tag for image",
				},
				"args": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Build args",
				},
				"ports": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Port mappings (e.g., 8080:80)",
				},
				"env": map[string]any{
					"type":        "object",
					"description": "Environment variables",
				},
				"volumes": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Volume mounts",
				},
				"detach": map[string]any{
					"type":        "boolean",
					"description": "Run in background",
				},
				"compose_file": map[string]any{
					"type":        "string",
					"description": "Docker Compose file path",
				},
			},
			"required": []string{"action"},
		},
	}
}

func (t *DockerTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params map[string]any
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	action := params["action"]
	if action == nil {
		return NewTextErrorResponse("action is required"), nil
	}

	result := fmt.Sprintf("Docker %s queued with params: %v\nNote: This is a stub. Real implementation would use Docker CLI or API.", action, params)
	return NewTextResponse(result), nil
}

type KubernetesTool struct{}

func NewKubernetesTool() BaseTool {
	return &KubernetesTool{}
}

func (t *KubernetesTool) Info() ToolInfo {
	return ToolInfo{
		Name:        KubernetesToolName,
		Description: "Kubernetes operations: apply, delete, get, logs, exec, port-forward, scale, rollout, helm.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "Kubernetes action",
					"enum":        []string{"apply", "delete", "get", "describe", "logs", "exec", "port_forward", "scale", "rollout_restart", "rollout_status", "rollout_undo", "helm_install", "helm_upgrade", "helm_uninstall", "helm_list", "context_list", "context_use"},
				},
				"resource": map[string]any{
					"type":        "string",
					"description": "Resource type (deployment, service, pod, configmap, secret, ingress, etc.)",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Resource name",
				},
				"namespace": map[string]any{
					"type":        "string",
					"description": "Namespace",
				},
				"file": map[string]any{
					"type":        "string",
					"description": "YAML file path (for apply/delete)",
				},
				"selector": map[string]any{
					"type":        "string",
					"description": "Label selector",
				},
				"replicas": map[string]any{
					"type":        "integer",
					"description": "Number of replicas (for scale)",
				},
				"container": map[string]any{
					"type":        "string",
					"description": "Container name (for logs/exec)",
				},
				"command": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Command for exec",
				},
				"port": map[string]any{
					"type":        "string",
					"description": "Port for port-forward (local:remote)",
				},
				"chart": map[string]any{
					"type":        "string",
					"description": "Helm chart name/path",
				},
				"values": map[string]any{
					"type":        "object",
					"description": "Helm values",
				},
				"release": map[string]any{
					"type":        "string",
					"description": "Helm release name",
				},
				"context": map[string]any{
					"type":        "string",
					"description": "Kubeconfig context",
				},
			},
			"required": []string{"action"},
		},
	}
}

func (t *KubernetesTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params map[string]any
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	action := params["action"]
	if action == nil {
		return NewTextErrorResponse("action is required"), nil
	}

	result := fmt.Sprintf("Kubernetes %s queued with params: %v\nNote: This is a stub. Real implementation would use kubectl/Helm CLI or client-go.", action, params)
	return NewTextResponse(result), nil
}

type CICDParams struct {
	Provider string         `json:"provider"`
	Action   string         `json:"action"`
	Project  string         `json:"project,omitempty"`
	Params   map[string]any `json:"params,omitempty"`
	Token    string         `json:"token,omitempty"`
	APIURL   string         `json:"api_url,omitempty"`
}

type CICDTool struct {
	httpClient *http.Client
}

func NewCICDTool() BaseTool {
	return &CICDTool{
		httpClient: &http.Client{},
	}
}

func (t *CICDTool) Info() ToolInfo {
	return ToolInfo{
		Name:        "cicd",
		Description: "CI/CD platform operations (GitHub Actions, GitLab CI, CircleCI, Buildkite, Jenkins, etc.). Actions: trigger, list, get, cancel, rerun, logs, artifacts.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type":        "string",
					"description": "CI/CD provider",
					"enum":        []string{"github_actions", "gitlab_ci", "circleci", "buildkite", "jenkins", "teamcity", "drone", "woodpecker"},
				},
				"action": map[string]any{
					"type":        "string",
					"description": "Action: trigger, list, get, cancel, rerun, logs, artifacts, workflows_list",
					"enum":        []string{"trigger", "list", "get", "cancel", "rerun", "logs", "artifacts", "workflows_list"},
				},
				"project": map[string]any{
					"type":        "string",
					"description": "Project/repo identifier (owner/repo for GitHub, group/project for GitLab)",
				},
				"params": map[string]any{
					"type":        "object",
					"description": "Action-specific parameters (branch, tag, inputs, etc.)",
				},
				"token": map[string]any{
					"type":        "string",
					"description": "API token",
				},
				"api_url": map[string]any{
					"type":        "string",
					"description": "Custom API endpoint",
				},
			},
			"required": []string{"provider", "action"},
		},
	}
}

func (t *CICDTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var params CICDParams
	if err := json.Unmarshal([]byte(call.Input), &params); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	provider := strings.ToLower(params.Provider)
	action := strings.ToLower(params.Action)

	validActions := []string{"trigger", "list", "get", "cancel", "rerun", "logs", "artifacts", "workflows_list"}
	found := false
	for _, a := range validActions {
		if a == action {
			found = true
			break
		}
	}
	if !found {
		return NewTextErrorResponse(fmt.Sprintf("unknown action %q. Valid: %s", action, strings.Join(validActions, ", "))), nil
	}

	if params.Project == "" && action != "workflows_list" {
		return NewTextErrorResponse("project is required"), nil
	}

	token := params.Token
	if token == "" {
		switch provider {
		case "github_actions":
			token = getEnvOrDefault("GITHUB_TOKEN", "")
		case "gitlab_ci":
			token = getEnvOrDefault("GITLAB_TOKEN", "")
		case "circleci":
			token = getEnvOrDefault("CIRCLECI_TOKEN", "")
		case "buildkite":
			token = getEnvOrDefault("BUILDKITE_TOKEN", "")
		case "jenkins":
			token = getEnvOrDefault("JENKINS_API_TOKEN", "")
		}
	}

	result := fmt.Sprintf(
		"CI/CD operation queued:\n- Provider: %s\n- Action: %s\n- Project: %s\n- Params: %v\n\nNote: This is a stub. Real implementation would call provider APIs.",
		provider, action, params.Project, params.Params,
	)

	return NewTextResponse(result), nil
}