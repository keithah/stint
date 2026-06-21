package workers

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/jobs"
	"github.com/keithah/stint/internal/services"
)

type StatsWorker struct {
	Store *db.Store
}

func (w StatsWorker) RecomputeLast7Days(ctx context.Context, userID uuid.UUID, timeoutMinutes int, writesOnly bool) (services.Stats, error) {
	return w.RecomputeRange(ctx, userID, "last_7_days", timeoutMinutes, writesOnly)
}

func (w StatsWorker) RecomputeRange(ctx context.Context, userID uuid.UUID, rangeName string, timeoutMinutes int, writesOnly bool) (services.Stats, error) {
	location := time.UTC
	if user, err := w.Store.UserByID(ctx, userID); err == nil && user.Timezone != "" {
		if loaded, err := time.LoadLocation(user.Timezone); err == nil {
			location = loaded
		}
	}
	now := time.Now().In(location)
	if rangeName == "all_time" {
		heartbeats, err := w.Store.AllHeartbeats(ctx, userID)
		if err != nil {
			return services.Stats{}, err
		}
		heartbeats = services.FilterWritesOnly(heartbeats, writesOnly)
		externalRows, err := w.Store.ListExternalDurations(ctx, userID)
		if err != nil {
			return services.Stats{}, err
		}
		costs, err := w.Store.AICostRates(ctx, userID)
		if err != nil {
			return services.Stats{}, err
		}
		stats, err := computeWorkerStats(rangeName, heartbeats, workerExternalDurations(externalRows), now, time.Duration(timeoutMinutes)*time.Minute, costs)
		if err != nil {
			return services.Stats{}, err
		}
		if err := w.Store.UpsertStatsCache(ctx, userID, rangeName, stats); err != nil {
			return services.Stats{}, err
		}
		return stats, nil
	}
	heartbeats, err := w.Store.HeartbeatsForStatsRange(ctx, userID, now, rangeName)
	if err != nil {
		return services.Stats{}, err
	}
	heartbeats = services.FilterWritesOnly(heartbeats, writesOnly)
	window, err := services.WindowForRange(now, rangeName)
	if err != nil {
		return services.Stats{}, err
	}
	externalRows, err := w.Store.ExternalDurationsBetween(ctx, userID, window.Start, window.End)
	if err != nil {
		return services.Stats{}, err
	}
	costs, err := w.Store.AICostRates(ctx, userID)
	if err != nil {
		return services.Stats{}, err
	}
	stats, err := computeWorkerStats(rangeName, heartbeats, workerExternalDurations(externalRows), now, time.Duration(timeoutMinutes)*time.Minute, costs)
	if err != nil {
		return services.Stats{}, err
	}
	if err := w.Store.UpsertStatsCache(ctx, userID, rangeName, stats); err != nil {
		return services.Stats{}, err
	}
	return stats, nil
}

func (w StatsWorker) HandleStatsRecomputeTask(ctx context.Context, task *asynq.Task) error {
	payload, err := jobs.ParseStatsRecomputeTask(task)
	if err != nil {
		return err
	}
	user, err := w.Store.GetUser(ctx, payload.UserID)
	if err != nil {
		return err
	}
	ranges := payload.Ranges
	if len(ranges) == 0 {
		ranges = jobs.DefaultStatsRanges()
	}
	for _, rangeName := range ranges {
		if _, err := w.RecomputeRange(ctx, payload.UserID, rangeName, user.TimeoutMinutes, user.WritesOnly); err != nil {
			return err
		}
	}
	return nil
}

func computeWorkerStats(rangeName string, heartbeats []services.Heartbeat, external []services.ExternalDuration, now time.Time, timeout time.Duration, costs map[string]services.AICostRate) (services.Stats, error) {
	if rangeName == "all_time" {
		return services.ComputeAllTimeStatsWithExternalDurationsAndAICosts(heartbeats, external, timeout, costs), nil
	}
	stats, _, err := services.ComputeStatsForRangeWithExternalDurationsAndAICosts(heartbeats, external, now, timeout, rangeName, costs)
	return stats, err
}

func workerExternalDurations(rows []db.ExternalDuration) []services.ExternalDuration {
	out := make([]services.ExternalDuration, 0, len(rows))
	for _, row := range rows {
		out = append(out, services.ExternalDuration{
			ID:         row.ID.String(),
			Provider:   row.Provider,
			ExternalID: row.ExternalID,
			Entity:     row.Entity,
			Type:       row.Type,
			Category:   row.Category,
			StartTime:  row.StartTime,
			EndTime:    row.EndTime,
			Project:    row.Project,
			Branch:     row.Branch,
			Language:   row.Language,
			Meta:       row.Meta,
		})
	}
	return out
}
