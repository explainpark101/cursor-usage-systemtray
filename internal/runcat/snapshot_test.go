package runcat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"cursor-tray/internal/cursorapi"
)

func TestFromUsage(t *testing.T) {
	s := FromUsage(&cursorapi.PlanUsage{
		TotalPercentUsed: 42.0,
		AutoPercentUsed:  35.5,
		APIPercentUsed:   8.0,
	})
	if s.Title != DefaultTitle {
		t.Fatalf("title: got %q", s.Title)
	}
	if s.MetricsBarValue != "42%" {
		t.Fatalf("metricsBarValue: got %q", s.MetricsBarValue)
	}
	if len(s.Metrics) != 3 {
		t.Fatalf("metrics len: got %d", len(s.Metrics))
	}
	if s.Metrics[0].NormalizedValue == nil || *s.Metrics[0].NormalizedValue != 0.42 {
		t.Fatalf("total normalized: %+v", s.Metrics[0].NormalizedValue)
	}
	if s.Metrics[1].FormattedValue != "35.5%" {
		t.Fatalf("auto formatted: got %q", s.Metrics[1].FormattedValue)
	}
	if s.LastUpdatedDate == "" {
		t.Fatal("lastUpdatedDate empty")
	}
}

func TestWriteAtomicRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runcat-usage.json")

	in := FromUsage(&cursorapi.PlanUsage{
		TotalPercentUsed: 10,
		AutoPercentUsed:  5,
		APIPercentUsed:   1,
	})
	if err := WriteAtomic(path, in); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out Snapshot
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Title != in.Title || out.MetricsBarValue != in.MetricsBarValue {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
	if len(out.Metrics) != 3 {
		t.Fatalf("metrics: %+v", out.Metrics)
	}
}

func TestStatusSnapshotOmitsBars(t *testing.T) {
	s := StatusSnapshot("login required", "2026-01-01T00:00:00Z")
	if s.MetricsBarValue != "---" {
		t.Fatalf("bar: %q", s.MetricsBarValue)
	}
	if len(s.Metrics) != 1 || s.Metrics[0].NormalizedValue != nil {
		t.Fatalf("status metric should have no bar: %+v", s.Metrics)
	}
}

func TestDefaultOutputPathEnv(t *testing.T) {
	t.Setenv("RUNCAT_OUT_FILE", "/tmp/custom-cursor.json")
	got := DefaultOutputPath("/ignored")
	if got != "/tmp/custom-cursor.json" {
		t.Fatalf("got %q", got)
	}
}
