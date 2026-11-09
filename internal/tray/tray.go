// Package tray glues the usage state to the Windows system tray icon.
package tray

import (
	"fmt"
	"sync"
	"time"

	"fyne.io/systray"

	"cursor-tray/internal/cursorapi"
)

// State describes what the tray should currently show.
type State struct {
	Usage        *cursorapi.PlanUsage
	Err          error
	Unauthorized bool
	Fetching     bool
}

// Tray wraps the systray lifecycle and menu.
type Tray struct {
	OnRefresh  func()
	OnSettings func()
	OnQuit     func()

	mu      sync.Mutex
	state   State
	busy    bool // login / other blocking UI work
	spinN   int
	spinStop chan struct{}

	mUsage    *systray.MenuItem
	mRefresh  *systray.MenuItem
	mSettings *systray.MenuItem
	mStatus   *systray.MenuItem
}

// New creates the tray wrapper. Call Run from the main goroutine.
func New() *Tray { return &Tray{} }

// Run starts the tray event loop (blocks). onReady runs once initialized.
func (t *Tray) Run(onReady func()) {
	systray.Run(func() {
		systray.SetTooltip("Cursor Tray — 연결 중…")
		systray.SetIcon(renderIcon("--", -1, idleColor))

		t.mStatus = systray.AddMenuItem("상태: 초기화 중…", "현재 상태")
		t.mStatus.Disable()
		systray.AddSeparator()
		t.mUsage = systray.AddMenuItem("사용량: —", "최근 사용량")
		t.mUsage.Disable()
		t.mRefresh = systray.AddMenuItem("새로고침", "지금 사용량을 다시 가져옵니다")
		t.mSettings = systray.AddMenuItem("설정…", "설정 창 열기")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("종료", "Cursor Tray를 종료합니다")

		go func() {
			for {
				select {
				case <-t.mRefresh.ClickedCh:
					if t.OnRefresh != nil {
						t.OnRefresh()
					}
				case <-t.mSettings.ClickedCh:
					if t.OnSettings != nil {
						t.OnSettings()
					}
				case <-mQuit.ClickedCh:
					if t.OnQuit != nil {
						t.OnQuit()
					}
					systray.Quit()
					return
				}
			}
		}()

		if onReady != nil {
			onReady()
		}
	}, nil)
}

// SetBusy shows an animated spinner in the tray (e.g. during login).
// When cleared, the latest Update state is applied (callers should refresh
// that state before clearing busy after a successful login).
func (t *Tray) SetBusy(busy bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.busy == busy {
		return
	}
	t.busy = busy
	if busy {
		t.startSpinnerLocked()
		return
	}
	t.stopSpinnerLocked()
	t.applyLocked(t.state)
}

// Update refreshes the icon, tooltip and menu labels.
// While busy, the state is still stored so SetBusy(false) can apply a
// post-login Fetching/usage snapshot instead of a stale Unauthorized one.
func (t *Tray) Update(s State) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state = s
	if t.busy {
		return
	}
	t.applyLocked(s)
}

func (t *Tray) startSpinnerLocked() {
	t.stopSpinnerLocked()
	stop := make(chan struct{})
	t.spinStop = stop
	t.spinN = 0
	systray.SetTooltip("Cursor Tray — 로그인 중…")
	if t.mStatus != nil {
		t.mStatus.SetTitle("상태: 로그인 중…")
	}
	systray.SetIcon(renderSpinnerFrame(0))
	go func() {
		ticker := time.NewTicker(90 * time.Millisecond)
		defer ticker.Stop()
		n := 0
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				n = (n + 1) % spinnerFrames
				systray.SetIcon(renderSpinnerFrame(n))
				systray.SetTooltip("Cursor Tray — 로그인 중…")
			}
		}
	}()
}

func (t *Tray) stopSpinnerLocked() {
	if t.spinStop != nil {
		close(t.spinStop)
		t.spinStop = nil
	}
}

func (t *Tray) applyLocked(s State) {
	tooltip := "Cursor Tray"
	iconText := "--"
	barPercent := -1
	iconColor := idleColor
	status := "초기화 중…"
	usageLabel := "사용량: —"

	switch {
	case s.Fetching:
		tooltip = "Cursor Tray — 가져오는 중…"
		status = "상태: 가져오는 중…"
	case s.Unauthorized:
		tooltip = "Cursor Tray — 로그인 필요"
		iconText = "!"
		iconColor = errorColor
		status = "상태: 로그인 필요 (설정에서 로그인)"
		usageLabel = "사용량: 로그인 필요"
	case s.Err != nil:
		tooltip = fmt.Sprintf("Cursor Tray — 오류: %v", s.Err)
		iconText = "!"
		iconColor = errorColor
		status = fmt.Sprintf("상태: 오류 (%v)", s.Err)
	case s.Usage != nil:
		u := s.Usage
		pct := int(u.TotalPercentUsed)
		tooltip = fmt.Sprintf("Cursor 사용량\nTotal %.1f%%  ·  Auto %.1f%%  ·  API %.1f%%",
			u.TotalPercentUsed, u.AutoPercentUsed, u.APIPercentUsed)
		iconText = fmt.Sprintf("%d", pct)
		barPercent = pct
		iconColor = fgColor
		status = fmt.Sprintf("상태: 정상 (Total %.1f%%)", u.TotalPercentUsed)
		usageLabel = fmt.Sprintf("사용량: Total %.1f%% · Auto %.1f%% · API %.1f%%",
			u.TotalPercentUsed, u.AutoPercentUsed, u.APIPercentUsed)
	default:
		tooltip = "Cursor Tray — 가져오는 중…"
		status = "상태: 가져오는 중…"
	}

	systray.SetTooltip(tooltip)
	systray.SetIcon(renderIcon(iconText, barPercent, iconColor))
	if t.mStatus != nil {
		t.mStatus.SetTitle(status)
	}
	if t.mUsage != nil {
		t.mUsage.SetTitle(usageLabel)
	}
}
