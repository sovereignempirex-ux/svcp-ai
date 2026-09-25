package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/svpc-ai/svpc/internal/permission"
)

const (
	CICDToolName = "cicd"

	// GitHubWorkflowToolName writes a workflow file into the repository.
	GitHubWorkflowToolName = "github_workflow"
)

// CICDTool drives the CI/CD platforms through their CLIs, so the model gets
// the full surface of each rather than a hand-picked subset.
type CICDTool struct {
	runner *runner
}

func NewCICDTool(permissions permission.Service) BaseTool {
	return &CICDTool{runner: newRunner(permissions)}
}

func (t *CICDTool) Info() ToolInfo {
	return ToolInfo{
		Name: CICDToolName,
		Description: "Interact with CI/CD platforms. Providers: github_actions, gitlab_ci, " +
			"circleci, buildkite, jenkins, travis. The action is passed to the platform's CLI, " +
			"so anything that CLI accepts works.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"provider": map[string]any{
					"type": "string",
					"enum": []string{"github_actions", "gitlab_ci", "circleci", "buildkite", "jenkins", "travis"},
				},
				"action": map[string]any{
					"type":        "string",
					"description": "Subcommand for the platform CLI, e.g. 'run list', 'workflow run'",
				},
				"args":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"repo":     map[string]any{"type": "string", "description": "Repository in owner/name form"},
				"project":  map[string]any{"type": "string", "description": "Project identifier for GitLab"},
				"workflow": map[string]any{"type": "string", "description": "Workflow or pipeline file"},
				"ref":      map[string]any{"type": "string", "description": "Git ref to run against"},
			},
			"required": []string{"provider", "action"},
		},
	}
}

type cicdParams struct {
	Provider string   `json:"provider"`
	Action   string   `json:"action"`
	Args     []string `json:"args"`
	Repo     string   `json:"repo"`
	Project  string   `json:"project"`
	Workflow string   `json:"workflow"`
	Ref      string   `json:"ref"`
}

// cicdCLI maps a provider onto the binary that drives it.
func cicdCLI(provider string) (string, []string, bool) {
	switch strings.ToLower(provider) {
	case "github_actions", "github":
		// gh is already the GitHub CLI the hosting tools use.
		return "gh", nil, true
	case "gitlab_ci", "gitlab":
		return "glab", nil, true
	case "circleci":
		return "circleci", nil, true
	case "buildkite":
		return "buildkite", nil, true
	case "jenkins":
		return "jenkins-cli", nil, true
	case "travis":
		return "travis", nil, true
	default:
		return "", nil, false
	}
}

func (t *CICDTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var p cicdParams
	if err := json.Unmarshal([]byte(call.Input), &p); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}
	if strings.TrimSpace(p.Action) == "" {
		return NewTextErrorResponse("action is required"), nil
	}

	binary, base, ok := cicdCLI(p.Provider)
	if !ok {
		return NewTextErrorResponse(fmt.Sprintf("unsupported provider: %s", p.Provider)), nil
	}

	args := append([]string{}, base...)
	args = append(args, strings.Fields(p.Action)...)
	args = append(args, p.Args...)
	if p.Repo != "" && binary == "gh" {
		args = append(args, "--repo", p.Repo)
	}
	if p.Workflow != "" {
		args = append(args, "-f", p.Workflow)
	}
	if p.Ref != "" {
		args = append(args, "-b", p.Ref)
	}

	sessionID, messageID := sessionContext(ctx)
	if err := t.runner.ask(ctx, sessionID, messageID, CICDToolName,
		fmt.Sprintf("%s %s", binary, strings.Join(args, " ")), p.Action,
		map[string]any{"provider": p.Provider, "action": p.Action}); err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	res, err := t.runner.exec(ctx, binary, args...)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}
	return NewTextResponse(res.String()), nil
}

// GitHubWorkflowTool writes a workflow file into the repository.
type GitHubWorkflowTool struct {
	runner *runner
}

func NewGitHubWorkflowTool(permissions permission.Service) BaseTool {
	return &GitHubWorkflowTool{runner: newRunner(permissions)}
}

func (t *GitHubWorkflowTool) Info() ToolInfo {
	return ToolInfo{
		Name: GitHubWorkflowToolName,
		Description: "Create a GitHub Actions workflow file at .github/workflows/<name>. " +
			"The trigger and jobs are provided as objects, so the generated YAML matches them exactly.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "File name including extension, e.g. 'ci.yml'",
				},
				"on": map[string]any{
					"type":        "object",
					"description": "Triggers, e.g. {\"push\":{\"branches\":[\"main\"]}}",
				},
				"jobs": map[string]any{
					"type":        "object",
					"description": "Job definitions keyed by job id",
				},
				"project_path": map[string]any{"type": "string", "description": "Repository root (default: working directory)"},
			},
			"required": []string{"name", "on", "jobs"},
		},
	}
}

type workflowParams struct {
	Name        string         `json:"name"`
	On          map[string]any `json:"on"`
	Jobs        map[string]any `json:"jobs"`
	ProjectPath string         `json:"project_path"`
}

func (t *GitHubWorkflowTool) Run(ctx context.Context, call ToolCall) (ToolResponse, error) {
	var p workflowParams
	if err := json.Unmarshal([]byte(call.Input), &p); err != nil {
		return NewTextErrorResponse(fmt.Sprintf("invalid parameters: %v", err)), nil
	}
	if p.Name == "" {
		return NewTextErrorResponse("name is required"), nil
	}
	if p.On == nil {
		return NewTextErrorResponse("on (triggers) is required"), nil
	}
	if p.Jobs == nil {
		return NewTextErrorResponse("jobs is required"), nil
	}
	if !strings.HasSuffix(p.Name, ".yml") && !strings.HasSuffix(p.Name, ".yaml") {
		p.Name += ".yml"
	}

	dir := p.ProjectPath
	if dir == "" {
		dir = t.runner.workingDir
	}

	// The file name comes from the model, so reject anything that could escape
	// the workflows directory.
	clean := strings.ReplaceAll(p.Name, "\\", "/")
	if strings.Contains(clean, "/") || strings.Contains(clean, "..") {
		return NewTextErrorResponse("name must be a plain file name, e.g. 'ci.yml'"), nil
	}

	target := "workflows/" + clean
	rel := ".github/" + target

	sessionID, messageID := sessionContext(ctx)
	if err := t.runner.ask(ctx, sessionID, messageID, GitHubWorkflowToolName,
		"write "+rel, "write", map[string]any{"file": rel}); err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	body, err := renderWorkflow(p)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	// The write goes through the file tool's own writer so the change is
	// recorded in history and picked up by the language servers.
	res, err := t.runner.execIn(ctx, dir, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return NewTextErrorResponse("not a git repository: " + err.Error()), nil
	}
	root := strings.TrimSpace(res.Stdout)
	if root == "" {
		return NewTextErrorResponse("could not determine the repository root"), nil
	}

	full := filepath.Join(root, ".github", target)
	if err := writeFileIfChanged(full, body); err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	return NewTextResponse(fmt.Sprintf("wrote %s\n\n%s", rel, body)), nil
}

// renderWorkflow produces the YAML for a workflow definition.
func renderWorkflow(p workflowParams) (string, error) {
	var b strings.Builder

	b.WriteString("name: " + yamlScalar(cleanWorkflowName(p.Name)) + "\n\n")

	b.WriteString("on:\n")
	writeYAMLMap(&b, p.On, "  ")

	b.WriteString("\njobs:\n")
	writeYAMLMap(&b, p.Jobs, "  ")

	return b.String(), nil
}

// cleanWorkflowName turns a file name into a human readable workflow name.
func cleanWorkflowName(file string) string {
	base := file
	for _, ext := range []string{".yml", ".yaml"} {
		base = strings.TrimSuffix(base, ext)
	}
	return strings.ReplaceAll(base, "-", " ")
}

// writeYAMLMap renders a map as YAML. Values are emitted as JSON when they are
// not scalars, which is valid YAML and avoids a full serialiser dependency.
func writeYAMLMap(b *strings.Builder, m map[string]any, indent string) {
	for _, key := range sortedKeys(m) {
		switch v := m[key].(type) {
		case nil:
			continue
		case string, bool, float64, int, int64:
			fmt.Fprintf(b, "%s%s: %s\n", indent, key, yamlScalar(fmt.Sprint(v)))
		case map[string]any:
			fmt.Fprintf(b, "%s%s:\n", indent, key)
			writeYAMLMap(b, v, indent+"  ")
		case []any:
			fmt.Fprintf(b, "%s%s: %s\n", indent, key, jsonValue(v))
		default:
			fmt.Fprintf(b, "%s%s: %s\n", indent, key, jsonValue(v))
		}
	}
}

// yamlScalar quotes a value when plain YAML would misread it.
func yamlScalar(v string) string {
	if v == "" {
		return `""`
	}
	needsQuote := strings.ContainsAny(v, ":#{}[],&*?|<>=!%@`\"'") ||
		strings.HasPrefix(v, " ") || strings.HasSuffix(v, " ") ||
		v == "true" || v == "false" || v == "null" || v == "yes" || v == "no" || v == "on" || v == "off"

	if isNumeric(v) {
		return v
	}
	if needsQuote {
		encoded, _ := json.Marshal(v)
		return string(encoded)
	}
	return v
}

func isNumeric(v string) bool {
	_, err := strconv.ParseFloat(v, 64)
	return err == nil
}

// jsonValue renders a value as compact JSON, which YAML accepts for objects
// and arrays alike.
func jsonValue(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}
