// Package cursorapi talks to the undocumented cursor.com dashboard usage APIs.
package cursorapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	PeriodUsageURL  = "https://cursor.com/api/dashboard/get-current-period-usage"
	UsageSummaryURL = "https://cursor.com/api/usage-summary"
	PlanInfoURL     = "https://cursor.com/api/dashboard/get-plan-info"
	Timeout         = 8 * time.Second
	maxBody         = 256 << 10
)

// PlanUsage mirrors the planUsage / summary percent fields used by the tray icon.
type PlanUsage struct {
	TotalPercentUsed float64 `json:"totalPercentUsed"`
	AutoPercentUsed  float64 `json:"autoPercentUsed"`
	APIPercentUsed   float64 `json:"apiPercentUsed"`
}

// Snapshot is the combined dashboard view for the tray and settings UI.
type Snapshot struct {
	PlanUsage PlanUsage

	MembershipType string
	PlanName       string
	PlanPrice      string
	DisplayMessage string

	BillingCycleStart time.Time
	BillingCycleEnd   time.Time // subscription / included-usage renewal instant

	// Spending — current-cycle dollar amounts (USD cents).
	TotalSpendCents    int64
	IncludedSpendCents int64
	BonusSpendCents    int64
	PlanLimitCents     int64

	// Included usage pool details.
	IncludedEnabled        bool
	IncludedUsedCents      int64
	IncludedLimitCents     int64
	IncludedRemainingCents int64
	IncludedBonusCents     int64
	IncludedBreakdownTotal int64

	// On-demand (usage-based) pool.
	OnDemandEnabled        bool
	OnDemandUsedCents      int64
	OnDemandLimitCents     *int64 // nil = no cap / not set
	OnDemandRemainingCents *int64
}

// Remaining returns how long until BillingCycleEnd from now (clamped at zero).
func (s *Snapshot) Remaining(now time.Time) time.Duration {
	if s == nil || s.BillingCycleEnd.IsZero() {
		return 0
	}
	d := s.BillingCycleEnd.Sub(now)
	if d < 0 {
		return 0
	}
	return d
}

// EstimatedCapacityCents estimates the full usage pool in USD cents from
// current Total spend and Total percent used:
//
//	capacity ≈ usedSpend / (percentUsed / 100)
//
// Returns ok=false when percent is too small to invert reliably.
func (s *Snapshot) EstimatedCapacityCents() (cents int64, ok bool) {
	if s == nil {
		return 0, false
	}
	return EstimateCapacityCents(s.TotalSpendCents, s.PlanUsage.TotalPercentUsed)
}

// EstimateCapacityCents computes capacityCents = usedCents * 100 / percentUsed.
func EstimateCapacityCents(usedCents int64, percentUsed float64) (cents int64, ok bool) {
	if percentUsed < 0.01 || usedCents < 0 {
		return 0, false
	}
	return int64(math.Round(float64(usedCents) * 100.0 / percentUsed)), true
}

var ErrUnauthorized = errors.New("cursorapi: unauthorized (HTTP 401/403)")

// Fetch requests the dashboard snapshot using the given session cookie header.
func Fetch(ctx context.Context, cookie string) (*Snapshot, error) {
	if cookie == "" {
		return nil, errors.New("cursorapi: no cookie configured")
	}

	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	var (
		wg      sync.WaitGroup
		summary *usageSummaryResponse
		period  *periodUsageResponse
		plan    *planInfoResponse
		sumErr  error
		perErr  error
		planErr error
	)

	wg.Add(3)
	go func() {
		defer wg.Done()
		summary, sumErr = getJSON[usageSummaryResponse](ctx, http.MethodGet, UsageSummaryURL, cookie, nil)
	}()
	go func() {
		defer wg.Done()
		period, perErr = getJSON[periodUsageResponse](ctx, http.MethodPost, PeriodUsageURL, cookie, []byte("{}"))
	}()
	go func() {
		defer wg.Done()
		plan, planErr = getJSON[planInfoResponse](ctx, http.MethodPost, PlanInfoURL, cookie, []byte("{}"))
	}()
	wg.Wait()

	// Auth failures on any primary endpoint win.
	for _, err := range []error{sumErr, perErr, planErr} {
		if errors.Is(err, ErrUnauthorized) {
			return nil, ErrUnauthorized
		}
	}
	// Summary is required; period/plan enrich when available.
	if sumErr != nil {
		return nil, sumErr
	}
	if summary == nil {
		return nil, errors.New("cursorapi: empty usage-summary")
	}
	if perErr != nil {
		log.Printf("cursorapi: period usage enrich failed: %v", perErr)
	}
	if planErr != nil {
		log.Printf("cursorapi: plan info enrich failed: %v", planErr)
	}

	return buildSnapshot(summary, period, plan), nil
}

func buildSnapshot(summary *usageSummaryResponse, period *periodUsageResponse, plan *planInfoResponse) *Snapshot {
	s := &Snapshot{
		MembershipType: summary.MembershipType,
		DisplayMessage: firstNonEmpty(
			periodMessage(period),
			summary.AutoModelSelectedDisplayMessage,
		),
	}

	if start, ok := summary.BillingCycleStart.Time(); ok {
		s.BillingCycleStart = start
	}
	if end, ok := summary.BillingCycleEnd.Time(); ok {
		s.BillingCycleEnd = end
	}

	if plan != nil && plan.PlanInfo != nil {
		s.PlanName = plan.PlanInfo.PlanName
		s.PlanPrice = plan.PlanInfo.Price
		if t, ok := plan.PlanInfo.RenewsAt.Time(); ok {
			s.BillingCycleEnd = t
		} else if t, ok := plan.PlanInfo.BillingCycleEnd.Time(); ok {
			s.BillingCycleEnd = t
		}
	}

	if period != nil {
		if start, ok := period.BillingCycleStart.Time(); ok && s.BillingCycleStart.IsZero() {
			s.BillingCycleStart = start
		}
		if end, ok := period.BillingCycleEnd.Time(); ok {
			// Prefer period end when summary lacked one; otherwise keep renewsAt if set.
			if s.BillingCycleEnd.IsZero() {
				s.BillingCycleEnd = end
			}
		}
		if pu := period.PlanUsage; pu != nil {
			s.TotalSpendCents = pu.TotalSpend
			s.IncludedSpendCents = pu.IncludedSpend
			s.BonusSpendCents = pu.BonusSpend
			s.PlanLimitCents = pu.Limit
			s.PlanUsage = PlanUsage{
				TotalPercentUsed: pu.TotalPercentUsed,
				AutoPercentUsed:  pu.AutoPercentUsed,
				APIPercentUsed:   pu.APIPercentUsed,
			}
		}
		if slu := period.SpendLimitUsage; slu != nil {
			// Individual on-demand caps when present on the period payload.
			if slu.IndividualUsed != nil {
				s.OnDemandUsedCents = *slu.IndividualUsed
			}
			if slu.IndividualLimit != nil {
				s.OnDemandLimitCents = slu.IndividualLimit
			}
			if slu.IndividualRemaining != nil {
				s.OnDemandRemainingCents = slu.IndividualRemaining
			}
		}
	}

	if iu := summary.IndividualUsage; iu != nil {
		if p := iu.Plan; p != nil {
			s.IncludedEnabled = p.Enabled
			s.IncludedUsedCents = p.Used
			s.IncludedLimitCents = p.Limit
			s.IncludedRemainingCents = p.Remaining
			if p.Breakdown != nil {
				s.IncludedBonusCents = p.Breakdown.Bonus
				s.IncludedBreakdownTotal = p.Breakdown.Total
				if s.IncludedSpendCents == 0 {
					s.IncludedSpendCents = p.Breakdown.Included
				}
				if s.BonusSpendCents == 0 {
					s.BonusSpendCents = p.Breakdown.Bonus
				}
				if s.TotalSpendCents == 0 {
					s.TotalSpendCents = p.Breakdown.Total
				}
			}
			if s.PlanLimitCents == 0 {
				s.PlanLimitCents = p.Limit
			}
			// Summary percents are authoritative when present.
			s.PlanUsage = PlanUsage{
				TotalPercentUsed: p.TotalPercentUsed,
				AutoPercentUsed:  p.AutoPercentUsed,
				APIPercentUsed:   p.APIPercentUsed,
			}
		}
		if od := iu.OnDemand; od != nil {
			s.OnDemandEnabled = od.Enabled
			s.OnDemandUsedCents = od.Used
			s.OnDemandLimitCents = od.Limit
			s.OnDemandRemainingCents = od.Remaining
		}
	}

	return s
}

func periodMessage(period *periodUsageResponse) string {
	if period == nil {
		return ""
	}
	return firstNonEmpty(period.DisplayMessage, period.AutoModelSelectedDisplayMessage)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

type usageSummaryResponse struct {
	BillingCycleStart                flexibleTime `json:"billingCycleStart"`
	BillingCycleEnd                  flexibleTime `json:"billingCycleEnd"`
	MembershipType                   string       `json:"membershipType"`
	AutoModelSelectedDisplayMessage  string       `json:"autoModelSelectedDisplayMessage"`
	NamedModelSelectedDisplayMessage string       `json:"namedModelSelectedDisplayMessage"`
	IndividualUsage                  *struct {
		Plan *struct {
			Enabled          bool    `json:"enabled"`
			Used             int64   `json:"used"`
			Limit            int64   `json:"limit"`
			Remaining        int64   `json:"remaining"`
			AutoPercentUsed  float64 `json:"autoPercentUsed"`
			APIPercentUsed   float64 `json:"apiPercentUsed"`
			TotalPercentUsed float64 `json:"totalPercentUsed"`
			Breakdown        *struct {
				Included int64 `json:"included"`
				Bonus    int64 `json:"bonus"`
				Total    int64 `json:"total"`
			} `json:"breakdown"`
		} `json:"plan"`
		OnDemand *struct {
			Enabled   bool   `json:"enabled"`
			Used      int64  `json:"used"`
			Limit     *int64 `json:"limit"`
			Remaining *int64 `json:"remaining"`
		} `json:"onDemand"`
	} `json:"individualUsage"`
}

type periodUsageResponse struct {
	BillingCycleStart               flexibleTime `json:"billingCycleStart"`
	BillingCycleEnd                 flexibleTime `json:"billingCycleEnd"`
	DisplayMessage                  string       `json:"displayMessage"`
	AutoModelSelectedDisplayMessage string       `json:"autoModelSelectedDisplayMessage"`
	PlanUsage                       *struct {
		TotalSpend       int64   `json:"totalSpend"`
		IncludedSpend    int64   `json:"includedSpend"`
		BonusSpend       int64   `json:"bonusSpend"`
		Limit            int64   `json:"limit"`
		AutoPercentUsed  float64 `json:"autoPercentUsed"`
		APIPercentUsed   float64 `json:"apiPercentUsed"`
		TotalPercentUsed float64 `json:"totalPercentUsed"`
	} `json:"planUsage"`
	SpendLimitUsage *struct {
		LimitType           string `json:"limitType"`
		IndividualLimit     *int64 `json:"individualLimit"`
		IndividualUsed      *int64 `json:"individualUsed"`
		IndividualRemaining *int64 `json:"individualRemaining"`
		TotalSpend          *int64 `json:"totalSpend"`
	} `json:"spendLimitUsage"`
}

type planInfoResponse struct {
	PlanInfo *struct {
		PlanName        string       `json:"planName"`
		Price           string       `json:"price"`
		BillingCycleEnd flexibleTime `json:"billingCycleEnd"`
		RenewsAt        flexibleTime `json:"renewsAt"`
	} `json:"planInfo"`
}

// flexibleTime accepts ISO-8601 strings, decimal epoch-ms strings, or JSON numbers.
type flexibleTime struct {
	raw json.RawMessage
}

func (f flexibleTime) Time() (time.Time, bool) {
	if len(f.raw) == 0 || string(f.raw) == "null" {
		return time.Time{}, false
	}
	var s string
	if err := json.Unmarshal(f.raw, &s); err == nil {
		return parseTimeString(s)
	}
	var n float64
	if err := json.Unmarshal(f.raw, &n); err == nil {
		return epochMillis(n), true
	}
	return time.Time{}, false
}

func (f *flexibleTime) UnmarshalJSON(b []byte) error {
	f.raw = append(f.raw[:0], b...)
	return nil
}

func parseTimeString(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return epochMillis(n), true
	}
	return time.Time{}, false
}

func epochMillis(n float64) time.Time {
	ms := int64(n)
	return time.UnixMilli(ms).UTC()
}

func getJSON[T any](ctx context.Context, method, url, cookie string, body []byte) (*T, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Origin", "https://cursor.com")
	req.Header.Set("Referer", "https://cursor.com/dashboard")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Cookie", cookie)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		msg := strings.TrimSpace(string(raw))
		if msg != "" {
			log.Printf("cursorapi: HTTP %d %s: %s", resp.StatusCode, url, truncate(msg, 200))
		}
		if strings.Contains(msg, "Invalid origin") {
			return nil, fmt.Errorf("cursorapi: rejected by CSRF origin check: %s", truncate(msg, 120))
		}
		return nil, ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cursorapi: HTTP %d %s: %s", resp.StatusCode, url, truncate(string(raw), 120))
	}

	var data T
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("cursorapi: decode %s: %w", url, err)
	}
	return &data, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// FormatUSD formats integer USD cents as a dollar string (e.g. 7000 → "$70.00").
func FormatUSD(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	dollars := cents / 100
	frac := cents % 100
	s := fmt.Sprintf("$%d.%02d", dollars, frac)
	if neg {
		return "-" + s
	}
	return s
}

// FormatUSDPtr formats an optional cents pointer; nil → "—".
func FormatUSDPtr(cents *int64) string {
	if cents == nil {
		return "—"
	}
	return FormatUSD(*cents)
}

// FormatEstimatedCapacity formats an estimated capacity, or "—" when unknown.
func FormatEstimatedCapacity(usedCents int64, percentUsed float64) string {
	cents, ok := EstimateCapacityCents(usedCents, percentUsed)
	if !ok {
		return "—"
	}
	return FormatUSD(cents)
}

// FormatRemaining formats a duration as "Dd Hh Mm Ss" (days/hours/minutes/seconds).
func FormatRemaining(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalSec := int64(d / time.Second)
	days := totalSec / (60 * 60 * 24)
	hours := (totalSec % (60 * 60 * 24)) / (60 * 60)
	mins := (totalSec % (60 * 60)) / 60
	secs := totalSec % 60
	return fmt.Sprintf("%dd %dh %dm %ds", days, hours, mins, secs)
}

// FormatLocalTime formats t in the local timezone.
func FormatLocalTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("2006-01-02 15:04")
}
