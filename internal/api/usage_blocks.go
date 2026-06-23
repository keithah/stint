package api

import (
	"net/http"
	"sort"
	"time"

	"github.com/keithah/stint/internal/pricing"
	"github.com/keithah/stint/internal/usage"
	"github.com/labstack/echo/v4"
)

// blockDuration is the billing window used by Claude/Codex (and ccusage):
// usage is bucketed into rolling 5-hour blocks.
const blockDuration = 5 * time.Hour

// usageBlock is one 5-hour billing window with its priced totals.
type usageBlock struct {
	start      time.Time
	end        time.Time
	isActive   bool
	costUSD    float64
	tokens     int
	eventCount int
	lastEvent  time.Time
}

// usageEventsBlocks groups recent events into ccusage-style 5-hour billing
// blocks and, for the currently active block, reports burn rate and
// projections at the current rate. Window defaults to the last 30 days but
// accepts range/start/end like the summary handler.
func (s *Server) usageEventsBlocks(c echo.Context) error {
	user := userFromContext(c)
	now := time.Now()

	start, end, _, err := resolveUsageWindow(c, now)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	mode := usageCostMode(c)

	events, err := s.Store.UsageEventsBetween(c.Request().Context(), user.ID, start, end)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	events = filterEventsByAgent(events, c.QueryParam("agent"))

	// Use the same custom-pricing-aware engine as the summary so the burn-rate
	// panel and the summary card never disagree on a custom-priced model.
	engine, err := s.pricingEngineForUser(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}

	location := userLocation(user)
	blocks, current := buildUsageBlocks(events, now, mode, engine, location)

	blocksOut := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		blocksOut = append(blocksOut, map[string]any{
			"start":       b.start.UTC().Format(time.RFC3339),
			"end":         b.end.UTC().Format(time.RFC3339),
			"is_active":   b.isActive,
			"cost_usd":    b.costUSD,
			"tokens":      b.tokens,
			"event_count": b.eventCount,
		})
	}

	data := map[string]any{
		"cost_mode": string(mode),
		"blocks":    blocksOut,
		"current":   nil,
	}
	if current != nil {
		data["current"] = currentBlockStats(*current, now, location)
	}

	return c.JSON(http.StatusOK, map[string]any{"data": data})
}

// buildUsageBlocks is the pure block-building core, extracted so it can be
// tested without HTTP/DB. It sorts events by time and assigns them to rolling
// 5-hour blocks following the ccusage algorithm:
//
//   - A block starts at the first event's timestamp floored to the hour and
//     spans 5 hours from that floored start.
//   - An event joins the current block if it falls within the 5h window AND is
//     within 5h of the previous event; otherwise it closes the current block
//     and starts a new one floored to the hour at that event.
//   - A block is active if now is within its 5h window and the last event was
//     less than 5h ago.
//
// It returns all blocks in chronological order plus a pointer to the active
// block (or nil when none is active / no events).
func buildUsageBlocks(events []usage.Event, now time.Time, mode pricing.Mode, engine *pricing.Engine, location *time.Location) ([]usageBlock, *usageBlock) {
	if location == nil {
		location = time.UTC
	}

	type timed struct {
		ts    time.Time
		event usage.Event
	}
	timedEvents := make([]timed, 0, len(events))
	for _, e := range events {
		ts, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			continue
		}
		timedEvents = append(timedEvents, timed{ts: ts.UTC(), event: e})
	}
	sort.Slice(timedEvents, func(i, j int) bool {
		return timedEvents[i].ts.Before(timedEvents[j].ts)
	})

	var blocks []usageBlock
	var blockStart, lastEventTime time.Time
	cur := -1

	priceTokens := func(e usage.Event) (float64, int) {
		var cost float64
		if engine != nil {
			cost = engine.Price(e, mode).USD
		}
		return cost, e.TotalTokens()
	}

	for _, te := range timedEvents {
		ts := te.ts
		floored := ts.Truncate(time.Hour)

		startNew := cur < 0 ||
			ts.Sub(blockStart) >= blockDuration ||
			ts.Sub(lastEventTime) >= blockDuration

		if startNew {
			blockStart = floored
			blocks = append(blocks, usageBlock{
				start: blockStart,
				end:   blockStart.Add(blockDuration),
			})
			cur = len(blocks) - 1
		}

		cost, tokens := priceTokens(te.event)
		blocks[cur].costUSD += cost
		blocks[cur].tokens += tokens
		blocks[cur].eventCount++
		blocks[cur].lastEvent = ts
		lastEventTime = ts
	}

	// Mark the active block: now within the 5h window and last event < 5h ago.
	var active *usageBlock
	for i := range blocks {
		b := &blocks[i]
		if !now.Before(b.start) && now.Before(b.end) && now.Sub(b.lastEvent) < blockDuration {
			b.isActive = true
			active = b
		}
	}
	return blocks, active
}

// currentBlockStats computes elapsed time, burn rate, and at-current-rate
// projections (to block end, end of local day, end of local month) for the
// active block.
func currentBlockStats(b usageBlock, now time.Time, location *time.Location) map[string]any {
	if location == nil {
		location = time.UTC
	}

	elapsed := now.Sub(b.start)
	if elapsed < 0 {
		elapsed = 0
	}
	elapsedMinutes := elapsed.Minutes()

	var burnCostPerHour, burnTokensPerMin float64
	if elapsedMinutes > 0 {
		burnCostPerHour = b.costUSD / (elapsedMinutes / 60.0)
		burnTokensPerMin = float64(b.tokens) / elapsedMinutes
	}

	// Projection to block end: extrapolate cost over the full 5h window.
	remainingToBlockEnd := b.end.Sub(now)
	if remainingToBlockEnd < 0 {
		remainingToBlockEnd = 0
	}
	projectedBlockCost := b.costUSD + burnCostPerHour*remainingToBlockEnd.Hours()

	// Projection to end of local day / month, bucketed in the user's timezone.
	nowLocal := now.In(location)
	endOfDay := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, location).AddDate(0, 0, 1)
	endOfMonth := time.Date(nowLocal.Year(), nowLocal.Month(), 1, 0, 0, 0, 0, location).AddDate(0, 1, 0)

	projDay := b.costUSD + burnCostPerHour*hoursUntil(now, endOfDay)
	projMonth := b.costUSD + burnCostPerHour*hoursUntil(now, endOfMonth)

	return map[string]any{
		"start":                    b.start.UTC().Format(time.RFC3339),
		"end":                      b.end.UTC().Format(time.RFC3339),
		"is_active":                b.isActive,
		"elapsed_minutes":          int(elapsedMinutes),
		"cost_usd":                 b.costUSD,
		"tokens":                   b.tokens,
		"burn_rate_cost_per_hour":  burnCostPerHour,
		"burn_rate_tokens_per_min": burnTokensPerMin,
		"projected_block_cost_usd": projectedBlockCost,
		"projected_day_cost_usd":   projDay,
		"projected_month_cost_usd": projMonth,
	}
}

// hoursUntil returns the non-negative number of hours from now to t.
func hoursUntil(now, t time.Time) float64 {
	d := t.Sub(now)
	if d < 0 {
		return 0
	}
	return d.Hours()
}
