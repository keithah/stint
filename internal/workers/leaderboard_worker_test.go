package workers

import (
	"testing"
	"time"

	"github.com/keithah/stint/internal/services"
)

func TestComputeWorkerLeaderboardStatsIncludesExternalDurations(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	external := []services.ExternalDuration{{
		StartTime: float64(now.Add(-1 * time.Hour).Unix()),
		EndTime:   float64(now.Add(-30 * time.Minute).Unix()),
		Project:   "planning",
		Language:  "Markdown",
	}}

	stats, err := computeWorkerLeaderboardStats(nil, external, now, 15*time.Minute, "last_7_days")
	if err != nil {
		t.Fatalf("computeWorkerLeaderboardStats returned error: %v", err)
	}

	if stats.TotalSeconds != 1800 {
		t.Fatalf("expected leaderboard worker stats to include external duration, got %d", stats.TotalSeconds)
	}
}
