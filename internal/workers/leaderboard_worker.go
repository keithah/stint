package workers

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/keithah/stint/internal/cache"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/jobs"
	"github.com/keithah/stint/internal/services"
)

const leaderboardCacheTTL = time.Hour

type LeaderboardWorker struct {
	Store leaderboardStore
	Cache cache.LeaderboardCache
}

type leaderboardStore interface {
	ListUsers(context.Context) ([]db.User, error)
	HeartbeatsForStatsRangeByUser(context.Context, []uuid.UUID, time.Time, string) (map[uuid.UUID][]services.Heartbeat, error)
	ExternalDurationsBetweenByUser(context.Context, []uuid.UUID, time.Time, time.Time) (map[uuid.UUID][]db.ExternalDuration, error)
}

func (w LeaderboardWorker) HandleLeaderboardUpdateTask(ctx context.Context, task *asynq.Task) error {
	payload, err := jobs.ParseLeaderboardUpdateTask(task)
	if err != nil {
		return err
	}
	rangeName := payload.Range
	if rangeName == "" {
		rangeName = "last_7_days"
	}
	entries, err := w.Compute(ctx, rangeName)
	if err != nil {
		return err
	}
	if w.Cache == nil {
		return nil
	}
	return w.Cache.Set(ctx, rangeName, entries, leaderboardCacheTTL)
}

func (w LeaderboardWorker) Compute(ctx context.Context, rangeName string) ([]services.LeaderboardEntry, error) {
	now := time.Now()
	window, err := services.WindowForRange(now, rangeName)
	if err != nil {
		return nil, err
	}
	users, err := w.Store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	userIDs := make([]uuid.UUID, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
	}
	heartbeatsByUser, err := w.Store.HeartbeatsForStatsRangeByUser(ctx, userIDs, now, rangeName)
	if err != nil {
		return nil, err
	}
	externalByUser, err := w.Store.ExternalDurationsBetweenByUser(ctx, userIDs, window.Start, window.End)
	if err != nil {
		return nil, err
	}
	entries := make([]services.LeaderboardEntry, 0, len(users))
	for _, user := range users {
		heartbeats := heartbeatsByUser[user.ID]
		heartbeats = services.FilterWritesOnly(heartbeats, user.WritesOnly)
		externalRows := externalByUser[user.ID]
		stats, err := computeWorkerLeaderboardStats(heartbeats, workerExternalDurations(externalRows), now, time.Duration(user.TimeoutMinutes)*time.Minute, rangeName)
		if err != nil {
			return nil, err
		}
		entries = append(entries, services.LeaderboardEntry{
			UserID:       user.ID.String(),
			Username:     user.GitHubUsername,
			DisplayName:  user.FullName,
			AvatarURL:    user.AvatarURL,
			Country:      user.Country,
			TotalSeconds: stats.TotalSeconds,
			Text:         stats.HumanReadableTotal,
		})
	}
	return services.RankLeaderboardEntries(entries), nil
}

func computeWorkerLeaderboardStats(heartbeats []services.Heartbeat, external []services.ExternalDuration, now time.Time, timeout time.Duration, rangeName string) (services.Stats, error) {
	stats, _, err := services.ComputeStatsForRangeWithExternalDurations(heartbeats, external, now, timeout, rangeName)
	return stats, err
}
