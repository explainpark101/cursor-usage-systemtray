// Package auth implements the cursor.com login flow: a WebView2 window is
// opened, the user signs in, and once the session cookie works against the
// usage API the cookie header value is returned.
//
// Windows-only (build-tagged files).
package auth

import "errors"

// ErrCancelled is returned when the user closes the login window without
// completing login.
var ErrCancelled = errors.New("auth: login cancelled")

// CookieHeader as sent in the HTTP Cookie header (e.g. "a=1; b=2").
type Result struct {
	CookieHeader string
}

// LoginFlow runs the interactive login and returns a verified cookie header.
// It must be called from a goroutine that can host a window message loop, and
// only one instance may run at a time (guarded internally).
func LoginFlow(dataPath string) (Result, error) {
	return loginFlow(dataPath)
}
