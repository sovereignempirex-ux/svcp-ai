// Package gui implements the SVPC AI desktop application.
//
// The window hosts a premium chat interface built on the WebView2 runtime that
// ships with Windows 10/11.
//
// The backend is not a thin model proxy: it boots the same application core the
// terminal UI uses, so the desktop client gets every tool, the language servers
// and persistent sessions for free. A local HTTP server bridges the window to
// that core over loopback, and the API key never leaves the process.
//
// Only the window host is platform-specific. The bridge in server.go is plain
// Go and is tested on every platform, so the logic the window depends on is
// covered even where WebView2 is unavailable.
package gui
