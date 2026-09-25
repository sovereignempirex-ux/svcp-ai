package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/svpc-ai/svpc/cmd"
	"github.com/svpc-ai/svpc/internal/logging"
)

func main() {
	defer logging.RecoverPanic("main", func() {
		logging.ErrorPersist("Application terminated due to unhandled panic")
	})

	// A double-click in Explorer passes no arguments, which is the common way
	// people launch a desktop app. In that case open the window instead of
	// dropping them into an interactive TUI they did not ask for. An explicit
	// argument (any flag, or the "tui" keyword) always wins.
	if len(os.Args) == 1 {
		if launchedFromExplorer() {
			os.Args = append(os.Args, "--gui")
		}
	}

	cmd.Execute()
}

// launchedFromExplorer reports whether the process was started by double
// clicking the executable rather than typed in a shell. Explorer launches the
// binary with its working directory set to the file's own folder, while a shell
// inherits the user's current directory.
func launchedFromExplorer() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	exeDir, err := filepath.Abs(filepath.Dir(exe))
	if err != nil {
		return false
	}

	wd, err := os.Getwd()
	if err != nil {
		return false
	}
	wd, err = filepath.Abs(wd)
	if err != nil {
		return false
	}

	return strings.EqualFold(exeDir, wd)
}
