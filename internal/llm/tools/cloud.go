package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/svpc-ai/svpc/internal/permission"
)

const (
	AWSToolName        = "aws"
	GCPToolName        = "gcp"
	AzureToolName      = "azure"
	CloudflareToolName = "cloudflare"
	VercelToolName     = "vercel"
	NetlifyToolName    = "netlify"
	HerokuToolName     = "heroku"
	KubernetesToolName = "k8s"
	DockerToolName     = "docker"

	// CloudToolName is the unified tool that fronts every cloud provider.
	CloudToolName = "cloud"
)

// CloudTool drives the provider CLIs (aws, gcloud, az, wrangler, vercel,
// netlify, heroku) through a single action surface, so the model does not have
// to remember seven different schemas.
type CloudTool struct {
	runner *runner
}

func NewCloudTool(permissions permission.Service) BaseTool {
	return &CloudTool{runner: newRunner(permissions)}
}

func (t *CloudTool) Info() ToolInfo {
	return ToolInfo{
		Name: CloudToolName,
		Description: "Interact with cloud providers through their official CLIs. " +
			"Providers: aws, gcp, azure, cloudflare, vercel, netlify, heroku. " +
			"The action is passed to the provider CLI, so anything that CLI accepts works.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type": "string",
					"enum": []string{"aws", "gcp", "azure", "cloudflare", "vercel", "netlify", "heroku"},
				},
				"action": map[string]any{
					"type":        "string",
					"description": "Subcommand passed to the provider CLI (e.g. 'ec2 describe-instances', 's3 ls', 'projects list')",
				},
				"args": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Additional arguments",
				},
				"region": map[string]any{
					"type":        "string",
					"description": "Region for regional resources (appended as --region where supported)",
				},
			},
			"required": []string{"provider", "action"},
		},
	}
}

type cloudParams struct {
	Provider string   `json:"provider"`
	Action   string   `json:"action"`
	Args     []string `json:"args"`
	Region   string   `json:"region"`
}

// cliFor maps a provider onto the binary and base subcommands that reach it.
func cliFor(provider string) (string, []string, bool) {
	switch strings.ToLower(provider) {
	case "aws":
		return "aws", nil, true
	case "gcp", "google":
		return "gcloud", nil, true
	case "azure":
		return "az", nil, true
	case "cloudflare":
		return "wrangler", nil, true
	case "vercel":
		return "vercel", nil, true
	case "netlify":
		return "netlify", nil, true
	case "heroku":
		return "heroku", nil, true
	default:
		return "", nil, false
	}
}

func (t *CloudTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var p cloudParams
	if err := json.Unmarshal([]byte(call.Input), &p); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	binary, base, ok := cliFor(p.Provider)
	if !ok {
		return NewTextErrorResponse(fmt.Sprintf("unsupported provider: %s", p.Provider)), nil
	}
	if strings.TrimSpace(p.Action) == "" {
		return NewTextErrorResponse("action is required"), nil
	}

	// Split the action so multi-word subcommands survive as separate arguments.
	args := append(base, strings.Fields(p.Action)...)
	args = append(args, p.Args...)
	if p.Region != "" {
		args = append(args, "--region", p.Region)
	}

	sessionID, messageID := sessionContext(ctx)
	if err := t.runner.ask(ctx, sessionID, messageID, CloudToolName,
		fmt.Sprintf("%s %s", binary, strings.Join(args, " ")), p.Action, map[string]any{
			"provider": p.Provider, "action": p.Action,
		}); err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	res, err := t.runner.exec(ctx, binary, args...)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}
	return NewTextResponse(res.String()), nil
}

// KubernetesTool drives kubectl and Helm.
type KubernetesTool struct {
	runner *runner
}

func NewKubernetesTool(permissions permission.Service) BaseTool {
	return &KubernetesTool{runner: newRunner(permissions)}
}

func (t *KubernetesTool) Info() ToolInfo {
	return ToolInfo{
		Name: KubernetesToolName,
		Description: "Interact with Kubernetes clusters through kubectl and Helm. " +
			"Actions: apply, delete, get, describe, logs, exec, port_forward, scale, " +
			"rollout_restart, rollout_status, rollout_undo, context_list, context_use, " +
			"helm_install, helm_upgrade, helm_uninstall, helm_list, helm_status.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type": "string",
					"enum": []string{
						"apply", "delete", "get", "describe", "logs", "exec", "port_forward",
						"scale", "rollout_restart", "rollout_status", "rollout_undo",
						"context_list", "context_use",
						"helm_install", "helm_upgrade", "helm_uninstall", "helm_list", "helm_status",
					},
				},
				"resource":         map[string]any{"type": "string", "description": "Resource type (deployment, service, pod, configmap, secret, ingress...)"},
				"name":             map[string]any{"type": "string", "description": "Resource name"},
				"namespace":        map[string]any{"type": "string", "description": "Namespace (default: current)"},
				"file":             map[string]any{"type": "string", "description": "Manifest file for apply/delete (-f)"},
				"selector":         map[string]any{"type": "string", "description": "Label selector for get/delete/logs (-l)"},
				"replicas":         map[string]any{"type": "integer", "description": "Replica count for scale"},
				"container":        map[string]any{"type": "string", "description": "Container name for logs/exec"},
				"follow":           map[string]any{"type": "boolean", "description": "Follow log output"},
				"tail":             map[string]any{"type": "integer", "description": "Number of log lines"},
				"since":            map[string]any{"type": "string", "description": "Only logs newer than this duration, e.g. 10m"},
				"command":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Command for exec"},
				"port":             map[string]any{"type": "string", "description": "Port mapping for port_forward, e.g. 8080:80"},
				"chart":            map[string]any{"type": "string", "description": "Helm chart reference"},
				"release":          map[string]any{"type": "string", "description": "Helm release name"},
				"values":           map[string]any{"type": "object", "description": "Helm values overrides"},
				"value_files":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Helm values files (-f)"},
				"create_namespace": map[string]any{"type": "boolean", "description": "Create the namespace for helm_install/upgrade"},
				"atomic":           map[string]any{"type": "boolean", "description": "Roll back the release if helm_upgrade fails"},
				"wait":             map[string]any{"type": "boolean", "description": "Wait for the operation to finish"},
				"context":          map[string]any{"type": "string", "description": "Kubeconfig context"},
			},
			"required": []string{"action"},
		},
	}
}

type k8sParams struct {
	Action          string         `json:"action"`
	Resource        string         `json:"resource"`
	Name            string         `json:"name"`
	Namespace       string         `json:"namespace"`
	File            string         `json:"file"`
	Selector        string         `json:"selector"`
	Replicas        int            `json:"replicas"`
	Container       string         `json:"container"`
	Follow          bool           `json:"follow"`
	Tail            int            `json:"tail"`
	Since           string         `json:"since"`
	Command         []string       `json:"command"`
	Port            string         `json:"port"`
	Chart           string         `json:"chart"`
	Release         string         `json:"release"`
	Values          map[string]any `json:"values"`
	ValueFiles      []string       `json:"value_files"`
	CreateNamespace bool           `json:"create_namespace"`
	Atomic          bool           `json:"atomic"`
	Wait            bool           `json:"wait"`
	Context         string         `json:"context"`
}

func (t *KubernetesTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var p k8sParams
	if err := json.Unmarshal([]byte(call.Input), &p); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	binary, args, err := k8sCommand(p)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	sessionID, messageID := sessionContext(ctx)
	if err := t.runner.ask(ctx, sessionID, messageID, KubernetesToolName,
		fmt.Sprintf("%s %s", binary, strings.Join(args, " ")), p.Action, map[string]any{
			"action": p.Action, "resource": p.Resource, "name": p.Name, "namespace": p.Namespace,
		}); err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	res, err := t.runner.exec(ctx, binary, args...)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}
	return NewTextResponse(res.String()), nil
}

// k8sCommand maps an action onto the real kubectl or Helm invocation.
func k8sCommand(p k8sParams) (string, []string, error) {
	action := strings.ToLower(p.Action)

	// Helm actions share a prefix so the branch is easy to scan.
	if strings.HasPrefix(action, "helm_") {
		sub := strings.Replace(action, "helm_", "", 1)
		args := []string{sub}
		if p.Release != "" {
			args = append(args, p.Release)
		} else if sub != "list" {
			return "", nil, fmt.Errorf("release is required for helm_%s", sub)
		}
		if p.CreateNamespace {
			args = append(args, "--create-namespace")
		}
		if p.Namespace != "" {
			args = append(args, "--namespace", p.Namespace)
		}
		if p.Atomic {
			args = append(args, "--atomic")
		}
		if p.Wait {
			args = append(args, "--wait")
		}
		for _, f := range p.ValueFiles {
			args = append(args, "-f", f)
		}
		for _, k := range sortedKeys(p.Values) {
			args = append(args, "--set", fmt.Sprintf("%s=%v", k, p.Values[k]))
		}
		if sub == "install" || sub == "upgrade" {
			if p.Chart == "" {
				return "", nil, fmt.Errorf("chart is required for helm_%s", sub)
			}
			args = append(args, p.Chart)
		}
		return "helm", args, nil
	}

	// Rollout actions are two-word subcommands, so they are handled before the
	// generic flag prefix is built from the single-word action.
	if strings.HasPrefix(action, "rollout_") {
		if p.Resource == "" || p.Name == "" {
			return "", nil, fmt.Errorf("resource and name are required for %s", action)
		}
		sub := strings.Replace(action, "rollout_", "", 1)
		args := []string{"rollout", sub}
		if p.Context != "" {
			args = append(args, "--context", p.Context)
		}
		if p.Namespace != "" {
			args = append(args, "--namespace", p.Namespace)
		}
		return "kubectl", append(args, p.Resource, p.Name), nil
	}

	args := []string{action}
	if p.Context != "" {
		args = append(args, "--context", p.Context)
	}
	if p.Namespace != "" {
		args = append(args, "--namespace", p.Namespace)
	}
	if p.Selector != "" {
		args = append(args, "-l", p.Selector)
	}
	if p.File != "" {
		args = append(args, "-f", p.File)
	}

	switch action {
	case "apply":
		if p.File == "" {
			return "", nil, fmt.Errorf("file is required for apply")
		}
		return "kubectl", args, nil

	case "delete":
		if p.Resource == "" {
			return "", nil, fmt.Errorf("resource is required for delete")
		}
		if p.Name == "" {
			return "", nil, fmt.Errorf("name is required for delete")
		}
		return "kubectl", append(args, p.Resource, p.Name), nil

	case "get", "describe":
		if p.Resource == "" {
			return "", nil, fmt.Errorf("resource is required for %s", action)
		}
		if p.Name != "" {
			args = append(args, p.Name)
		}
		return "kubectl", append(args, p.Resource), nil

	case "logs":
		if p.Name == "" {
			return "", nil, fmt.Errorf("name (pod) is required for logs")
		}
		args = append(args, p.Name)
		if p.Container != "" {
			args = append(args, "-c", p.Container)
		}
		if p.Follow {
			args = append(args, "-f")
		}
		if p.Tail > 0 {
			args = append(args, "--tail", fmt.Sprint(p.Tail))
		}
		if p.Since != "" {
			args = append(args, "--since", p.Since)
		}
		return "kubectl", args, nil

	case "exec":
		if p.Name == "" {
			return "", nil, fmt.Errorf("name (pod) is required for exec")
		}
		if len(p.Command) == 0 {
			return "", nil, fmt.Errorf("command is required for exec")
		}
		if p.Container != "" {
			args = append(args, "-c", p.Container)
		}
		args = append(args, p.Name, "--")
		return "kubectl", append(args, p.Command...), nil

	case "port_forward":
		if p.Name == "" || p.Port == "" {
			return "", nil, fmt.Errorf("name and port are required for port_forward")
		}
		return "kubectl", []string{"port-forward", p.Name, p.Port}, nil

	case "scale":
		if p.Resource == "" || p.Name == "" {
			return "", nil, fmt.Errorf("resource and name are required for scale")
		}
		if p.Replicas <= 0 {
			return "", nil, fmt.Errorf("replicas must be greater than zero")
		}
		return "kubectl", append(args, p.Resource, p.Name,
			"--replicas="+fmt.Sprint(p.Replicas)), nil

	case "context_list":
		return "kubectl", []string{"config", "get-contexts"}, nil

	case "context_use":
		if p.Context == "" {
			return "", nil, fmt.Errorf("context is required for context_use")
		}
		return "kubectl", []string{"config", "use-context", p.Context}, nil

	default:
		return "", nil, fmt.Errorf("unknown action: %s", p.Action)
	}
}
