package usagestats

import (
	"testing"
	"time"

	"github.com/keithah/stint/internal/pricing"
)

// AICostsFromGroups prices cache reads at the discounted rate (not as fresh
// input) and buckets cost by agent, day, and period. Golden values use the
// pinned claude-sonnet-4-5 snapshot: input 3e-6, output 1.5e-5, cacheRead 3e-7.
func TestAICostsFromGroups(t *testing.T) {
	engine := newEngine(t)
	loc := time.UTC
	windowEnd := time.Date(2026, 6, 25, 0, 0, 0, 0, loc) // exclusive end (start of 06-25)

	groups := []Group{
		// Today (06-24): in the daily/weekly/monthly windows.
		{Agent: "claude", Model: "claude-sonnet-4-5", Day: "2026-06-24", Input: 1000, Output: 500, CacheRead: 100000, EventCount: 1},
		// 10 days ago (06-14): in monthly but not weekly/daily.
		{Agent: "codex", Model: "claude-sonnet-4-5", Day: "2026-06-14", Input: 2000, CacheRead: 0, EventCount: 1},
	}

	got := AICostsFromGroups(groups, engine, pricing.ModeCalculate, windowEnd)

	// claude: 1000*3e-6 + 500*1.5e-5 + 100000*3e-7 = 0.003 + 0.0075 + 0.03 = 0.0405 -> 4 cents
	if got.ByAgentCents["claude"] != 4 {
		t.Fatalf("claude cents: want 4, got %d", got.ByAgentCents["claude"])
	}
	// codex: 2000*3e-6 = 0.006 -> 1 cent
	if got.ByAgentCents["codex"] != 1 {
		t.Fatalf("codex cents: want 1, got %d", got.ByAgentCents["codex"])
	}
	if got.TotalCents != 5 {
		t.Fatalf("total cents: want 5, got %d", got.TotalCents)
	}

	// Cache reads are NOT priced as fresh input: 100000 cache-read tok at fresh
	// input rate would add 100000*3e-6 = 0.30 (30c). The discounted result (4c)
	// proves the meter is cache-aware.
	if got.ByDayCents["2026-06-24"] != 4 {
		t.Fatalf("day cents: want 4, got %d", got.ByDayCents["2026-06-24"])
	}

	// Period bucketing: claude (today) counts in daily; codex (10 days ago) only
	// in monthly.
	claude := got.PeriodByAgent["claude"]
	if claude.Daily != 4 || claude.Weekly != 4 || claude.Monthly != 4 || claude.Total != 4 {
		t.Fatalf("claude periods: want 4/4/4/4, got %d/%d/%d/%d", claude.Daily, claude.Weekly, claude.Monthly, claude.Total)
	}
	codex := got.PeriodByAgent["codex"]
	if codex.Daily != 0 || codex.Weekly != 0 || codex.Monthly != 1 || codex.Total != 1 {
		t.Fatalf("codex periods: want 0/0/1/1, got %d/%d/%d/%d", codex.Daily, codex.Weekly, codex.Monthly, codex.Total)
	}
}

func TestAICostsFromGroupsNilEngine(t *testing.T) {
	got := AICostsFromGroups([]Group{{Agent: "claude", Day: "2026-06-24"}}, nil, pricing.ModeCalculate, time.Now())
	if got.TotalCents != 0 || len(got.ByAgentCents) != 0 {
		t.Fatalf("nil engine should yield empty costs, got %+v", got)
	}
}
