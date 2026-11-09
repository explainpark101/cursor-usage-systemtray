//go:build windows

package auth

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"cursor-tray/internal/cursorapi"
	"cursor-tray/internal/webview2"
	"cursor-tray/internal/webview2/edge"
)

const loginURL = "https://cursor.com/login"

var loginMu sync.Mutex

// loginFlow opens the WebView2 login window and polls for a working session
// cookie after every navigation. Blocks until success or window close.
//
// dataPath must be a WebView2 user-data folder that is NOT currently used by
// another WebView2 instance (e.g. the settings window).
func loginFlow(dataPath string) (Result, error) {
	loginMu.Lock()
	defer loginMu.Unlock()

	resCh := make(chan Result, 1)
	done := make(chan struct{})
	var closed bool
	var mu sync.Mutex
	setClosed := func() {
		mu.Lock()
		if !closed {
			closed = true
			close(done)
		}
		mu.Unlock()
	}

	w, err := webview2.New(webview2.Options{
		Title:     "Cursor 로그인 — cursor-tray",
		Width:     960,
		Height:    720,
		Center:    true,
		DataPath:  dataPath,
		AutoFocus: true,
		OnClose:   setClosed,
	})
	if err != nil {
		log.Printf("auth: login webview create failed: %v (dataPath=%s)", err, dataPath)
		return Result{}, err
	}

	w.Chromium().NavigationCompletedCallback = func(_ *edge.ICoreWebView2, _ *edge.ICoreWebView2NavigationCompletedEventArgs) {
		// Debounce: cookie state settles right after navigation.
		select {
		case <-done:
			return
		case <-time.After(800 * time.Millisecond):
		}
		w.Chromium().GetAllCookiesDevTools(func(jsonStr string, err error) {
			if err != nil {
				return
			}
			header := cursorCookieHeader(jsonStr)
			if header == "" {
				return
			}
			// Verify the cookie actually authorizes the usage API.
			ctx, cancel := context.WithTimeout(context.Background(), cursorapi.Timeout)
			defer cancel()
			if _, err := cursorapi.Fetch(ctx, header); err != nil {
				return
			}
			select {
			case resCh <- Result{CookieHeader: header}:
			default:
			}
			setClosed()
			w.Destroy()
		})
	}

	w.Navigate(loginURL)
	w.Run()
	setClosed()

	select {
	case r := <-resCh:
		return r, nil
	default:
		return Result{}, ErrCancelled
	}
}

type devToolsCookie struct {
	Cookies []struct {
		Name   string `json:"name"`
		Value  string `json:"value"`
		Domain string `json:"domain"`
	} `json:"cookies"`
}

// cursorCookieHeader builds a Cookie header from the DevTools cookie JSON,
// keeping only cursor.com cookies. Returns "" when none exist.
func cursorCookieHeader(jsonStr string) string {
	var list devToolsCookie
	if err := json.Unmarshal([]byte(jsonStr), &list); err != nil {
		return ""
	}
	var parts []string
	for _, c := range list.Cookies {
		if isCursorDomain(c.Domain) {
			parts = append(parts, c.Name+"="+c.Value)
		}
	}
	return strings.Join(parts, "; ")
}
