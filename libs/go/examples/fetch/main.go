// Example: fetch Cursor dashboard snapshot with a Cookie header or cookies.txt.
//
//	CURSOR_COOKIE='WorkosCursorSessionToken=...' go run ./examples/fetch
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"cursor-api/cursorapi"
)

func main() {
	raw := os.Getenv("CURSOR_COOKIE")
	if raw == "" && len(os.Args) > 1 {
		b, err := os.ReadFile(os.Args[1])
		if err != nil {
			fatal(err)
		}
		raw = string(b)
	}
	if raw == "" {
		fatal(fmt.Errorf("set CURSOR_COOKIE or pass a cookies.txt path"))
	}

	cookie, err := cursorapi.ParseCookieInput(raw)
	if err != nil {
		fatal(err)
	}

	snap, err := cursorapi.Fetch(context.Background(), cookie)
	if err != nil {
		fatal(err)
	}

	fmt.Printf("plan=%s (%s)\n", snap.PlanName, snap.PlanPrice)
	fmt.Printf("renew=%s\n", cursorapi.FormatLocalTime(snap.BillingCycleEnd))
	fmt.Printf("remain=%s\n", cursorapi.FormatRemaining(snap.Remaining(time.Now())))
	fmt.Printf("total=%.1f%% auto=%.1f%% api=%.1f%%\n",
		snap.PlanUsage.TotalPercentUsed, snap.PlanUsage.AutoPercentUsed, snap.PlanUsage.APIPercentUsed)
	fmt.Printf("spending=%s (included %s + bonus %s)\n",
		cursorapi.FormatUSD(snap.TotalSpendCents),
		cursorapi.FormatUSD(snap.IncludedSpendCents),
		cursorapi.FormatUSD(snap.BonusSpendCents))
	fmt.Printf("included=%s / %s remaining=%s\n",
		cursorapi.FormatUSD(snap.IncludedUsedCents),
		cursorapi.FormatUSD(snap.IncludedLimitCents),
		cursorapi.FormatUSD(snap.IncludedRemainingCents))
	fmt.Printf("onDemand enabled=%v used=%s\n",
		snap.OnDemandEnabled, cursorapi.FormatUSD(snap.OnDemandUsedCents))
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
