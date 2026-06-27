package workers

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/db"
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

func TestLeaderboardWorkerUsesBatchedRangeLoads(t *testing.T) {
	userID := uuid.New()
	store := &countingLeaderboardStore{
		users: []db.User{{
			ID:             userID,
			GitHubUsername: "leader",
			Timezone:       "UTC",
			TimeoutMinutes: 15,
		}},
	}
	worker := LeaderboardWorker{Store: store}

	entries, err := worker.Compute(context.Background(), "last_7_days")
	if err != nil {
		t.Fatalf("Compute returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one leaderboard entry, got %d", len(entries))
	}
	if store.heartbeatsCalls != 1 {
		t.Fatalf("expected one batched heartbeat load, got %d", store.heartbeatsCalls)
	}
	if store.externalCalls != 1 {
		t.Fatalf("expected one batched external duration load, got %d", store.externalCalls)
	}
}

type countingLeaderboardStore struct {
	users           []db.User
	heartbeatsCalls int
	externalCalls   int
}

func (s *countingLeaderboardStore) ListUsers(context.Context) ([]db.User, error) {
	return s.users, nil
}

func (s *countingLeaderboardStore) HeartbeatsForStatsRangeByUser(context.Context, []uuid.UUID, time.Time, string) (map[uuid.UUID][]services.Heartbeat, error) {
	s.heartbeatsCalls++
	return map[uuid.UUID][]services.Heartbeat{}, nil
}

func (s *countingLeaderboardStore) ExternalDurationsBetweenByUser(context.Context, []uuid.UUID, time.Time, time.Time) (map[uuid.UUID][]db.ExternalDuration, error) {
	s.externalCalls++
	return map[uuid.UUID][]db.ExternalDuration{}, nil
}
