package workers

import (
	"testing"
	"time"

	"github.com/keithah/stint/internal/services"
)

func TestComputeWorkerStatsIncludesExternalDurationsForRange(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	external := []services.ExternalDuration{{
		StartTime: float64(now.Add(-30 * time.Minute).Unix()),
		EndTime:   float64(now.Unix()),
		Project:   "external-docs",
		Language:  "Markdown",
	}}

	stats, err := computeWorkerStats("last_7_days", nil, external, now, 15*time.Minute, nil)
	if err != nil {
		t.Fatalf("computeWorkerStats returned error: %v", err)
	}

	assertSliceTotal(t, stats.Projects, "external-docs", 1800)
	assertSliceTotal(t, stats.Languages, "Markdown", 1800)
}

func TestComputeWorkerStatsIncludesExternalDurationsForAllTime(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	external := []services.ExternalDuration{{
		StartTime: float64(now.Add(-45 * time.Minute).Unix()),
		EndTime:   float64(now.Unix()),
		Project:   "external-planning",
	}}

	stats, err := computeWorkerStats("all_time", nil, external, now, 15*time.Minute, nil)
	if err != nil {
		t.Fatalf("computeWorkerStats returned error: %v", err)
	}

	assertSliceTotal(t, stats.Projects, "external-planning", 2700)
}

func assertSliceTotal(t *testing.T, totals []services.SliceTotal, name string, seconds int) {
	t.Helper()
	for _, total := range totals {
		if total.Name == name {
			if total.TotalSeconds != seconds {
				t.Fatalf("%s: expected %d seconds, got %d", name, seconds, total.TotalSeconds)
			}
			return
		}
	}
	t.Fatalf("expected totals to include %s, got %#v", name, totals)
}

func TestShouldSkipCustomRulesApplyForAbortedProgress(t *testing.T) {
	if !shouldSkipCustomRulesApply("Aborted") {
		t.Fatal("expected aborted custom-rules progress to skip worker application")
	}
	if shouldSkipCustomRulesApply("Queued") {
		t.Fatal("expected queued custom-rules progress to run worker application")
	}
}
