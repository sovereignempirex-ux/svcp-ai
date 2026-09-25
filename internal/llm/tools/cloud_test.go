package tools

import (
	"context"
	"strings"
	"testing"
)

func TestCloudTool_Info(t *testing.T) {
	info := NewCloudTool(nil).Info()

	if info.Name != CloudToolName {
		t.Errorf("expected name %q, got %q", CloudToolName, info.Name)
	}

	props := info.Parameters["properties"].(map[string]any)
	for _, want := range []string{"provider", "action", "args", "region"} {
		if props[want] == nil {
			t.Errorf("missing property: %s", want)
		}
	}

	enum := props["provider"].(map[string]any)["enum"].([]string)
	for _, want := range []string{"aws", "gcp", "azure", "cloudflare", "vercel", "netlify", "heroku"} {
		if !containsString(enum, want) {
			t.Errorf("missing provider in enum: %s", want)
		}
	}
}

func TestCloudTool_Run_Validation(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"invalid json", `{invalid json`},
		{"missing both", `{}`},
		{"unknown provider", `{"provider": "unknown", "action": "list"}`},
		{"blank action", `{"provider": "aws", "action": "   "}`},
	}

	tool := NewCloudTool(nil)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tool.Run(context.Background(), ToolCall{
				ID: "test", Name: CloudToolName, Input: tc.input,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.IsError {
				t.Error("expected an error response")
			}
		})
	}
}

func TestCliFor(t *testing.T) {
	cases := map[string]string{
		"aws":        "aws",
		"gcp":        "gcloud",
		"google":     "gcloud",
		"azure":      "az",
		"cloudflare": "wrangler",
		"vercel":     "vercel",
		"netlify":    "netlify",
		"heroku":     "heroku",
	}
	for provider, want := range cases {
		got, _, ok := cliFor(provider)
		if !ok {
			t.Errorf("cliFor(%q) reported the provider as unsupported", provider)
			continue
		}
		if got != want {
			t.Errorf("cliFor(%q) = %q, want %q", provider, got, want)
		}
	}

	if _, _, ok := cliFor("digitalocean"); ok {
		t.Error("an unsupported provider should be reported as such")
	}
}

func TestDockerTool_Info(t *testing.T) {
	info := NewDockerTool(nil).Info()

	if info.Name != DockerToolName {
		t.Errorf("expected name %q, got %q", DockerToolName, info.Name)
	}

	props := info.Parameters["properties"].(map[string]any)
	enum := props["action"].(map[string]any)["enum"].([]string)
	for _, want := range []string{
		"build", "run", "push", "pull", "tag", "login", "logout",
		"compose_up", "compose_down", "images_list", "containers_list",
		"container_logs", "container_exec", "container_stop", "container_remove", "prune",
	} {
		if !containsString(enum, want) {
			t.Errorf("missing action in enum: %s", want)
		}
	}
}

func TestDockerTool_Run_Validation(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"missing action", `{}`},
		{"unknown action", `{"action": "teleport"}`},
		{"build without an image", `{"action": "build"}`},
		{"run without an image", `{"action": "run"}`},
		{"logs without a container", `{"action": "container_logs"}`},
		{"exec without a command", `{"action": "container_exec", "container": "web"}`},
	}

	tool := NewDockerTool(nil)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tool.Run(context.Background(), ToolCall{
				ID: "test", Name: DockerToolName, Input: tc.input,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.IsError {
				t.Error("expected an error response")
			}
		})
	}
}

func TestDockerCommand(t *testing.T) {
	t.Run("build with args in a stable order", func(t *testing.T) {
		binary, args, err := dockerCommand(dockerParams{
			Action:     "build",
			Image:      "app:1",
			Dockerfile: "docker/Dockerfile",
			Context:    ".",
			BuildArgs:  map[string]string{"ZED": "1", "ALPHA": "2"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if binary != "docker" || args[0] != "build" || args[1] != "-t" {
			t.Fatalf("unexpected invocation: %s %v", binary, args)
		}
		// Alphabetical build args keep the command reproducible.
		alphaAt := indexOf(args, "ALPHA=2")
		zedAt := indexOf(args, "ZED=1")
		if alphaAt < 0 || zedAt < 0 || alphaAt > zedAt {
			t.Errorf("build args should be sorted: %v", args)
		}
		if !containsString(args, "-f") {
			t.Errorf("a custom Dockerfile needs -f: %v", args)
		}
	})

	t.Run("run with ports and volumes", func(t *testing.T) {
		_, args, err := dockerCommand(dockerParams{
			Action:    "run",
			Image:     "app",
			Container: "web",
			Ports:     []string{"8080:80"},
			Volumes:   []string{"./data:/data"},
			Env:       map[string]string{"PORT": "80"},
			Detach:    true,
			Remove:    true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, want := range []string{"-d", "--rm", "--name", "-p", "-v", "-e", "app"} {
			if !containsString(args, want) {
				t.Errorf("missing %q in %v", want, args)
			}
		}
	})

	t.Run("push qualifies the image with the registry", func(t *testing.T) {
		_, args, err := dockerCommand(dockerParams{
			Action: "push", Image: "app", Registry: "ghcr.io/acme",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if args[1] != "ghcr.io/acme/app" {
			t.Errorf("image = %q, want the registry prefix", args[1])
		}
	})

	t.Run("an already qualified image is left alone", func(t *testing.T) {
		_, args, _ := dockerCommand(dockerParams{
			Action: "push", Image: "acme/app", Registry: "ghcr.io",
		})
		if args[1] != "acme/app" {
			t.Errorf("image = %q, want it unchanged", args[1])
		}
	})

	t.Run("login reads the password from stdin", func(t *testing.T) {
		_, args, _ := dockerCommand(dockerParams{Action: "login", Username: "me"})
		// A password in the argument list would leak into the process table.
		if !containsString(args, "--password-stdin") {
			t.Errorf("expected --password-stdin, got %v", args)
		}
	})

	t.Run("exec passes the command through verbatim", func(t *testing.T) {
		_, args, _ := dockerCommand(dockerParams{
			Action: "container_exec", Container: "web", Command: []string{"ls", "-la", "/app"},
		})
		if args[0] != "exec" || args[1] != "web" {
			t.Fatalf("unexpected prefix: %v", args)
		}
		for i, want := range []string{"ls", "-la", "/app"} {
			if args[2+i] != want {
				t.Errorf("argument %d = %q, want %q", 2+i, args[2+i], want)
			}
		}
	})

	t.Run("compose defaults", func(t *testing.T) {
		_, args, _ := dockerCommand(dockerParams{Action: "compose_up"})
		if !containsString(args, "compose") || !containsString(args, "-d") {
			t.Errorf("unexpected compose args: %v", args)
		}
		_, args, _ = dockerCommand(dockerParams{Action: "compose_down", ComposeFile: "docker-compose.yml"})
		if !containsString(args, "-f") {
			t.Errorf("an explicit compose file needs -f: %v", args)
		}
	})

	t.Run("logs flags", func(t *testing.T) {
		_, args, _ := dockerCommand(dockerParams{Action: "container_logs", Container: "web", Follow: true, Tail: "50"})
		if !containsString(args, "-f") || !containsString(args, "--tail") {
			t.Errorf("unexpected logs args: %v", args)
		}
	})

	t.Run("prune is explicit about being destructive", func(t *testing.T) {
		_, args, _ := dockerCommand(dockerParams{Action: "prune"})
		if !containsString(args, "-f") {
			t.Errorf("prune must not prompt: %v", args)
		}
	})
}

func TestRedactParamsHidesPassword(t *testing.T) {
	got := redactParams(dockerParams{Action: "login", Image: "app", Password: "hunter2"})
	if got["password"] != "***" {
		t.Errorf("password should be redacted, got %v", got["password"])
	}
	if got["image"] != "app" {
		t.Errorf("non-secret fields must survive, got %v", got)
	}
}

func TestKubernetesTool_Info(t *testing.T) {
	info := NewKubernetesTool(nil).Info()

	if info.Name != KubernetesToolName {
		t.Errorf("expected name %q, got %q", KubernetesToolName, info.Name)
	}

	enum := info.Parameters["properties"].(map[string]any)["action"].(map[string]any)["enum"].([]string)
	for _, want := range []string{
		"apply", "delete", "get", "describe", "logs", "exec", "port_forward", "scale",
		"rollout_restart", "rollout_status", "rollout_undo",
		"context_list", "context_use",
		"helm_install", "helm_upgrade", "helm_uninstall", "helm_list", "helm_status",
	} {
		if !containsString(enum, want) {
			t.Errorf("missing action in enum: %s", want)
		}
	}
}

func TestKubernetesTool_Run_Validation(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"missing action", `{}`},
		{"unknown action", `{"action": "teleport"}`},
		{"apply without a file", `{"action": "apply"}`},
		{"delete without a name", `{"action": "delete", "resource": "pod"}`},
		{"logs without a pod", `{"action": "logs"}`},
		{"scale without replicas", `{"action": "scale", "resource": "deployment", "name": "web"}`},
		{"context_use without a context", `{"action": "context_use"}`},
		{"helm_install without a chart", `{"action": "helm_install", "release": "web"}`},
		{"helm without a release", `{"action": "helm_upgrade", "chart": "bitnami/nginx"}`},
	}

	tool := NewKubernetesTool(nil)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tool.Run(context.Background(), ToolCall{
				ID: "test", Name: KubernetesToolName, Input: tc.input,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.IsError {
				t.Error("expected an error response")
			}
		})
	}
}

func TestK8sCommand(t *testing.T) {
	t.Run("apply passes the manifest", func(t *testing.T) {
		binary, args, err := k8sCommand(k8sParams{Action: "apply", File: "k8s/deploy.yaml", Namespace: "prod"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if binary != "kubectl" || args[0] != "apply" {
			t.Fatalf("unexpected invocation: %s %v", binary, args)
		}
		if !containsString(args, "--namespace") || !containsString(args, "-f") {
			t.Errorf("missing flags: %v", args)
		}
	})

	t.Run("get with a selector", func(t *testing.T) {
		_, args, _ := k8sCommand(k8sParams{Action: "get", Resource: "pods", Selector: "app=web"})
		if !containsString(args, "-l") || !containsString(args, "pods") {
			t.Errorf("unexpected args: %v", args)
		}
	})

	t.Run("exec separates the command with a double dash", func(t *testing.T) {
		_, args, _ := k8sCommand(k8sParams{
			Action: "exec", Name: "web-0", Container: "app", Command: []string{"sh", "-c", "ls /app"},
		})
		if indexOf(args, "--") < 0 {
			t.Fatalf("exec needs a -- separator: %v", args)
		}
		sep := indexOf(args, "--")
		if args[sep+1] != "sh" || args[sep+2] != "-c" {
			t.Errorf("the command should follow the separator: %v", args)
		}
		if indexOf(args, "web-0") > sep {
			t.Errorf("the pod name belongs before the separator: %v", args)
		}
	})

	t.Run("scale sets the replica count", func(t *testing.T) {
		_, args, _ := k8sCommand(k8sParams{Action: "scale", Resource: "deployment", Name: "web", Replicas: 3})
		if !containsString(args, "--replicas=3") {
			t.Errorf("unexpected args: %v", args)
		}
	})

	t.Run("rollout actions", func(t *testing.T) {
		_, args, _ := k8sCommand(k8sParams{Action: "rollout_restart", Resource: "deployment", Name: "web"})
		if args[0] != "rollout" || args[1] != "restart" {
			t.Errorf("unexpected args: %v", args)
		}
	})

	t.Run("helm install", func(t *testing.T) {
		binary, args, err := k8sCommand(k8sParams{
			Action: "helm_install", Release: "web", Chart: "bitnami/nginx",
			Namespace: "prod", CreateNamespace: true, Wait: true,
			Values: map[string]any{"replicaCount": 2},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if binary != "helm" || args[0] != "install" {
			t.Fatalf("unexpected invocation: %s %v", binary, args)
		}
		for _, want := range []string{"--create-namespace", "--namespace", "--wait", "--set", "replicaCount=2"} {
			if !containsString(args, want) {
				t.Errorf("missing %q in %v", want, args)
			}
		}
		if args[len(args)-1] != "bitnami/nginx" {
			t.Errorf("the chart must be last, got %v", args)
		}
	})

	t.Run("helm list needs no release", func(t *testing.T) {
		_, args, err := k8sCommand(k8sParams{Action: "helm_list"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if args[0] != "list" {
			t.Errorf("unexpected args: %v", args)
		}
	})

	t.Run("port_forward is not namespace-scoped", func(t *testing.T) {
		// kubectl rejects --namespace here, so the args are built deliberately.
		_, args, err := k8sCommand(k8sParams{Action: "port_forward", Name: "web-0", Port: "8080:80"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if containsString(args, "--namespace") {
			t.Errorf("port-forward takes no namespace flag: %v", args)
		}
	})
}

func TestCICDTool_Info(t *testing.T) {
	info := NewCICDTool(nil).Info()

	if info.Name != CICDToolName {
		t.Errorf("expected name %q, got %q", CICDToolName, info.Name)
	}

	enum := info.Parameters["properties"].(map[string]any)["provider"].(map[string]any)["enum"].([]string)
	for _, want := range []string{"github_actions", "gitlab_ci", "circleci", "buildkite", "jenkins", "travis"} {
		if !containsString(enum, want) {
			t.Errorf("missing provider in enum: %s", want)
		}
	}
}

func TestCICDCLI(t *testing.T) {
	cases := map[string]string{
		"github_actions": "gh",
		"github":         "gh",
		"gitlab_ci":      "glab",
		"circleci":       "circleci",
		"buildkite":      "buildkite",
		"jenkins":        "jenkins-cli",
		"travis":         "travis",
	}
	for provider, want := range cases {
		got, _, ok := cicdCLI(provider)
		if !ok || got != want {
			t.Errorf("cicdCLI(%q) = %q (ok=%v), want %q", provider, got, ok, want)
		}
	}
	if _, _, ok := cicdCLI("hudson"); ok {
		t.Error("an unknown provider should be reported as unsupported")
	}
}

func TestCICDTool_Run_Validation(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"invalid json", `{invalid json`},
		{"missing action", `{"provider": "github_actions"}`},
		{"unknown provider", `{"provider": "hudson", "action": "run"}`},
	}

	tool := NewCICDTool(nil)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tool.Run(context.Background(), ToolCall{
				ID: "test", Name: CICDToolName, Input: tc.input,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.IsError {
				t.Error("expected an error response")
			}
		})
	}
}

// quote renders a Go string as a JSON string literal.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		if r == '"' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

func indexOf(list []string, want string) int {
	for i, s := range list {
		if s == want {
			return i
		}
	}
	return -1
}
