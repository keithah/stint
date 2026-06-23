package usagestats

import (
	"sort"
	"time"

	"github.com/keithah/stint/internal/pricing"
	"github.com/keithah/stint/internal/usage"
)

// BlockDuration is the billing window used by Claude/Codex (and ccusage):
// usage is bucketed into rolling 5-hour blocks.
const BlockDuration = 5 * time.Hour

// Block is one 5-hour billing window with its priced totals. Times are kept as
// time.Time; the handler formats them to RFC3339 strings in the response.
type Block struct {
	Start      time.Time
	End        time.Time
	IsActive   bool
	CostUSD    float64
	Tokens     int
	EventCount int
	LastEvent  time.Time
}

// CurrentBlock is the active block plus burn-rate and at-current-rate
// projections. JSON tags match the client-facing "current" object exactly.
type CurrentBlock struct {
	Start                string  `json:"start"`
	End                  string  `json:"end"`
	IsActive             bool    `json:"is_active"`
	ElapsedMinutes       int     `json:"elapsed_minutes"`
	CostUSD              float64 `json:"cost_usd"`
	Tokens               int     `json:"tokens"`
	BurnRateCostPerHour  float64 `json:"burn_rate_cost_per_hour"`
	BurnRateTokensPerMin float64 `json:"burn_rate_tokens_per_min"`
	ProjectedBlockCost   float64 `json:"projected_block_cost_usd"`
	ProjectedDayCost     float64 `json:"projected_day_cost_usd"`
	ProjectedMonthCost   float64 `json:"projected_month_cost_usd"`
}

// Blocks groups events into ccusage-style 5-hour billing blocks and, for the
// currently active block, computes burn rate and projections. It sorts events
// by time and assigns them to rolling 5-hour blocks following the ccusage
// algorithm:
//
//   - A block starts at the first event's timestamp floored to the hour and
//     spans 5 hours from that floored start.
//   - An event joins the current block if it falls within the 5h window AND is
//     within 5h of the previous event; otherwise it closes the current block
//     and starts a new one floored to the hour at that event.
//   - A block is active if now is within its 5h window and the last event was
//     less than 5h ago.
//
// It returns all blocks in chronological order plus the stats for the active
// block (or nil when none is active / no events). loc defaults to UTC and is
// used to bucket the end-of-day/month projections. billingOverride, keyed by
// agent, reclasses an agent's billing type at view time before pricing so a
// subscription-classified agent contributes zero marginal cost (nil = use the
// stored billing type on each event).
func Blocks(events []usage.Event, now time.Time, mode pricing.Mode, engine *pricing.Engine, loc *time.Location, billingOverride map[string]usage.BillingType) ([]Block, *CurrentBlock) {
	if loc == nil {
		loc = time.UTC
	}

	blocks, active := buildUsageBlocks(events, now, mode, engine, billingOverride)
	if active == nil {
		return blocks, nil
	}
	stats := currentBlockStats(*active, now, loc)
	return blocks, &stats
}

// buildUsageBlocks is the pure block-building core, separated so the assignment
// logic can be tested independently of projection math.
func buildUsageBlocks(events []usage.Event, now time.Time, mode pricing.Mode, engine *pricing.Engine, billingOverride map[string]usage.BillingType) ([]Block, *Block) {
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

	var blocks []Block
	var blockStart, lastEventTime time.Time
	cur := -1

	priceTokens := func(e usage.Event) (float64, int) {
		if override, ok := billingOverride[e.Agent]; ok {
			e.BillingType = override
		}
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
			ts.Sub(blockStart) >= BlockDuration ||
			ts.Sub(lastEventTime) >= BlockDuration

		if startNew {
			blockStart = floored
			blocks = append(blocks, Block{
				Start: blockStart,
				End:   blockStart.Add(BlockDuration),
			})
			cur = len(blocks) - 1
		}

		cost, tokens := priceTokens(te.event)
		blocks[cur].CostUSD += cost
		blocks[cur].Tokens += tokens
		blocks[cur].EventCount++
		blocks[cur].LastEvent = ts
		lastEventTime = ts
	}

	// Mark the active block: now within the 5h window and last event < 5h ago.
	var active *Block
	for i := range blocks {
		b := &blocks[i]
		if !now.Before(b.Start) && now.Before(b.End) && now.Sub(b.LastEvent) < BlockDuration {
			b.IsActive = true
			active = b
		}
	}
	return blocks, active
}

// currentBlockStats computes elapsed time, burn rate, and at-current-rate
// projections (to block end, end of local day, end of local month) for the
// active block.
func currentBlockStats(b Block, now time.Time, location *time.Location) CurrentBlock {
	if location == nil {
		location = time.UTC
	}

	elapsed := now.Sub(b.Start)
	if elapsed < 0 {
		elapsed = 0
	}
	elapsedMinutes := elapsed.Minutes()

	var burnCostPerHour, burnTokensPerMin float64
	if elapsedMinutes > 0 {
		burnCostPerHour = b.CostUSD / (elapsedMinutes / 60.0)
		burnTokensPerMin = float64(b.Tokens) / elapsedMinutes
	}

	// Projection to block end: extrapolate cost over the full 5h window.
	remainingToBlockEnd := b.End.Sub(now)
	if remainingToBlockEnd < 0 {
		remainingToBlockEnd = 0
	}
	projectedBlockCost := b.CostUSD + burnCostPerHour*remainingToBlockEnd.Hours()

	// Projection to end of local day / month, bucketed in the user's timezone.
	nowLocal := now.In(location)
	endOfDay := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, location).AddDate(0, 0, 1)
	endOfMonth := time.Date(nowLocal.Year(), nowLocal.Month(), 1, 0, 0, 0, 0, location).AddDate(0, 1, 0)

	projDay := b.CostUSD + burnCostPerHour*hoursUntil(now, endOfDay)
	projMonth := b.CostUSD + burnCostPerHour*hoursUntil(now, endOfMonth)

	return CurrentBlock{
		Start:                b.Start.UTC().Format(time.RFC3339),
		End:                  b.End.UTC().Format(time.RFC3339),
		IsActive:             b.IsActive,
		ElapsedMinutes:       int(elapsedMinutes),
		CostUSD:              b.CostUSD,
		Tokens:               b.Tokens,
		BurnRateCostPerHour:  burnCostPerHour,
		BurnRateTokensPerMin: burnTokensPerMin,
		ProjectedBlockCost:   projectedBlockCost,
		ProjectedDayCost:     projDay,
		ProjectedMonthCost:   projMonth,
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
