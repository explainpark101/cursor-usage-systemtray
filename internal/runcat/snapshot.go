// Package runcat writes Cursor usage snapshots in the RunCat Neo
// Custom Metrics JSON schema.
//
// Schema reference:
// https://github.com/runcat-dev/RunCatNeo/blob/main/docs/CustomMetricsSchema.md
package runcat

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"cursor-tray/internal/cursorapi"
)

const (
	// DefaultTitle is the card header shown in RunCat Neo.
	DefaultTitle = "Cursor"
	// DefaultSymbol is an SF Symbol identifier next to the title.
	DefaultSymbol = "sparkles"
)

// Metric is one row inside a Custom Metrics card.
type Metric struct {
	Title           string   `json:"title"`
	FormattedValue  string   `json:"formattedValue"`
	NormalizedValue *float64 `json:"normalizedValue,omitempty"`
}

// Snapshot is the top-level Custom Metrics document RunCat Neo watches.
type Snapshot struct {
	Title           string   `json:"title"`
	Symbol          string   `json:"symbol,omitempty"`
	MetricsBarValue string   `json:"metricsBarValue,omitempty"`
	Metrics         []Metric `json:"metrics"`
	LastUpdatedDate string   `json:"lastUpdatedDate"`
}

// FromUsage builds a Snapshot from a successful Cursor planUsage response.
func FromUsage(u *cursorapi.PlanUsage) Snapshot {
	now := time.Now().UTC().Format(time.RFC3339)
	if u == nil {
		return StatusSnapshot("no usage data", now)
	}
	return Snapshot{
		Title:           DefaultTitle,
		Symbol:          DefaultSymbol,
		MetricsBarValue: formatPercent(u.TotalPercentUsed),
		Metrics: []Metric{
			percentMetric("Total", u.TotalPercentUsed),
			percentMetric("Auto", u.AutoPercentUsed),
			percentMetric("API", u.APIPercentUsed),
		},
		LastUpdatedDate: now,
	}
}

// StatusSnapshot builds a text-only card (no progress bars) for error /
// unauthorized / idle states so the Metrics Bar still updates.
func StatusSnapshot(message, lastUpdated string) Snapshot {
	if lastUpdated == "" {
		lastUpdated = time.Now().UTC().Format(time.RFC3339)
	}
	return Snapshot{
		Title:           DefaultTitle,
		Symbol:          DefaultSymbol,
		MetricsBarValue: "---",
		Metrics: []Metric{
			{Title: "Status", FormattedValue: message},
		},
		LastUpdatedDate: lastUpdated,
	}
}

// WriteAtomic marshals s as strict JSON and replaces path via temp file + rename
// so RunCat Neo never observes a half-written file.
func WriteAtomic(path string, s Snapshot) error {
	if path == "" {
		return fmt.Errorf("runcat: empty output path")
	}
	if s.Title == "" {
		return fmt.Errorf("runcat: title is required")
	}
	if s.Metrics == nil {
		s.Metrics = []Metric{}
	}
	if s.LastUpdatedDate == "" {
		s.LastUpdatedDate = time.Now().UTC().Format(time.RFC3339)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, ".runcat-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func percentMetric(title string, pct float64) Metric {
	n := clamp01(pct / 100)
	return Metric{
		Title:           title,
		FormattedValue:  formatPercent(pct),
		NormalizedValue: &n,
	}
}

func formatPercent(pct float64) string {
	if math.Abs(pct-math.Round(pct)) < 0.05 {
		return fmt.Sprintf("%.0f%%", math.Round(pct))
	}
	return fmt.Sprintf("%.1f%%", pct)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// DefaultOutputPath returns the default metrics file location under the
// given store directory (typically Application Support/cursor-tray).
func DefaultOutputPath(storeDir string) string {
	if v := os.Getenv("RUNCAT_OUT_FILE"); v != "" {
		return v
	}
	return filepath.Join(storeDir, "runcat-usage.json")
}
