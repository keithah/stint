// Package aicostbake bakes cache-aware, list-price AI cost (computed from
// usage_events by the pricing engine) into a stats.AI metrics block, replacing
// the legacy heartbeat token-rate estimate. It is the single home for that
// orchestration, shared by the API compute path and the stats worker so both
// write identical cost into the stats cache.
package aicostbake

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/pricing"
	"github.com/keithah/stint/internal/services"
	"github.com/keithah/stint/internal/usagestats"
)

// Bake overwrites stats.AI cost fields with the metered-API-equivalent cost for
// the user's usage events in the given stats range. No-op when pricing is
// unavailable or the window has no usage events (the heartbeat estimate stands).
func Bake(ctx context.Context, store *db.Store, engine *pricing.Engine, userID uuid.UUID, loc *time.Location, rangeName string, stats *services.Stats) {
	if store == nil || engine == nil || stats == nil {
		return
	}
	start, end, ok := services.AICostWindow(time.Now().In(loc), rangeName)
	if !ok {
		return
	}
	aggs, err := store.UsageAggregatesBetween(ctx, userID, start, end, "", loc.String())
	if err != nil || len(aggs) == 0 {
		return
	}
	groups := make([]usagestats.Group, 0, len(aggs))
	for _, a := range aggs {
		groups = append(groups, a.StatsGroup())
	}
	services.ApplyUsageEventCosts(&stats.AI, groups, engine, end)
}
