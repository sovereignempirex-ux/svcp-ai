//go:build !windows

// The bridge, the embedded assets and the agent bootstrap all build and are
// tested on every platform. Only the window host is Windows-specific, so on
// everything else this package compiles and Run reports why it cannot open.
package gui

// ErrUnsupported is returned by Run on platforms without a native window host.
var ErrUnsupported = errUnsupported{}

// Run is a no-op stub for non-Windows builds.
func Run(workingDir string, debug bool) error { return ErrUnsupported }

type errUnsupported struct{}

func (errUnsupported) Error() string {
	return "SVPC AI: the desktop client is only available on Windows"
}
