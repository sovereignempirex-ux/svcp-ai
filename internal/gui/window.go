//go:build windows

package gui

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"

	"github.com/jchv/go-webview2"
)

// errWebView2Missing is returned when the runtime cannot create a window.
var errWebView2Missing = errors.New("SVPC AI: the Microsoft Edge WebView2 runtime is required. " +
	"Install it from https://developer.microsoft.com/microsoft-edge/webview2/ and try again")

// enableDPIAwareness makes the process per-monitor DPI aware so the WebView2
// content and the native chrome share one coordinate space. Without it Windows
// stretches the bitmap, which cuts off the right edge of the layout on any
// display that is not at 100%.
func enableDPIAwareness() {
	user32 := syscall.NewLazyDLL("user32.dll")
	shcore := syscall.NewLazyDLL("shcore.dll")

	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 is (DWORD)-4.
	if proc := user32.NewProc("SetProcessDpiAwarenessContext"); proc.Find() == nil {
		if ret, _, _ := proc.Call(^uintptr(3)); ret != 0 {
			return
		}
	}
	// PROCESS_PER_MONITOR_DPI_AWARE (2)
	if proc := shcore.NewProc("SetProcessDpiAwareness"); proc.Find() == nil {
		if ret, _, _ := proc.Call(2); ret == 0 {
			return
		}
	}
	// Fall back to the system-wide flag for older Windows builds.
	user32.NewProc("SetProcessDPIAware").Call()
}

// applyWindowIcon sets the SVPC AI mark on the given window.
//
// The icon is read straight from the executable's own resource section
// (group RT_GROUP_ICON / id 1, embedded at build time), so nothing has to be
// unpacked to disk.
//
// Note: SetIcon is a macro in winuser.h, not an exported function, so the
// icon is installed with WM_SETICON through SendMessageW.
func applyWindowIcon(hwnd uintptr) {
	if hwnd == 0 {
		return
	}

	user32 := syscall.NewLazyDLL("user32.dll")
	kernel32 := syscall.NewLazyDLL("kernel32.dll")

	getModuleHandle := kernel32.NewProc("GetModuleHandleW")
	loadImageW := user32.NewProc("LoadImageW")
	sendMessageW := user32.NewProc("SendMessageW")
	getMetrics := user32.NewProc("GetSystemMetrics")

	instance, _, _ := getModuleHandle.Call(0)
	if instance == 0 {
		return
	}

	const (
		imageIcon = 1
		iconID    = 1 // MAKEINTRESOURCE(1)

		iconSmall  = 0 // ICON_SMALL
		iconBig    = 1 // ICON_BIG
		iconSmall2 = 2 // ICON_SMALL2

		wmSetIcon = 0x0080

		smCxSmallIcon = 49
		smCySmallIcon = 50
	)

	// MAKEINTRESOURCE(1) is just the integer 1.
	res := uintptr(iconID)

	// 32x32 class icon: taskbar button, Alt+Tab, and the large title-bar glyph.
	if big, _, _ := loadImageW.Call(instance, res, imageIcon, 0, 0, 0); big != 0 {
		sendMessageW.Call(hwnd, wmSetIcon, iconBig, big)
	}

	cx, _, _ := getMetrics.Call(uintptr(smCxSmallIcon))
	cy, _, _ := getMetrics.Call(uintptr(smCySmallIcon))

	// 16x16 class icon: the small glyph in the title bar.
	if small, _, _ := loadImageW.Call(instance, res, imageIcon, cx, cy, 0); small != 0 {
		sendMessageW.Call(hwnd, wmSetIcon, iconSmall, small)
		sendMessageW.Call(hwnd, wmSetIcon, iconSmall2, small)
	}

	// The shell caches the taskbar entry; nudge it so the new icon shows up.
	shell32 := syscall.NewLazyDLL("shell32.dll")
	notifyIconChange := shell32.NewProc("SHChangeNotify")
	const (
		shcneAssocChanged = 0x08000000
		shcnfFlush        = 0x1000
	)
	notifyIconChange.Call(shcneAssocChanged, shcnfFlush, 0, 0)
}

// Run opens the SVPC AI desktop window and blocks until the user closes it.
//
// It boots the same application core the terminal UI uses, so every tool, the
// language servers and the session store are available in the window too. A
// missing provider is not fatal: the window still opens and the user can
// configure one from its own settings panel.
func Run(workingDir string, debug bool) error {
	enableDPIAwareness()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sess, err := bootstrap(ctx, workingDir, debug)
	if err != nil {
		return err
	}
	defer sess.Close()

	url, shutdown, err := serve(ctx, sess.Conn, sess.Core, sess.SetupErr)
	if err != nil {
		return err
	}
	defer shutdown()

	profile := tempProfileDir()
	defer os.RemoveAll(profile)

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		DataPath:  profile,
		WindowOptions: webview2.WindowOptions{
			Title:  "SVPC AI",
			Width:  1280,
			Height: 820,
			Center: true,
		},
	})
	if w == nil {
		// WebView2 runtime is missing: tell the user what to install instead of
		// failing with a native error.
		return errWebView2Missing
	}
	defer w.Destroy()

	w.Navigate(url)

	// The native window is created synchronously by NewWithOptions, but the
	// shell may still be initialising; apply the icon on the UI thread and
	// once more shortly after start-up to be safe.
	if hwnd := uintptr(w.Window()); hwnd != 0 {
		w.Dispatch(func() { applyWindowIcon(hwnd) })
		go func() {
			time.Sleep(600 * time.Millisecond)
			w.Dispatch(func() { applyWindowIcon(hwnd) })
		}()
	}

	w.Run()
	return nil
}
