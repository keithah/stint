package workers

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/jobs"
	"github.com/keithah/stint/internal/pricing"
	"github.com/keithah/stint/internal/services"
	"github.com/keithah/stint/internal/usagestats"
)

type StatsWorker struct {
	Store *db.Store
	// Pricing prices usage_events when baking the AI cost meter into the stats
	// cache. Optional: when nil, a process-wide bundled engine is used so every
	// StatsWorker construction site stays a simple {Store: ...} literal.
	Pricing *pricing.Engine
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

// bakeAICosts replaces stats.AI cost fields with cache-aware list prices computed
// from usage_events, so the value written to the stats cache already reflects
// actual API cost (mirrors the API's compute path). No-op when pricing or usage
// events are unavailable.
func (w StatsWorker) bakeAICosts(ctx context.Context, userID uuid.UUID, loc *time.Location, rangeName string, stats *services.Stats) {
	engine := w.pricingEngine()
	if engine == nil || stats == nil {
		return
	}
	start, end, ok := services.AICostWindow(time.Now().In(loc), rangeName)
	if !ok {
		return
	}
	aggs, err := w.Store.UsageAggregatesBetween(ctx, userID, start, end, "", loc.String())
	if err != nil || len(aggs) == 0 {
		return
	}
	groups := make([]usagestats.Group, 0, len(aggs))
	for _, a := range aggs {
		groups = append(groups, a.StatsGroup())
	}
	services.ApplyUsageEventCosts(&stats.AI, groups, engine, end)
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
