package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/svpc-ai/svpc/internal/config"
)

func TestGetContextFromPaths(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	_, err := config.Load(tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	cfg := config.Get()
	cfg.WorkingDir = tmpDir
	cfg.ContextPaths = []string{
		"file.txt",
		"directory/",
	}
	testFiles := []string{
		"file.txt",
		"directory/file_a.txt",
		"directory/file_b.txt",
		"directory/file_c.txt",
	}

	createTestFiles(t, tmpDir, testFiles)

	context := getContextFromPaths()

	// The context is built with filepath.Join, so the expected paths have to use
	// the platform separator too — hardcoding "/" only works on Unix.
	from := func(rel string) string {
		return "# From:" + filepath.Join(tmpDir, filepath.FromSlash(rel))
	}
	expectedContext := strings.Join([]string{
		from("file.txt"), "file.txt: test content",
		from("directory/file_a.txt"), "directory/file_a.txt: test content",
		from("directory/file_b.txt"), "directory/file_b.txt: test content",
		from("directory/file_c.txt"), "directory/file_c.txt: test content",
	}, "\n")

	assert.Equal(t, expectedContext, context)
}

func createTestFiles(t *testing.T, tmpDir string, testFiles []string) {
	t.Helper()
	for _, path := range testFiles {
		fullPath := filepath.Join(tmpDir, path)
		if path[len(path)-1] == '/' {
			err := os.MkdirAll(fullPath, 0755)
			require.NoError(t, err)
		} else {
			dir := filepath.Dir(fullPath)
			err := os.MkdirAll(dir, 0755)
			require.NoError(t, err)
			err = os.WriteFile(fullPath, []byte(path+": test content"), 0644)
			require.NoError(t, err)
		}
	}
}
