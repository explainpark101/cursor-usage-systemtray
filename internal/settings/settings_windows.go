//go:build windows

package settings

import (
	"context"
	"errors"
	"log"
	"os"
	"runtime"
	"sync"

	"cursor-tray/internal/auth"
	"cursor-tray/internal/autostart"
	"cursor-tray/internal/cursorapi"
	"cursor-tray/internal/webview2"
	win32 "cursor-tray/internal/win32"
)

const appName = "cursor-tray"

var openMu sync.Mutex

type usageState struct {
	TotalPercent float64 `json:"totalPercent"`
	AutoPercent  float64 `json:"autoPercent"`
	APIPercent   float64 `json:"apiPercent"`
}

type state struct {
	LoggedIn            bool        `json:"loggedIn"`
	PollIntervalSeconds int         `json:"pollIntervalSeconds"`
	AutoStart           bool        `json:"autoStart"`
	Usage               *usageState `json:"usage"`
	UsageError          string      `json:"usageError"`
}

func openWindow(d Deps) error {
	openMu.Lock()
	defer openMu.Unlock()

	w, err := webview2.New(webview2.Options{
		Title:    "Cursor Tray 설정",
		Width:    460,
		Height:   680,
		Center:   true,
		DataPath: d.WebViewDataPath,
	})
	if err != nil {
		return err
	}
	w.SetSize(460, 680, webview2.HintFixed)

	toUsageState := func(u *cursorapi.PlanUsage) *usageState {
		if u == nil {
			return nil
		}
		return &usageState{
			TotalPercent: u.TotalPercentUsed,
			AutoPercent:  u.AutoPercentUsed,
			APIPercent:   u.APIPercentUsed,
		}
	}

	// Fast path: no network — used for initial paint.
	cachedState := func() state {
		c := d.Store.Config()
		cookie := d.Store.Cookie()
		s := state{
			LoggedIn:            cookie != "",
			PollIntervalSeconds: c.PollIntervalSeconds,
			AutoStart:           autostart.Enabled(appName, d.ExePath),
		}
		if d.Snapshot != nil {
			snap := d.Snapshot()
			s.Usage = toUsageState(snap.Usage)
			s.UsageError = snap.Err
			if cookie != "" && snap.Unauthorized {
				s.LoggedIn = false
				if s.UsageError == "" {
					s.UsageError = "세션이 유효하지 않습니다. 다시 로그인하세요."
				}
			}
		}
		return s
	}

	liveState := func() state {
		s := cachedState()
		cookie := d.Store.Cookie()
		if cookie == "" {
			s.LoggedIn = false
			s.Usage = nil
			s.UsageError = ""
			return s
		}
		ctx, cancel := context.WithTimeout(context.Background(), cursorapi.Timeout)
		defer cancel()
		u, err := cursorapi.Fetch(ctx, cookie)
		if err != nil {
			s.Usage = nil
			if errors.Is(err, cursorapi.ErrUnauthorized) {
				s.LoggedIn = false
				s.UsageError = "세션이 유효하지 않습니다. 다시 로그인하세요."
			} else {
				s.UsageError = err.Error()
			}
			return s
		}
		s.LoggedIn = true
		s.Usage = toUsageState(u)
		s.UsageError = ""
		return s
	}

	notify := func() {
		if d.OnCookieChanged != nil {
			d.OnCookieChanged()
		}
	}

	loginDataPath := d.LoginWebViewDataPath
	if loginDataPath == "" {
		loginDataPath = d.WebViewDataPath + "-login"
	}

	if err := w.Bind("getState", func() state { return cachedState() }); err != nil {
		return err
	}
	if err := w.Bind("refreshUsage", func() state { return liveState() }); err != nil {
		return err
	}
	if err := w.Bind("setPollInterval", func(seconds int) error {
		if seconds < 5 || seconds > 3600 {
			return errors.New("5~3600초 사이로 입력하세요")
		}
		c := d.Store.Config()
		c.PollIntervalSeconds = seconds
		return d.Store.SetConfig(c)
	}); err != nil {
		return err
	}
	if err := w.Bind("setAutoStart", func(enabled bool) error {
		return autostart.SetEnabled(appName, d.ExePath, enabled)
	}); err != nil {
		return err
	}
	if err := w.Bind("login", func() (state, error) {
		if d.OnLoginStart != nil {
			d.OnLoginStart()
		}
		defer func() {
			if d.OnLoginEnd != nil {
				d.OnLoginEnd()
			}
		}()

		type outcome struct {
			res auth.Result
			err error
		}
		ch := make(chan outcome, 1)
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			r, err := auth.LoginFlow(loginDataPath)
			ch <- outcome{r, err}
		}()
		out := <-ch

		if out.err != nil {
			if errors.Is(out.err, auth.ErrCancelled) {
				return cachedState(), nil
			}
			log.Printf("settings: login failed: %v", out.err)
			return cachedState(), out.err
		}
		if err := d.Store.SetCookie(out.res.CookieHeader); err != nil {
			return cachedState(), err
		}
		notify()
		return liveState(), nil
	}); err != nil {
		return err
	}
	if err := w.Bind("logout", func() state {
		_ = d.Store.SetCookie("")
		notify()
		return cachedState()
	}); err != nil {
		return err
	}
	if err := w.Bind("saveCookie", func(cookie string) (state, error) {
		header, err := auth.ParseCookieInput(cookie)
		if err != nil {
			return cachedState(), err
		}
		ctx, cancel := context.WithTimeout(context.Background(), cursorapi.Timeout)
		defer cancel()
		u, err := cursorapi.Fetch(ctx, header)
		if err != nil {
			if errors.Is(err, cursorapi.ErrUnauthorized) {
				return cachedState(), errors.New("쿠키가 거부되었습니다. WorkosCursorSessionToken 값을 확인하세요")
			}
			return cachedState(), err
		}
		if err := d.Store.SetCookie(header); err != nil {
			return cachedState(), err
		}
		notify()
		s := cachedState()
		s.LoggedIn = true
		s.Usage = toUsageState(u)
		s.UsageError = ""
		return s, nil
	}); err != nil {
		return err
	}
	type pickCookieResult struct {
		Cancelled bool   `json:"cancelled"`
		Content   string `json:"content"`
	}
	if err := w.Bind("pickCookieFile", func() (pickCookieResult, error) {
		path, ok := win32.PickOpenFile(w.Window(), "cookies.txt 선택", [][2]string{
			{"Cookie / Text Files (*.txt)", "*.txt"},
			{"All Files (*.*)", "*.*"},
		}, "txt")
		if !ok {
			return pickCookieResult{Cancelled: true}, nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return pickCookieResult{}, err
		}
		return pickCookieResult{Content: string(b)}, nil
	}); err != nil {
		return err
	}
	if err := w.Bind("closeWindow", func() { w.Destroy() }); err != nil {
		return err
	}

	w.SetHTML(pageHTML)
	w.Run()
	return nil
}
