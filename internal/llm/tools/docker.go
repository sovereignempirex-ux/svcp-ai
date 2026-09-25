package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/svpc-ai/svpc/internal/permission"
)

// DockerTool drives the Docker CLI. It is a thin, explicit wrapper: the model
// picks an action and the parameters, and every invocation is shown to the
// user before it runs.
type DockerTool struct {
	runner *runner
}

func NewDockerTool(permissions permission.Service) BaseTool {
	return &DockerTool{runner: newRunner(permissions)}
}

func (t *DockerTool) Info() ToolInfo {
	return ToolInfo{
		Name:        DockerToolName,
		Description: "Build, run and manage Docker images and containers. Actions: build, run, push, pull, compose_up, compose_down, images_list, containers_list, container_logs, container_exec, stop, remove, prune, tag, login.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type": "string",
					"enum": []string{
						"build", "run", "push", "pull", "tag", "login", "logout",
						"compose_up", "compose_down",
						"images_list", "containers_list", "container_logs", "container_exec",
						"container_stop", "container_remove", "prune",
					},
				},
				"image":        map[string]any{"type": "string", "description": "Image name, e.g. 'myapp:1.0'"},
				"dockerfile":   map[string]any{"type": "string", "description": "Path to the Dockerfile (default 'Dockerfile')"},
				"context":      map[string]any{"type": "string", "description": "Build context directory (default '.')"},
				"target":       map[string]any{"type": "string", "description": "Multi-stage build target"},
				"build_args":   map[string]any{"type": "object", "description": "Build arguments, key/value"},
				"registry":     map[string]any{"type": "string", "description": "Registry for tag/push/login"},
				"tag":          map[string]any{"type": "string", "description": "Tag to apply"},
				"username":     map[string]any{"type": "string", "description": "Registry username for login"},
				"password":     map[string]any{"type": "string", "description": "Registry password or token for login"},
				"container":    map[string]any{"type": "string", "description": "Container name or id"},
				"ports":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Port mappings, e.g. ['8080:80']"},
				"volumes":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Volume mounts, e.g. ['./data:/data']"},
				"env":          map[string]any{"type": "object", "description": "Environment variables"},
				"detach":       map[string]any{"type": "boolean", "description": "Run in the background"},
				"remove":       map[string]any{"type": "boolean", "description": "Remove the container when it exits (--rm)"},
				"follow":       map[string]any{"type": "boolean", "description": "Follow log output (logs -f)"},
				"tail":         map[string]any{"type": "string", "description": "Number of log lines to show"},
				"compose_file": map[string]any{"type": "string", "description": "Compose file (default 'docker-compose.yml')"},
				"service":      map[string]any{"type": "string", "description": "Compose service name"},
				"command":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Command for container_exec"},
			},
			"required": []string{"action"},
		},
	}
}

type dockerParams struct {
	Action      string            `json:"action"`
	Image       string            `json:"image"`
	Dockerfile  string            `json:"dockerfile"`
	Context     string            `json:"context"`
	Target      string            `json:"target"`
	BuildArgs   map[string]string `json:"build_args"`
	Registry    string            `json:"registry"`
	Tag         string            `json:"tag"`
	Username    string            `json:"username"`
	Password    string            `json:"password"`
	Container   string            `json:"container"`
	Ports       []string          `json:"ports"`
	Volumes     []string          `json:"volumes"`
	Env         map[string]string `json:"env"`
	Detach      bool              `json:"detach"`
	Remove      bool              `json:"remove"`
	Follow      bool              `json:"follow"`
	Tail        string            `json:"tail"`
	ComposeFile string            `json:"compose_file"`
	Service     string            `json:"service"`
	Command     []string          `json:"command"`
}

func (t *DockerTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var p dockerParams
	if err := json.Unmarshal([]byte(call.Input), &p); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}

	binary, args, err := dockerCommand(p)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	sessionID, messageID := sessionContext(ctx)
	if err := t.runner.ask(ctx, sessionID, messageID, DockerToolName,
		fmt.Sprintf("docker %s", strings.Join(args, " ")), p.Action, redactParams(p)); err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	// The password goes in on stdin, never as an argument, so it cannot be
	// read out of the process table or the transcript.
	if p.Password != "" {
		return runWithInput(ctx, t.runner, binary, args, nil, p.Password)
	}

	res, err := t.runner.exec(ctx, binary, args...)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}
	return NewTextResponse(res.String()), nil
}

// dockerCommand maps the action onto the real CLI invocation.
func dockerCommand(p dockerParams) (string, []string, error) {
	switch strings.ToLower(p.Action) {
	case "build":
		if p.Image == "" {
			return "", nil, fmt.Errorf("image is required for build")
		}
		image := p.Image
		if p.Target != "" {
			image = p.Target
		}
		args := []string{"build", "-t", image}
		if df := orString(p.Dockerfile, "Dockerfile"); df != "Dockerfile" {
			args = append(args, "-f", df)
		}
		if p.Context != "" {
			args = append(args, p.Context)
		}
		for _, k := range sortedKeys(p.BuildArgs) {
			args = append(args, "--build-arg", k+"="+p.BuildArgs[k])
		}
		return "docker", args, nil

	case "run":
		if p.Image == "" {
			return "", nil, fmt.Errorf("image is required for run")
		}
		args := []string{"run"}
		if p.Detach {
			args = append(args, "-d")
		}
		if p.Remove {
			args = append(args, "--rm")
		}
		if p.Container != "" {
			args = append(args, "--name", p.Container)
		}
		for _, port := range p.Ports {
			args = append(args, "-p", port)
		}
		for _, vol := range p.Volumes {
			args = append(args, "-v", vol)
		}
		for _, k := range sortedKeys(p.Env) {
			args = append(args, "-e", k+"="+p.Env[k])
		}
		return "docker", append(args, p.Image), nil

	case "push", "pull":
		if p.Image == "" {
			return "", nil, fmt.Errorf("image is required for %s", p.Action)
		}
		image := p.Image
		if p.Registry != "" && !strings.Contains(image, "/") {
			image = p.Registry + "/" + image
		}
		return "docker", []string{strings.ToLower(p.Action), image}, nil

	case "tag":
		if p.Image == "" || p.Tag == "" {
			return "", nil, fmt.Errorf("image and tag are required for tag")
		}
		tag := p.Tag
		if p.Registry != "" && !strings.Contains(tag, "/") {
			tag = p.Registry + "/" + tag
		}
		return "docker", []string{"tag", p.Image, tag}, nil

	case "login":
		registry := orString(p.Registry, "https://index.docker.io/v1/")
		args := []string{"login", registry}
		if p.Username != "" {
			args = append(args, "-u", p.Username)
		}
		// --password-stdin is unconditional: without it docker falls back to an
		// interactive prompt that would hang the agent with nobody to answer.
		// The password itself arrives through the environment instead.
		return "docker", append(args, "--password-stdin"), nil

	case "logout":
		return "docker", []string{"logout"}, nil

	case "compose_up":
		return "docker", append(composeArgs(p.ComposeFile), "up", "-d"), nil

	case "compose_down":
		return "docker", append(composeArgs(p.ComposeFile), "down"), nil

	case "images_list":
		return "docker", []string{"images"}, nil

	case "containers_list":
		return "docker", []string{"ps", "-a"}, nil

	case "container_logs":
		if p.Container == "" {
			return "", nil, fmt.Errorf("container is required for container_logs")
		}
		args := []string{"logs"}
		if p.Follow {
			args = append(args, "-f")
		}
		if p.Tail != "" {
			args = append(args, "--tail", p.Tail)
		}
		return "docker", append(args, p.Container), nil

	case "container_exec":
		if p.Container == "" {
			return "", nil, fmt.Errorf("container is required for container_exec")
		}
		if len(p.Command) == 0 {
			return "", nil, fmt.Errorf("command is required for container_exec")
		}
		return "docker", append([]string{"exec", p.Container}, p.Command...), nil

	case "container_stop":
		if p.Container == "" {
			return "", nil, fmt.Errorf("container is required for container_stop")
		}
		return "docker", []string{"stop", p.Container}, nil

	case "container_remove":
		if p.Container == "" {
			return "", nil, fmt.Errorf("container is required for container_remove")
		}
		args := []string{"rm"}
		if p.Force() {
			args = append(args, "-f")
		}
		return "docker", append(args, p.Container), nil

	case "prune":
		return "docker", []string{"system", "prune", "-f"}, nil

	default:
		return "", nil, fmt.Errorf("unknown action: %s", p.Action)
	}
}

// Force reports whether a removal should skip the confirmation prompt.
func (p dockerParams) Force() bool { return true }

func composeArgs(file string) []string {
	if file == "" {
		return []string{"compose"}
	}
	return []string{"compose", "-f", file}
}

// redactParams drops secrets before the parameters are shown in the
// permission prompt or written to the transcript.
func redactParams(p dockerParams) map[string]any {
	params := map[string]any{"action": p.Action}
	if p.Image != "" {
		params["image"] = p.Image
	}
	if p.Container != "" {
		params["container"] = p.Container
	}
	if p.Service != "" {
		params["service"] = p.Service
	}
	if p.Password != "" {
		params["password"] = "***"
	}
	return params
}
