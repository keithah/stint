package workers

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/keithah/stint/internal/aicostbake"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/jobs"
	"github.com/keithah/stint/internal/pricing"
	"github.com/keithah/stint/internal/services"
	"github.com/keithah/stint/internal/tzcache"
)

type StatsWorker struct {
	Store        statsStore
	Baker        statsCostBaker
	ProjectBaker projectStatsCostBaker
	// Pricing prices usage_events when baking the AI cost meter into the stats
	// cache. Optional: when nil, a process-wide bundled engine is used so every
	// StatsWorker construction site stays a simple {Store: ...} literal.
	Pricing *pricing.Engine
}

type statsStore interface {
	UserByID(context.Context, uuid.UUID) (db.User, error)
	HeartbeatsForAllTimeStats(context.Context, uuid.UUID) ([]services.Heartbeat, error)
	HeartbeatsForProject(context.Context, uuid.UUID, string) ([]services.Heartbeat, error)
	HeartbeatsForStatsRange(context.Context, uuid.UUID, time.Time, string) ([]services.Heartbeat, error)
	HeartbeatsForProjectStatsRange(context.Context, uuid.UUID, string, time.Time, string) ([]services.Heartbeat, error)
	ListExternalDurations(context.Context, uuid.UUID) ([]db.ExternalDuration, error)
	ListExternalDurationsForProject(context.Context, uuid.UUID, string) ([]db.ExternalDuration, error)
	ExternalDurationsBetween(context.Context, uuid.UUID, time.Time, time.Time) ([]db.ExternalDuration, error)
	ExternalDurationsForProjectBetween(context.Context, uuid.UUID, string, time.Time, time.Time) ([]db.ExternalDuration, error)
	AICostRates(context.Context, uuid.UUID) (map[string]services.AICostRate, error)
	UpsertStatsCache(context.Context, uuid.UUID, string, services.Stats) error
	UpsertProjectStatsCache(context.Context, uuid.UUID, string, string, services.Stats) error
}

type statsCostBaker interface {
	BakeStatsCosts(context.Context, uuid.UUID, *time.Location, string, *services.Stats)
}

type projectStatsCostBaker interface {
	BakeProjectStatsCosts(context.Context, uuid.UUID, *time.Location, string, string, *services.Stats)
}

type statsCostBakeFunc func(context.Context, uuid.UUID, *time.Location, string, *services.Stats)

func (f statsCostBakeFunc) BakeStatsCosts(ctx context.Context, userID uuid.UUID, location *time.Location, rangeName string, stats *services.Stats) {
	f(ctx, userID, location, rangeName, stats)
}

type projectStatsCostBakeFunc func(context.Context, uuid.UUID, *time.Location, string, string, *services.Stats)

func (f projectStatsCostBakeFunc) BakeProjectStatsCosts(ctx context.Context, userID uuid.UUID, location *time.Location, rangeName, projectName string, stats *services.Stats) {
	f(ctx, userID, location, rangeName, projectName, stats)
}

var (
	workerPricingOnce   sync.Once
	workerPricingShared *pricing.Engine
)

// pricingEngine returns the worker's engine, falling back to a lazily-built,
// process-wide bundled engine so cost baking works without wiring an engine
// through every StatsWorker construction site.
func (w StatsWorker) pricingEngine() *pricing.Engine {
	if w.Pricing != nil {
		return w.Pricing
	}
	workerPricingOnce.Do(func() {
		workerPricingShared, _ = pricing.NewFromBundled()
	})
	return workerPricingShared
}

func (w StatsWorker) RecomputeLast7Days(ctx context.Context, userID uuid.UUID, timeoutMinutes int, writesOnly bool) (services.Stats, error) {
	return w.RecomputeRange(ctx, userID, "last_7_days", timeoutMinutes, writesOnly)
}

func (w StatsWorker) RecomputeRange(ctx context.Context, userID uuid.UUID, rangeName string, timeoutMinutes int, writesOnly bool) (services.Stats, error) {
	location := time.UTC
	if user, err := w.Store.UserByID(ctx, userID); err == nil && user.Timezone != "" {
		location = tzcache.Location(user.Timezone)
	}
	now := time.Now().In(location)
	if rangeName == "all_time" {
		heartbeats, err := w.Store.HeartbeatsForAllTimeStats(ctx, userID)
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
		w.bakeAICosts(ctx, userID, location, rangeName, &stats)
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
	w.bakeAICosts(ctx, userID, location, rangeName, &stats)
	if err := w.Store.UpsertStatsCache(ctx, userID, rangeName, stats); err != nil {
		return services.Stats{}, err
	}
	return stats, nil
}

func (w StatsWorker) RecomputeRanges(ctx context.Context, userID uuid.UUID, ranges []string, timeoutMinutes int, writesOnly bool) error {
	if len(ranges) == 0 {
		ranges = jobs.DefaultStatsRanges()
	}
	for _, rangeName := range ranges {
		if _, err := w.RecomputeRange(ctx, userID, rangeName, timeoutMinutes, writesOnly); err != nil {
			return err
		}
	}
	return nil
}

func (w StatsWorker) RecomputeProjectRange(ctx context.Context, userID uuid.UUID, projectName, rangeName string, timeoutMinutes int, writesOnly bool) (services.Stats, error) {
	location := time.UTC
	if user, err := w.Store.UserByID(ctx, userID); err == nil && user.Timezone != "" {
		location = tzcache.Location(user.Timezone)
	}
	now := time.Now().In(location)
	var heartbeats []services.Heartbeat
	var externalRows []db.ExternalDuration
	var err error
	if rangeName == "all_time" {
		heartbeats, err = w.Store.HeartbeatsForProject(ctx, userID, projectName)
		if err != nil {
			return services.Stats{}, err
		}
		externalRows, err = w.Store.ListExternalDurationsForProject(ctx, userID, projectName)
		if err != nil {
			return services.Stats{}, err
		}
	} else {
		heartbeats, err = w.Store.HeartbeatsForProjectStatsRange(ctx, userID, projectName, now, rangeName)
		if err != nil {
			return services.Stats{}, err
		}
		window, err := services.WindowForRange(now, rangeName)
		if err != nil {
			return services.Stats{}, err
		}
		externalRows, err = w.Store.ExternalDurationsForProjectBetween(ctx, userID, projectName, window.Start, window.End)
		if err != nil {
			return services.Stats{}, err
		}
	}
	heartbeats = services.FilterWritesOnly(heartbeats, writesOnly)
	projectExternal := workerExternalDurations(externalRows)
	costs, err := w.Store.AICostRates(ctx, userID)
	if err != nil {
		return services.Stats{}, err
	}
	stats, err := computeWorkerStats(rangeName, heartbeats, projectExternal, now, time.Duration(timeoutMinutes)*time.Minute, costs)
	if err != nil {
		return services.Stats{}, err
	}
	w.bakeProjectAICosts(ctx, userID, location, rangeName, projectName, &stats)
	if err := w.Store.UpsertProjectStatsCache(ctx, userID, projectName, rangeName, stats); err != nil {
		return services.Stats{}, err
	}
	return stats, nil
}

func (w StatsWorker) HandleStatsRecomputeTask(ctx context.Context, task *asynq.Task) error {
	payload, err := jobs.ParseStatsRecomputeTask(task)
	if err != nil {
		return err
	}
	user, err := w.Store.UserByID(ctx, payload.UserID)
	if err != nil {
		return err
	}
	ranges := payload.Ranges
	if len(ranges) == 0 {
		ranges = jobs.DefaultStatsRanges()
	}
	return w.RecomputeRanges(ctx, payload.UserID, ranges, user.TimeoutMinutes, user.WritesOnly)
}

func (w StatsWorker) HandleProjectStatsRecomputeTask(ctx context.Context, task *asynq.Task) error {
	payload, err := jobs.ParseProjectStatsRecomputeTask(task)
	if err != nil {
		return err
	}
	user, err := w.Store.UserByID(ctx, payload.UserID)
	if err != nil {
		return err
	}
	_, err = w.RecomputeProjectRange(ctx, payload.UserID, payload.Project, payload.Range, user.TimeoutMinutes, user.WritesOnly)
	return err
}

func (w StatsWorker) bakeAICosts(ctx context.Context, userID uuid.UUID, location *time.Location, rangeName string, stats *services.Stats) {
	if w.Baker != nil {
		w.Baker.BakeStatsCosts(ctx, userID, location, rangeName, stats)
	}
}

func (w StatsWorker) bakeProjectAICosts(ctx context.Context, userID uuid.UUID, location *time.Location, rangeName, projectName string, stats *services.Stats) {
	if w.ProjectBaker != nil {
		w.ProjectBaker.BakeProjectStatsCosts(ctx, userID, location, rangeName, projectName, stats)
	}
}

func NewStatsWorker(store *db.Store, engine *pricing.Engine) StatsWorker {
	worker := StatsWorker{Store: store, Pricing: engine}
	worker.Baker = statsCostBakeFunc(func(ctx context.Context, userID uuid.UUID, location *time.Location, rangeName string, stats *services.Stats) {
		aicostbake.Bake(ctx, store, worker.pricingEngine(), userID, location, rangeName, stats)
	})
	worker.ProjectBaker = projectStatsCostBakeFunc(func(ctx context.Context, userID uuid.UUID, location *time.Location, rangeName, projectName string, stats *services.Stats) {
		aicostbake.BakeProject(ctx, store, worker.pricingEngine(), userID, location, rangeName, projectName, stats)
	})
	return worker
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
