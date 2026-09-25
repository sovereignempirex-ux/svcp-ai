package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/svpc-ai/svpc/internal/config"
	"github.com/svpc-ai/svpc/internal/permission"
)

// maxCommandOutput caps what a single command may return. Build logs are
// enormous and the model only ever needs the tail, which is where the error is.
const maxCommandOutput = 64 * 1024

// defaultCommandTimeout stops a hung CLI from blocking the agent forever.
const defaultCommandTimeout = 5 * time.Minute

// commandResult is the outcome of one external invocation.
type commandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// String renders the result the way a model wants to read it: the useful
// output, plus a clear marker when the command failed.
func (r commandResult) String() string {
	var b strings.Builder

	out := strings.TrimSpace(r.Stdout)
	errOut := strings.TrimSpace(r.Stderr)

	if out != "" {
		b.WriteString(out)
	}
	if errOut != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(errOut)
	}
	if b.Len() == 0 {
		b.WriteString("(no output)")
	}

	if r.ExitCode != 0 {
		fmt.Fprintf(&b, "\n\n[exit code %d]", r.ExitCode)
	}
	return b.String()
}

// runner executes external programs on behalf of a tool. Every invocation goes
// through the permission service first, so a build or deploy still asks the
// user before it touches the machine.
type runner struct {
	permissions permission.Service
	workingDir  string
}

// newRunner builds a runner rooted at the configured working directory.
func newRunner(permissions permission.Service) *runner {
	return &runner{permissions: permissions, workingDir: workingDirectory()}
}

// workingDirectory returns the configured working directory, falling back to
// the process directory. config.WorkingDirectory panics when no configuration
// has been loaded, which is not something a tool should crash over.
func workingDirectory() string {
	if cfg := config.Get(); cfg != nil && cfg.WorkingDir != "" {
		return cfg.WorkingDir
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

// exec runs a program with args in the working directory, capturing its output.
// A non-zero exit is reported in the result rather than as an error, because
// most build tools use it to signal a normal failure.
func (r *runner) exec(ctx context.Context, name string, args ...string) (commandResult, error) {
	path, err := lookPath(name)
	if err != nil {
		return commandResult{}, err
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultCommandTimeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = r.workingDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &stdout, limit: maxCommandOutput}
	cmd.Stderr = &limitedWriter{w: &stderr, limit: maxCommandOutput}

	runErr := cmd.Run()
	res := commandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if runErr != nil {
		var exitErr *exec.ExitError
		if ok := asExitError(runErr, &exitErr); ok {
			res.ExitCode = exitErr.ExitCode()
			return res, nil
		}
		// The context expiring is the common case here and deserves a clear
		// message rather than the raw "signal: killed".
		if ctx.Err() != nil {
			return res, fmt.Errorf("%s timed out after %s", name, defaultCommandTimeout)
		}
		return res, fmt.Errorf("%s failed: %w", name, runErr)
	}
	return res, nil
}

// execIn runs a program in a specific directory, leaving the runner's default
// working directory untouched.
func (r *runner) execIn(ctx context.Context, dir, name string, args ...string) (commandResult, error) {
	if dir == "" || sameDir(dir, r.workingDir) {
		return r.exec(ctx, name, args...)
	}

	path, err := lookPath(name)
	if err != nil {
		return commandResult{}, err
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultCommandTimeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &stdout, limit: maxCommandOutput}
	cmd.Stderr = &limitedWriter{w: &stderr, limit: maxCommandOutput}

	runErr := cmd.Run()
	res := commandResult{Stdout: stdout.String(), Stderr: stderr.String()}

	if runErr != nil {
		var exitErr *exec.ExitError
		if asExitError(runErr, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
			return res, nil
		}
		if ctx.Err() != nil {
			return res, fmt.Errorf("%s timed out after %s", name, defaultCommandTimeout)
		}
		return res, fmt.Errorf("%s failed: %w", name, runErr)
	}
	return res, nil
}

func sameDir(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return strings.EqualFold(aa, bb)
}

// fileExists reports whether a path inside dir is present.
func fileExists(dir, name string) bool {
	if dir == "" {
		dir = "."
	}
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

// hasDependency reports whether package.json declares a dependency.
func hasDependency(dir, name string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return false
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false
	}
	_, inDeps := pkg.Dependencies[name]
	_, inDev := pkg.DevDependencies[name]
	return inDeps || inDev
}

// npmScriptExists reports whether package.json defines a named script.
func npmScriptExists(dir, name string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false
	}
	_, ok := pkg.Scripts[name]
	return ok
}

// stringField reads a string from an untyped config map.
func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

// writeFileIfChanged creates the parent directories and writes content only
// when it differs from what is already on disk, so an unchanged write does not
// churn the file's modification time.
func writeFileIfChanged(path, content string) error {
	if existing, err := os.ReadFile(path); err == nil && string(existing) == content {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("error creating directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("error writing file: %w", err)
	}
	return nil
}

// firstNonEmpty returns the first non-blank argument.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// ask requests permission for a command before it runs. The session and message
// IDs come from the context, exactly like the file tools do.
func (r *runner) ask(ctx context.Context, sessionID, messageID, toolName, description, action string, params map[string]any) error {
	if r.permissions == nil {
		return nil
	}
	if r.permissions.Request(permission.CreatePermissionRequest{
		SessionID:   sessionID,
		ToolName:    toolName,
		Description: description,
		Action:      action,
		Params:      params,
	}) {
		return nil
	}
	return permission.ErrorPermissionDenied
}

// sessionContext pulls the identifiers the permission service needs out of the
// tool context.
func sessionContext(ctx context.Context) (string, string) {
	return GetContextValues(ctx)
}

// runWithEnv executes a program with extra environment variables. It exists so
// credentials can be passed to a CLI without ever appearing in an argument
// list, where they would show up in the process table and the transcript.
func runWithEnv(ctx context.Context, r *runner, name string, args []string, extraEnv []string) (ToolResponse, error) {
	return runWithInput(ctx, r, name, args, extraEnv, "")
}

// runWithInput executes a program with extra environment variables and a
// standard input payload, for CLIs that read a secret from stdin.
func runWithInput(ctx context.Context, r *runner, name string, args, extraEnv []string, stdin string) (ToolResponse, error) {
	path, err := lookPath(name)
	if err != nil {
		return NewTextErrorResponse(err.Error()), nil
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultCommandTimeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = r.workingDir
	cmd.Env = append(shellEnv(), extraEnv...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &stdout, limit: maxCommandOutput}
	cmd.Stderr = &limitedWriter{w: &stderr, limit: maxCommandOutput}

	if runErr := cmd.Run(); runErr != nil {
		var exitErr *exec.ExitError
		if asExitError(runErr, &exitErr) {
			return NewTextResponse(commandResult{
				Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: exitErr.ExitCode(),
			}.String()), nil
		}
		if ctx.Err() != nil {
			return NewTextErrorResponse(fmt.Sprintf("%s timed out", name)), nil
		}
		return NewTextErrorResponse(fmt.Sprintf("%s failed: %v", name, runErr)), nil
	}

	return NewTextResponse(commandResult{Stdout: stdout.String(), Stderr: stderr.String()}.String()), nil
}

// shellEnv is the inherited environment for a child process.
func shellEnv() []string { return os.Environ() }

// limitedWriter keeps the first `limit` bytes and silently drops the rest, so
// a runaway build log cannot exhaust memory.
type limitedWriter struct {
	w       *bytes.Buffer
	limit   int
	written int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.written >= l.limit {
		// Report the full length so the writer above does not error out; the
		// bytes are simply discarded.
		return len(p), nil
	}
	room := l.limit - l.written
	if len(p) > room {
		l.w.Write(p[:room])
		l.written = l.limit
		return len(p), nil
	}
	n, err := l.w.Write(p)
	l.written += n
	return n, err
}

// lookPath resolves a program name, tolerating the ".exe" suffix Windows tools
// often ship without it.
func lookPath(name string) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(name, ".exe") {
		if path, err := exec.LookPath(name + ".exe"); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("%q is not installed or not on PATH", name)
}

// asExitError is errors.As specialised for *exec.ExitError, kept in one place
// so the runner stays readable.
func asExitError(err error, target **exec.ExitError) bool {
	for err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
