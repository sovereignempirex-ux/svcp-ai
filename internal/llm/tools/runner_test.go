package tools

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestCommandResultString(t *testing.T) {
	cases := []struct {
		name string
		in   commandResult
		want []string
		none []string
	}{
		{
			name: "stdout only",
			in:   commandResult{Stdout: "built ok\n"},
			want: []string{"built ok"},
			none: []string{"exit code"},
		},
		{
			name: "stderr included",
			in:   commandResult{Stderr: "cannot find module"},
			want: []string{"cannot find module"},
		},
		{
			name: "exit code reported",
			in:   commandResult{Stderr: "boom", ExitCode: 2},
			want: []string{"boom", "[exit code 2]"},
		},
		{
			name: "empty output",
			in:   commandResult{},
			want: []string{"(no output)"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.String()
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("output missing %q:\n%s", w, got)
				}
			}
			for _, n := range tc.none {
				if strings.Contains(got, n) {
					t.Errorf("output should not contain %q:\n%s", n, got)
				}
			}
		})
	}
}

func TestLimitedWriter(t *testing.T) {
	var buf bytes.Buffer
	w := &limitedWriter{w: &buf, limit: 10}

	n, err := w.Write([]byte("12345"))
	if err != nil || n != 5 {
		t.Fatalf("first write = %d, %v", n, err)
	}

	// This write straddles the limit and must be truncated, not dropped.
	n, err = w.Write([]byte("67890abc"))
	if err != nil || n != 8 {
		t.Fatalf("second write = %d, %v; want the full length reported", n, err)
	}
	if got := buf.String(); got != "1234567890" {
		t.Errorf("buffer = %q, want %q", got, "1234567890")
	}

	// Once full, writes are discarded but still report success.
	before := buf.Len()
	n, err = w.Write([]byte("more"))
	if err != nil || n != 4 {
		t.Fatalf("overflow write = %d, %v", n, err)
	}
	if buf.Len() != before {
		t.Error("overflow write should not append anything")
	}
}

func TestRunnerReportsMissingBinary(t *testing.T) {
	r := newRunner(nil)
	_, err := r.exec(context.Background(), "definitely-not-a-real-binary-xyz")
	if err == nil {
		t.Fatal("expected an error for a missing binary")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("error = %q, want it to mention the missing binary", err)
	}
}

func TestRunnerCapturesExitCode(t *testing.T) {
	if _, err := lookPath("go"); err != nil {
		t.Skipf("go is not on PATH: %v", err)
	}

	// A directory of the test's own, because the process-wide configuration
	// may point somewhere that no longer exists.
	r := newRunner(nil)
	res, err := r.execIn(context.Background(), t.TempDir(), "go", "env", "GOOS")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("go env exited %d: %s", res.ExitCode, res.String())
	}
	if strings.TrimSpace(res.Stdout) == "" {
		t.Error("go env should have printed a value")
	}
}

func TestSessionContextIsSafeWithoutValues(t *testing.T) {
	session, message := sessionContext(context.Background())
	if session != "" || message != "" {
		t.Errorf("expected empty identifiers, got %q / %q", session, message)
	}
}
