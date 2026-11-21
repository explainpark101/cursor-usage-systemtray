package cursorapi

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFlexibleTimeISOAndEpoch(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want time.Time
	}{
		{
			name: "iso",
			raw:  `"2026-11-21T16:15:29.000Z"`,
			want: time.Date(2026, 11, 21, 16, 15, 29, 0, time.UTC),
		},
		{
			name: "epoch_ms_string",
			raw:  `"1795277729000"`,
			want: time.UnixMilli(1795277729000).UTC(),
		},
		{
			name: "epoch_ms_number",
			raw:  `1795277729000`,
			want: time.UnixMilli(1795277729000).UTC(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ft flexibleTime
			if err := json.Unmarshal([]byte(tc.raw), &ft); err != nil {
				t.Fatal(err)
			}
			got, ok := ft.Time()
			if !ok {
				t.Fatal("expected ok")
			}
			if !got.Equal(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestBuildSnapshotMergesSummaryPeriodPlan(t *testing.T) {
	summaryRaw := `{
	  "billingCycleStart": "2026-10-21T16:15:29.000Z",
	  "billingCycleEnd": "2026-11-21T16:15:29.000Z",
	  "membershipType": "pro_plus",
	  "individualUsage": {
	    "plan": {
	      "enabled": true,
	      "used": 7000,
	      "limit": 7000,
	      "remaining": 0,
	      "breakdown": {"included": 7000, "bonus": 75693, "total": 82693},
	      "autoPercentUsed": 68.9,
	      "apiPercentUsed": 0,
	      "totalPercentUsed": 67.3
	    },
	    "onDemand": {"enabled": false, "used": 0, "limit": null, "remaining": null}
	  }
	}`
	periodRaw := `{
	  "billingCycleStart": "1792599329000",
	  "billingCycleEnd": "1795277729000",
	  "planUsage": {
	    "totalSpend": 82693,
	    "includedSpend": 7000,
	    "bonusSpend": 75693,
	    "limit": 7000,
	    "autoPercentUsed": 68.9,
	    "apiPercentUsed": 0,
	    "totalPercentUsed": 67.3
	  },
	  "displayMessage": "You've hit your usage limit"
	}`
	planRaw := `{
	  "planInfo": {
	    "planName": "Pro+",
	    "price": "$60/mo",
	    "renewsAt": "1795277729000"
	  }
	}`

	var summary usageSummaryResponse
	var period periodUsageResponse
	var plan planInfoResponse
	if err := json.Unmarshal([]byte(summaryRaw), &summary); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(periodRaw), &period); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(planRaw), &plan); err != nil {
		t.Fatal(err)
	}

	s := buildSnapshot(&summary, &period, &plan)
	if s.PlanName != "Pro+" || s.PlanPrice != "$60/mo" {
		t.Fatalf("plan meta: %+v", s)
	}
	if s.IncludedSpendCents != 7000 || s.BonusSpendCents != 75693 || s.TotalSpendCents != 82693 {
		t.Fatalf("spend: %+v", s)
	}
	if s.OnDemandEnabled {
		t.Fatal("on-demand should be disabled")
	}
	if s.PlanUsage.TotalPercentUsed < 67 || s.PlanUsage.TotalPercentUsed > 68 {
		t.Fatalf("percent: %v", s.PlanUsage.TotalPercentUsed)
	}
	wantEnd := time.UnixMilli(1795277729000).UTC()
	if !s.BillingCycleEnd.Equal(wantEnd) {
		t.Fatalf("renewal got %v want %v", s.BillingCycleEnd, wantEnd)
	}
	if s.DisplayMessage == "" {
		t.Fatal("expected display message")
	}
}

func TestFormatUSDAndRemaining(t *testing.T) {
	if got := FormatUSD(7000); got != "$70.00" {
		t.Fatalf("FormatUSD: %s", got)
	}
	if got := FormatUSD(75693); got != "$756.93" {
		t.Fatalf("FormatUSD bonus: %s", got)
	}
	d := 26*time.Hour + 5*time.Minute
	if got := FormatRemaining(d); got != "1d 2h 5m" {
		t.Fatalf("FormatRemaining: %s", got)
	}
}
