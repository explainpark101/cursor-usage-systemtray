// Package cursorapi talks to the cursor.com usage dashboard endpoint.
package cursorapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	APIURL  = "https://cursor.com/api/dashboard/get-current-period-usage"
	Timeout = 5 * time.Second
)

// PlanUsage mirrors the planUsage object returned by the endpoint.
type PlanUsage struct {
	TotalPercentUsed float64 `json:"totalPercentUsed"`
	AutoPercentUsed  float64 `json:"autoPercentUsed"`
	APIPercentUsed   float64 `json:"apiPercentUsed"`
}

type response struct {
	PlanUsage *PlanUsage `json:"planUsage"`
}

var ErrUnauthorized = errors.New("cursorapi: unauthorized (HTTP 401/403)")

// Fetch requests current usage using the given session cookie header value.
func Fetch(ctx context.Context, cookie string) (*PlanUsage, error) {
	if cookie == "" {
		return nil, errors.New("cursorapi: no cookie configured")
	}

	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, APIURL, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	// Cursor rejects POSTs without a cursor.com Origin (CSRF check → HTTP 403).
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://cursor.com")
	req.Header.Set("Referer", "https://cursor.com/dashboard")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Cookie", cookie)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		msg := strings.TrimSpace(string(body))
		if msg != "" {
			log.Printf("cursorapi: HTTP %d: %s", resp.StatusCode, truncate(msg, 200))
		}
		// Distinguish CSRF misconfig (should be rare after Origin is set) from bad session.
		if strings.Contains(msg, "Invalid origin") {
			return nil, fmt.Errorf("cursorapi: rejected by CSRF origin check: %s", truncate(msg, 120))
		}
		return nil, ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cursorapi: HTTP %d: %s", resp.StatusCode, truncate(string(body), 120))
	}

	var data response
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("cursorapi: decode: %w", err)
	}
	if data.PlanUsage == nil {
		return nil, errors.New("cursorapi: planUsage is missing")
	}
	return data.PlanUsage, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
