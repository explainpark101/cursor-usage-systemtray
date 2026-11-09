//go:build windows

// cursor-tray: system-tray-only Cursor usage monitor for Windows.
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"cursor-tray/internal/cursorapi"
	"cursor-tray/internal/poller"
	"cursor-tray/internal/settings"
	"cursor-tray/internal/storage"
	"cursor-tray/internal/tray"
	"cursor-tray/internal/version"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	// Log to file only; the app is tray-only and must never open a console.
	if exe, err := os.Executable(); err == nil {
		if f, err := os.OpenFile(
			filepath.Join(filepath.Dir(exe), "cursor-tray.log"),
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600,
		); err == nil {
			log.SetOutput(f)
			defer f.Close()
		}
	}
	log.Printf("cursor-tray %s starting", version.Version)

	store, err := storage.Open()
	if err != nil {
		log.Printf("storage open failed: %v", err)
		os.Exit(1)
	}

	exePath, _ := os.Executable()
	webviewDataPath := filepath.Join(store.Dir, "webview")
	loginWebViewDataPath := filepath.Join(store.Dir, "webview-login")

	events := make(chan poller.Event, 4)
	p := &poller.Poller{
		Out:         events,
		Cookie:      store.Cookie,
		MinInterval: time.Duration(store.Config().PollIntervalSeconds) * time.Second,
		MaxInterval: 5 * time.Minute,
	}

	t := tray.New()

	var (
		snapMu sync.RWMutex
		snap   settings.UsageInfo
	)
	setSnap := func(u *cursorapi.PlanUsage, err error, unauthorized bool) {
		snapMu.Lock()
		defer snapMu.Unlock()
		snap.Usage = u
		snap.Unauthorized = unauthorized
		if err != nil && !unauthorized {
			snap.Err = err.Error()
		} else if unauthorized {
			snap.Err = "세션이 유효하지 않습니다. 다시 로그인하세요."
		} else {
			snap.Err = ""
		}
	}
	getSnap := func() settings.UsageInfo {
		snapMu.RLock()
		defer snapMu.RUnlock()
		return snap
	}

	t.OnRefresh = func() {
		c := store.Config()
		p.SetMinInterval(time.Duration(c.PollIntervalSeconds) * time.Second)
		p.RefreshNow()
	}

	settingsOpen := false
	t.OnSettings = func() {
		if settingsOpen {
			return
		}
		settingsOpen = true
		go func() {
			// WebView2 requires a dedicated STA OS thread. Without LockOSThread
			// the goroutine can migrate mid-COM call and freeze the whole app.
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			defer func() { settingsOpen = false }()
			err := settings.Open(settings.Deps{
				Store:                store,
				ExePath:              exePath,
				WebViewDataPath:      webviewDataPath,
				LoginWebViewDataPath: loginWebViewDataPath,
				Snapshot:             getSnap,
				OnCookieChanged: func() {
					// Clear stale "login required" immediately. Otherwise
					// SetBusy(false) after login reapplies the pre-login
					// Unauthorized snapshot and the tray looks stuck.
					if store.Cookie() == "" {
						t.Update(tray.State{Unauthorized: true})
					} else {
						t.Update(tray.State{Fetching: true})
					}
					c := store.Config()
					p.SetMinInterval(time.Duration(c.PollIntervalSeconds) * time.Second)
					p.RefreshNow()
				},
				OnLoginStart: func() { t.SetBusy(true) },
				OnLoginEnd:   func() { t.SetBusy(false) },
			})
			if err != nil {
				log.Printf("settings window failed: %v", err)
			}
		}()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.OnQuit = func() {
		p.Stop()
		cancel()
	}

	go p.Run(ctx)
	go func() {
		for ev := range events {
			setSnap(ev.Usage, ev.Err, ev.Unauthorized)
			t.Update(tray.State{
				Usage:        ev.Usage,
				Err:          ev.Err,
				Unauthorized: ev.Unauthorized,
			})
		}
	}()

	// Tray first fetch on startup.
	p.RefreshNow()
	t.Run(nil)
}
