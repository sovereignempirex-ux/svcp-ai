//go:build !windows

// The bridge, the embedded assets and the authentication all build and are
// tested on every platform, so the command line works everywhere. Only the
// window host is Windows-specific.
package gui

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/svpc-ai/svpc/internal/app"
)

// ErrUnsupported is returned by RunWindow where there is no native window host.
var ErrUnsupported = errUnsupported{}

// RunWindow is a no-op stub for non-Windows builds. The bridge is unaffected,
// so `--serve` still works: that is how the Android client and a phone browser
// reach the agent from anywhere.
func RunWindow(workingDir string, debug bool, url string) error { return ErrUnsupported }

type errUnsupported struct{}

func (errUnsupported) Error() string {
	return "SVPC AI: the desktop window is only available on Windows; " +
		"use --serve to reach the same interface from another device"
}

// unusedCore keeps the signature of ServeRemote honest on platforms where the
// window is a stub but the bridge is not.
var _ = func(ctx context.Context, conn *sql.DB, a *app.App, err error) (string, func(), error) {
	return "", nil, fmt.Errorf("unreachable")
}

var _ = errors.Is
