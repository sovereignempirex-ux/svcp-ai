package tools

import (
	"context"
	"strings"
	"testing"
)

func TestOpenTool_Info(t *testing.T) {
	info := NewOpenTool(nil).Info()

	if info.Name != OpenToolName {
		t.Errorf("expected name %q, got %q", OpenToolName, info.Name)
	}
	if info.Description == "" {
		t.Error("description should not be empty")
	}
	if info.Parameters == nil {
		t.Fatal("parameters should not be nil")
	}
}

func TestOpenTool_Run_Validation(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"invalid json", `{invalid json`},
		{"no target", `{}`},
		{"both empty", `{"url": "", "path": ""}`},
	}

	tool := NewOpenTool(nil)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tool.Run(context.Background(), ToolCall{
				ID: "test", Name: OpenToolName, Input: tc.input,
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

// needsApproval decides whether a target is worth interrupting the user for.
func TestNeedsApprovalForOpen(t *testing.T) {
	cases := []struct {
		name   string
		params OpenParams
		want   bool
	}{
		// A plain web address is the common case and only opens a tab.
		{"https url", OpenParams{URL: "https://example.com"}, false},
		{"http url", OpenParams{URL: "http://localhost:3000"}, false},
		{"uppercase scheme", OpenParams{URL: "HTTPS://example.com"}, false},
		{"leading space is tolerated", OpenParams{URL: "  https://example.com"}, false},

		// A path is handed to the default handler, which is an executable.
		{"a path can execute", OpenParams{Path: "C:/Windows/System32/cmd.exe"}, true},
		{"an app name", OpenParams{Path: "code"}, true},
		{"a relative path", OpenParams{Path: "./notes.txt"}, true},

		// These all resolve through a registered handler, same as a path.
		{"a file url", OpenParams{URL: "file:///C:/secret.txt"}, true},
		{"a custom scheme", OpenParams{URL: "steam://run/440"}, true},
		{"a javascript url", OpenParams{URL: "javascript:alert(1)"}, true},
		{"a scheme-less string", OpenParams{URL: "example.com"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := needsApproval(tc.params); got != tc.want {
				t.Errorf("needsApproval = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestOpenTool_AsksBeforeLaunching proves a refused request never reaches the
// system handler, which is the whole point: "open" can run a program.
func TestOpenTool_AsksBeforeLaunching(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"an application", `{"path": "calc.exe"}`},
		{"a script", `{"path": "/tmp/thing.sh"}`},
		{"a file url", `{"url": "file:///C:/Windows/System32/cmd.exe"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := NewOpenTool(denyPermissions{}).Run(context.Background(), ToolCall{
				ID: "test", Name: OpenToolName, Input: tc.input,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.IsError {
				t.Error("a refused open must report an error")
			}
			if !strings.Contains(strings.ToLower(res.Content), "denied") {
				t.Errorf("the error should say the request was denied, got %q", res.Content)
			}
		})
	}
}
