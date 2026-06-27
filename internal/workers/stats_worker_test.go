package workers

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/db"
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

func TestStatsWorkerRecomputeRangesLoadsRawRowsOnce(t *testing.T) {
	userID := uuid.New()
	store := &countingStatsStore{
		user: db.User{ID: userID, Timezone: "UTC", TimeoutMinutes: 15},
	}
	worker := StatsWorker{Store: store}

	if err := worker.RecomputeRanges(context.Background(), userID, []string{"last_7_days", "last_30_days"}, 15, false); err != nil {
		t.Fatalf("RecomputeRanges returned error: %v", err)
	}
	if store.allHeartbeatsCalls != 1 {
		t.Fatalf("expected one all-heartbeats load, got %d", store.allHeartbeatsCalls)
	}
	if store.externalCalls != 1 {
		t.Fatalf("expected one external-duration load, got %d", store.externalCalls)
	}
	if store.costCalls != 1 {
		t.Fatalf("expected one AI cost load, got %d", store.costCalls)
	}
	if store.upsertCalls != 2 {
		t.Fatalf("expected one upsert per range, got %d", store.upsertCalls)
	}
}

func TestStatsWorkerDoesNotHideConcreteStoreRequirement(t *testing.T) {
	source, err := os.ReadFile("stats_worker.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "w.Store.(*db.Store)") {
		t.Fatal("StatsWorker must not silently type-assert a store interface back to *db.Store")
	}
}

type countingStatsStore struct {
	user               db.User
	allHeartbeatsCalls int
	externalCalls      int
	costCalls          int
	upsertCalls        int
}

func (s *countingStatsStore) UserByID(context.Context, uuid.UUID) (db.User, error) {
	return s.user, nil
}

func (s *countingStatsStore) AllHeartbeats(context.Context, uuid.UUID) ([]services.Heartbeat, error) {
	s.allHeartbeatsCalls++
	return nil, nil
}

func (s *countingStatsStore) ListExternalDurations(context.Context, uuid.UUID) ([]db.ExternalDuration, error) {
	s.externalCalls++
	return nil, nil
}

func (s *countingStatsStore) AICostRates(context.Context, uuid.UUID) (map[string]services.AICostRate, error) {
	s.costCalls++
	return nil, nil
}

func (s *countingStatsStore) UpsertStatsCache(context.Context, uuid.UUID, string, services.Stats) error {
	s.upsertCalls++
	return nil
}

func (s *countingStatsStore) HeartbeatsForStatsRange(context.Context, uuid.UUID, time.Time, string) ([]services.Heartbeat, error) {
	return nil, nil
}

func (s *countingStatsStore) ExternalDurationsBetween(context.Context, uuid.UUID, time.Time, time.Time) ([]db.ExternalDuration, error) {
	return nil, nil
}
