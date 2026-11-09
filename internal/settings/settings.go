// Package settings implements the settings window: login state, login flow,
// manual cookie entry, polling interval, and autostart toggle.
//
// Windows-only (build-tagged files).
package settings

import (
	"cursor-tray/internal/cursorapi"
	"cursor-tray/internal/storage"
)

// UsageInfo is a tray/poller snapshot shown immediately when settings opens
// so the UI does not wait on a network round-trip.
type UsageInfo struct {
	Usage        *cursorapi.PlanUsage
	Err          string
	Unauthorized bool
}

// Deps supplies the settings UI with everything it needs.
type Deps struct {
	Store *storage.Store

	// ExePath is this program's executable path (for autostart registration).
	ExePath string

	// WebViewDataPath is the WebView2 profile folder for the settings UI.
	WebViewDataPath string

	// LoginWebViewDataPath is a separate WebView2 profile folder for the
	// interactive login window. It must differ from WebViewDataPath so two
	// WebView2 instances are not fighting over the same user-data folder.
	LoginWebViewDataPath string

	// Snapshot returns the latest polled usage without hitting the network.
	Snapshot func() UsageInfo

	// OnCookieChanged is called after login/logout/manual cookie save so the
	// poller can refresh immediately.
	OnCookieChanged func()

	// OnLoginStart / OnLoginEnd bookend the WebView2 login flow so the tray
	// can show a busy spinner while the user authenticates.
	OnLoginStart func()
	OnLoginEnd   func()
}

// Open shows the settings window and blocks until it is closed. Must be
// called from a goroutine that can host a window message loop. Only one
// settings window may be open at a time (guarded internally).
func Open(d Deps) error {
	return openWindow(d)
}
