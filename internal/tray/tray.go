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
	Snapshot     *cursorapi.Snapshot
	Err          error
	Unauthorized bool
	Fetching     bool
}

// DisplayOpts controls which usage metric and icon style the tray uses.
type DisplayOpts struct {
	Percent string // total|auto|api
	Style   string // bar|circle|battery|number|colored
}

// Tray wraps the systray lifecycle and menu.
type Tray struct {
	OnRefresh  func()
	OnSettings func()
	OnQuit     func()

	mu       sync.Mutex
	state    State
	opts     DisplayOpts
	busy     bool // login / other blocking UI work
	spinN    int
	spinStop chan struct{}
	tickStop chan struct{}

	mStatus    *systray.MenuItem
	mPlan      *systray.MenuItem
	mRenewal   *systray.MenuItem
	mCountdown *systray.MenuItem
	mUsage     *systray.MenuItem
	mSpending  *systray.MenuItem
	mCapacity  *systray.MenuItem
	mIncluded  *systray.MenuItem
	mIncluded2 *systray.MenuItem
	mOnDemand  *systray.MenuItem
	mRefresh   *systray.MenuItem
	mSettings  *systray.MenuItem
}

// New creates the tray wrapper. Call Run from the main goroutine.
func New() *Tray {
	return &Tray{opts: DisplayOpts{Percent: "total", Style: StyleBar}}
}

// SetDisplayOpts updates percent source / icon style and reapplies the
// current state so the tray icon refreshes immediately.
func (t *Tray) SetDisplayOpts(o DisplayOpts) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if o.Percent == "" {
		o.Percent = "total"
	}
	if o.Style == "" {
		o.Style = StyleBar
	}
	t.opts = o
	if t.busy {
		return
	}
	t.applyLocked(t.state)
}

// Run starts the tray event loop (blocks). onReady runs once initialized.
func (t *Tray) Run(onReady func()) {
	systray.Run(func() {
		t.mu.Lock()
		initStyle := t.opts.Style
		t.mu.Unlock()
		systray.SetTooltip("Cursor Tray — 연결 중…")
		systray.SetIcon(renderIcon("--", -1, idleColor, initStyle))

		// Info rows stay enabled so Windows draws them in normal (dark) text.
		// Their ClickedCh is drained but ignored — only action items do work.
		t.mStatus = systray.AddMenuItem("상태: 초기화 중…", "현재 상태")
		systray.AddSeparator()
		t.mPlan = systray.AddMenuItem("플랜: —", "구독 플랜")
		t.mRenewal = systray.AddMenuItem("갱신: —", "구독 갱신 시각")
		t.mCountdown = systray.AddMenuItem("남은 시간: —", "갱신까지 남은 시간")
		systray.AddSeparator()
		t.mUsage = systray.AddMenuItem("사용률: —", "Total / Auto / API 퍼센트")
		t.mSpending = systray.AddMenuItem("Spending: —", "현재 주기 총 사용 금액")
		t.mCapacity = systray.AddMenuItem("총 사용가능: —", "사용량%%와 달러로 추정한 총 사용가능 금액")
		t.mIncluded = systray.AddMenuItem("Included: —", "Included Usage")
		t.mIncluded2 = systray.AddMenuItem("  상세: —", "Included breakdown")
		t.mOnDemand = systray.AddMenuItem("On-Demand: —", "On Demand Usage")
		systray.AddSeparator()
		t.mRefresh = systray.AddMenuItem("새로고침", "지금 사용량을 다시 가져옵니다")
		t.mSettings = systray.AddMenuItem("설정…", "설정 창 열기")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("종료", "Cursor Tray를 종료합니다")

		go func() {
			for {
				select {
				case <-t.mStatus.ClickedCh:
				case <-t.mPlan.ClickedCh:
				case <-t.mRenewal.ClickedCh:
				case <-t.mCountdown.ClickedCh:
				case <-t.mUsage.ClickedCh:
				case <-t.mSpending.ClickedCh:
				case <-t.mCapacity.ClickedCh:
				case <-t.mIncluded.ClickedCh:
				case <-t.mIncluded2.ClickedCh:
				case <-t.mOnDemand.ClickedCh:
					// read-only info rows
				case <-t.mRefresh.ClickedCh:
					if t.OnRefresh != nil {
						t.OnRefresh()
					}
				case <-t.mSettings.ClickedCh:
					if t.OnSettings != nil {
						t.OnSettings()
					}
				case <-mQuit.ClickedCh:
					t.stopCountdownTicker()
					if t.OnQuit != nil {
						t.OnQuit()
					}
					systray.Quit()
					return
				}
			}
		}()

		t.startCountdownTicker()

		if onReady != nil {
			onReady()
		}
	}, nil)
}

// startCountdownTicker refreshes remaining-time labels every second.
func (t *Tray) startCountdownTicker() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopCountdownTickerLocked()
	stop := make(chan struct{})
	t.tickStop = stop
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				t.tickCountdown()
			}
		}
	}()
}

func (t *Tray) stopCountdownTicker() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopCountdownTickerLocked()
}

func (t *Tray) stopCountdownTickerLocked() {
	if t.tickStop != nil {
		close(t.tickStop)
		t.tickStop = nil
	}
}

func (t *Tray) tickCountdown() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.busy {
		return
	}
	s := t.state
	if s.Fetching || s.Unauthorized || s.Err != nil || s.Snapshot == nil {
		return
	}
	if s.Snapshot.BillingCycleEnd.IsZero() {
		return
	}
	t.applyCountdownLocked(s.Snapshot)
}

// applyCountdownLocked updates only labels that include live remaining time.
func (t *Tray) applyCountdownLocked(snap *cursorapi.Snapshot) {
	remainStr := cursorapi.FormatRemaining(snap.Remaining(time.Now()))
	renewAt := cursorapi.FormatLocalTime(snap.BillingCycleEnd)

	planBits := snap.PlanName
	if planBits == "" {
		planBits = snap.MembershipType
	}
	if snap.PlanPrice != "" {
		planBits = fmt.Sprintf("%s (%s)", planBits, snap.PlanPrice)
	}
	if planBits == "" {
		planBits = "—"
	}

	label, val := selectedPercent(snap.PlanUsage, t.opts.Percent)
	setTitle(t.mCountdown, "남은 시간: "+remainStr)
	setTitle(t.mStatus, fmt.Sprintf("상태: 정상 (%s %.1f%% · %s 남음)", label, val, remainStr))
	systray.SetTooltip(fmt.Sprintf("Cursor %s\n갱신 %s (%s 남음)\nTotal %.0f%% · Included %s/%s",
		planBits, renewAt, remainStr,
		snap.PlanUsage.TotalPercentUsed,
		cursorapi.FormatUSD(snap.IncludedUsedCents),
		cursorapi.FormatUSD(snap.IncludedLimitCents)))
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

func selectedPercent(u cursorapi.PlanUsage, which string) (label string, value float64) {
	switch which {
	case "auto":
		return "Auto", u.AutoPercentUsed
	case "api":
		return "API", u.APIPercentUsed
	default:
		return "Total", u.TotalPercentUsed
	}
}

func setTitle(m *systray.MenuItem, title string) {
	if m != nil {
		m.SetTitle(title)
	}
}

func (t *Tray) applyLocked(s State) {
	tooltip := "Cursor Tray"
	iconText := "--"
	barPercent := -1
	iconColor := idleColor
	status := "초기화 중…"
	style := t.opts.Style

	planLabel := "플랜: —"
	renewalLabel := "갱신: —"
	countdownLabel := "남은 시간: —"
	usageLabel := "사용률: —"
	spendingLabel := "Spending: —"
	capacityLabel := "총 사용가능: —"
	includedLabel := "Included: —"
	included2Label := "  상세: —"
	onDemandLabel := "On-Demand: —"

	switch {
	case s.Fetching:
		tooltip = "Cursor Tray — 가져오는 중…"
		status = "상태: 가져오는 중…"
	case s.Unauthorized:
		tooltip = "Cursor Tray — 로그인 필요"
		iconText = "!"
		iconColor = errorColor
		status = "상태: 로그인 필요 (설정에서 로그인)"
		usageLabel = "사용률: 로그인 필요"
	case s.Err != nil:
		tooltip = fmt.Sprintf("Cursor Tray — 오류: %v", s.Err)
		iconText = "!"
		iconColor = errorColor
		status = fmt.Sprintf("상태: 오류 (%v)", s.Err)
	case s.Snapshot != nil:
		snap := s.Snapshot
		u := snap.PlanUsage
		label, val := selectedPercent(u, t.opts.Percent)
		pct := int(val)
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}

		remain := snap.Remaining(time.Now())
		remainStr := cursorapi.FormatRemaining(remain)
		renewAt := cursorapi.FormatLocalTime(snap.BillingCycleEnd)

		planBits := snap.PlanName
		if planBits == "" {
			planBits = snap.MembershipType
		}
		if snap.PlanPrice != "" {
			planBits = fmt.Sprintf("%s (%s)", planBits, snap.PlanPrice)
		}
		if planBits == "" {
			planBits = "—"
		}
		planLabel = "플랜: " + planBits
		renewalLabel = "갱신: " + renewAt
		countdownLabel = "남은 시간: " + remainStr

		usageLabel = fmt.Sprintf("사용률: Total %.1f%% · Auto %.1f%% · API %.1f%%",
			u.TotalPercentUsed, u.AutoPercentUsed, u.APIPercentUsed)

		spendingLabel = fmt.Sprintf("Spending: %s (포함 %s + 보너스 %s)",
			cursorapi.FormatUSD(snap.TotalSpendCents),
			cursorapi.FormatUSD(snap.IncludedSpendCents),
			cursorapi.FormatUSD(snap.BonusSpendCents))

		capStr := cursorapi.FormatEstimatedCapacity(snap.TotalSpendCents, u.TotalPercentUsed)
		if cap, ok := snap.EstimatedCapacityCents(); ok {
			left := cap - snap.TotalSpendCents
			if left < 0 {
				left = 0
			}
			capacityLabel = fmt.Sprintf("총 사용가능: %s (사용 %s · %.1f%% · 추정잔여 %s)",
				capStr,
				cursorapi.FormatUSD(snap.TotalSpendCents),
				u.TotalPercentUsed,
				cursorapi.FormatUSD(left))
		} else {
			capacityLabel = "총 사용가능: —"
		}

		includedLabel = fmt.Sprintf("Included: %s / %s (남은 %s)",
			cursorapi.FormatUSD(snap.IncludedUsedCents),
			cursorapi.FormatUSD(snap.IncludedLimitCents),
			cursorapi.FormatUSD(snap.IncludedRemainingCents))
		included2Label = fmt.Sprintf("  상세: 포함 %s · 보너스 %s · 합계 %s",
			cursorapi.FormatUSD(snap.IncludedSpendCents),
			cursorapi.FormatUSD(snap.IncludedBonusCents),
			cursorapi.FormatUSD(snap.IncludedBreakdownTotal))

		if snap.OnDemandEnabled {
			onDemandLabel = fmt.Sprintf("On-Demand: %s / %s (남은 %s)",
				cursorapi.FormatUSD(snap.OnDemandUsedCents),
				cursorapi.FormatUSDPtr(snap.OnDemandLimitCents),
				cursorapi.FormatUSDPtr(snap.OnDemandRemainingCents))
		} else {
			onDemandLabel = fmt.Sprintf("On-Demand: 비활성 · 사용 %s",
				cursorapi.FormatUSD(snap.OnDemandUsedCents))
		}

		tooltip = fmt.Sprintf("Cursor %s\n갱신 %s (%s 남음)\nTotal %.0f%% · 사용 %s / 총가능 %s",
			planBits, renewAt, remainStr,
			u.TotalPercentUsed,
			cursorapi.FormatUSD(snap.TotalSpendCents),
			capStr)

		iconText = fmt.Sprintf("%d", pct)
		barPercent = pct
		iconColor = fgColor
		status = fmt.Sprintf("상태: 정상 (%s %.1f%% · %s 남음)", label, val, remainStr)
	default:
		tooltip = "Cursor Tray — 가져오는 중…"
		status = "상태: 가져오는 중…"
	}

	systray.SetTooltip(tooltip)
	systray.SetIcon(renderIcon(iconText, barPercent, iconColor, style))
	setTitle(t.mStatus, status)
	setTitle(t.mPlan, planLabel)
	setTitle(t.mRenewal, renewalLabel)
	setTitle(t.mCountdown, countdownLabel)
	setTitle(t.mUsage, usageLabel)
	setTitle(t.mSpending, spendingLabel)
	setTitle(t.mCapacity, capacityLabel)
	setTitle(t.mIncluded, includedLabel)
	setTitle(t.mIncluded2, included2Label)
	setTitle(t.mOnDemand, onDemandLabel)
}
