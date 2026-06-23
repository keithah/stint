package api

import (
	"math"
	"testing"
	"time"

	"github.com/keithah/stint/internal/pricing"
	"github.com/keithah/stint/internal/usage"
)

// fixedCostEngine is unavailable; we instead drive cost via CostUSDProvided so
// the pure helper can be tested without a real pricing table. With ModeAuto the
// engine returns the provided cost.

func ev(ts string, input int, cost float64) usage.Event {
	c := cost
	return usage.Event{
		Agent:           "claude-code",
		Timestamp:       ts,
		Model:           "claude-sonnet-4-6",
		InputTokens:     input,
		CostUSDProvided: &c,
	}
}

func newEngine(t *testing.T) *pricing.Engine {
	t.Helper()
	e, err := pricing.NewFromBundled()
	if err != nil {
		t.Fatalf("pricing.NewFromBundled: %v", err)
	}
	return e
}

func approx(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

func TestBuildUsageBlocksWithinFiveHoursIsOneBlock(t *testing.T) {
	engine := newEngine(t)
	events := []usage.Event{
		ev("2026-06-23T10:15:00Z", 100, 1.0),
		ev("2026-06-23T12:30:00Z", 200, 2.0),
		ev("2026-06-23T14:00:00Z", 300, 3.0), // within 5h of floored start 10:00
	}
	now := time.Date(2026, 6, 23, 14, 30, 0, 0, time.UTC)
	blocks, current := buildUsageBlocks(events, now, pricing.ModeAuto, engine, time.UTC)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	b := blocks[0]
	if !b.start.Equal(time.Date(2026, 6, 23, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("block start should be floored to 10:00, got %v", b.start)
	}
	if !b.end.Equal(time.Date(2026, 6, 23, 15, 0, 0, 0, time.UTC)) {
		t.Fatalf("block end should be 15:00, got %v", b.end)
	}
	if b.eventCount != 3 || b.tokens != 600 || !approx(b.costUSD, 6.0) {
		t.Fatalf("bad totals: count=%d tokens=%d cost=%v", b.eventCount, b.tokens, b.costUSD)
	}
	if current == nil {
		t.Fatal("expected an active block (now within window, last event <5h ago)")
	}
}

func TestBuildUsageBlocksGapStartsNewBlock(t *testing.T) {
	engine := newEngine(t)
	events := []usage.Event{
		ev("2026-06-23T10:00:00Z", 100, 1.0),
		// >5h gap from previous event -> new block.
		ev("2026-06-23T16:00:00Z", 100, 1.0),
	}
	now := time.Date(2026, 6, 23, 16, 30, 0, 0, time.UTC)
	blocks, _ := buildUsageBlocks(events, now, pricing.ModeAuto, engine, time.UTC)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks across a >5h gap, got %d", len(blocks))
	}
	if !blocks[1].start.Equal(time.Date(2026, 6, 23, 16, 0, 0, 0, time.UTC)) {
		t.Fatalf("second block should start floored at 16:00, got %v", blocks[1].start)
	}
}

func TestBuildUsageBlocksWindowOverflowStartsNewBlock(t *testing.T) {
	engine := newEngine(t)
	// Events are <5h apart pairwise but the third exceeds the 5h window from
	// the floored start (10:00 -> window ends 15:00).
	events := []usage.Event{
		ev("2026-06-23T10:30:00Z", 100, 1.0),
		ev("2026-06-23T13:00:00Z", 100, 1.0),
		ev("2026-06-23T15:30:00Z", 100, 1.0), // within 5h of prev, but past window end 15:00
	}
	now := time.Date(2026, 6, 23, 16, 0, 0, 0, time.UTC)
	blocks, _ := buildUsageBlocks(events, now, pricing.ModeAuto, engine, time.UTC)
	if len(blocks) != 2 {
		t.Fatalf("expected window overflow to start a new block, got %d blocks", len(blocks))
	}
}

func TestBuildUsageBlocksEmptyHasNoCurrent(t *testing.T) {
	engine := newEngine(t)
	blocks, current := buildUsageBlocks(nil, time.Now(), pricing.ModeAuto, engine, time.UTC)
	if len(blocks) != 0 {
		t.Fatalf("expected no blocks, got %d", len(blocks))
	}
	if current != nil {
		t.Fatal("expected no current block for empty input")
	}
}

func TestBuildUsageBlocksStaleBlockNotActive(t *testing.T) {
	engine := newEngine(t)
	events := []usage.Event{ev("2026-06-23T10:00:00Z", 100, 1.0)}
	// now is within the 5h window (10:00-15:00) but last event was >5h ago is
	// impossible here; instead test now past window end.
	now := time.Date(2026, 6, 23, 16, 0, 0, 0, time.UTC)
	_, current := buildUsageBlocks(events, now, pricing.ModeAuto, engine, time.UTC)
	if current != nil {
		t.Fatal("block whose window has ended must not be active")
	}
}

func TestCurrentBlockStatsBurnRateAndProjection(t *testing.T) {
	engine := newEngine(t)
	// Block floored start 10:00; two events totalling 1200 tokens, $6.
	events := []usage.Event{
		ev("2026-06-23T10:00:00Z", 600, 2.0),
		ev("2026-06-23T11:00:00Z", 600, 4.0),
	}
	// now = 12:00 -> elapsed 120 minutes (2h) from block start 10:00.
	now := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	_, current := buildUsageBlocks(events, now, pricing.ModeAuto, engine, time.UTC)
	if current == nil {
		t.Fatal("expected active block")
	}
	stats := currentBlockStats(*current, now, time.UTC)

	if stats["elapsed_minutes"].(int) != 120 {
		t.Fatalf("elapsed_minutes: got %v want 120", stats["elapsed_minutes"])
	}
	// cost $6 over 2h -> $3/h.
	if !approx(stats["burn_rate_cost_per_hour"].(float64), 3.0) {
		t.Fatalf("burn cost/hour: got %v want 3.0", stats["burn_rate_cost_per_hour"])
	}
	// 1200 tokens over 120 min -> 10 tokens/min.
	if !approx(stats["burn_rate_tokens_per_min"].(float64), 10.0) {
		t.Fatalf("burn tokens/min: got %v want 10.0", stats["burn_rate_tokens_per_min"])
	}
	// Block end 15:00, 3h remaining at $3/h -> $6 + $9 = $15.
	if !approx(stats["projected_block_cost_usd"].(float64), 15.0) {
		t.Fatalf("projected block cost: got %v want 15.0", stats["projected_block_cost_usd"])
	}
	// End of day: 12h remaining at $3/h -> $6 + $36 = $42.
	if !approx(stats["projected_day_cost_usd"].(float64), 42.0) {
		t.Fatalf("projected day cost: got %v want 42.0", stats["projected_day_cost_usd"])
	}
	// Month projection must be >= day projection.
	if stats["projected_month_cost_usd"].(float64) < stats["projected_day_cost_usd"].(float64) {
		t.Fatalf("month projection should be >= day projection")
	}
}
