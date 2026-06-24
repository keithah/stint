package usagestats

import (
	"testing"
	"time"

	"github.com/keithah/stint/internal/pricing"
)

// PriceGroups prices each group once; BucketCents aggregates the priced slice by
// a caller key. Cache reads are priced at the discounted rate (not as fresh
// input). Golden values use the pinned claude-sonnet-4-5 snapshot: input 3e-6,
// output 1.5e-5, cacheRead 3e-7.
func TestPriceAndBucketCents(t *testing.T) {
	engine := newEngine(t)
	loc := time.UTC
	windowEnd := time.Date(2026, 6, 25, 0, 0, 0, 0, loc) // exclusive end (start of 06-25)

	groups := []Group{
		// Today (06-24): in the daily/weekly/monthly windows.
		{Agent: "claude", Model: "claude-sonnet-4-5", Day: "2026-06-24", Input: 1000, Output: 500, CacheRead: 100000, EventCount: 1},
		// 10 days ago (06-14): in monthly but not weekly/daily.
		{Agent: "codex", Model: "claude-sonnet-4-5", Day: "2026-06-14", Input: 2000, CacheRead: 0, EventCount: 1},
	}

	priced := PriceGroups(groups, engine, pricing.ModeCalculate)
	if len(priced) != 2 {
		t.Fatalf("expected 2 priced groups, got %d", len(priced))
	}
	got := BucketCents(priced, func(g Group) string { return g.Agent }, windowEnd)

	// claude: 1000*3e-6 + 500*1.5e-5 + 100000*3e-7 = 0.003 + 0.0075 + 0.03 = 0.0405 -> 4c.
	// Cache reads at fresh-input rate would add 100000*3e-6 = 0.30 (30c); the 4c
	// result proves the meter is cache-aware.
	if got.ByKeyCents["claude"] != 4 {
		t.Fatalf("claude cents: want 4, got %d", got.ByKeyCents["claude"])
	}
	if got.ByKeyCents["codex"] != 1 { // 2000*3e-6 = 0.006 -> 1c
		t.Fatalf("codex cents: want 1, got %d", got.ByKeyCents["codex"])
	}
	if got.TotalCents != 5 {
		t.Fatalf("total cents: want 5, got %d", got.TotalCents)
	}
	if got.ByDayCents["2026-06-24"] != 4 {
		t.Fatalf("day cents: want 4, got %d", got.ByDayCents["2026-06-24"])
	}

	claude := got.PeriodByKey["claude"]
	if claude.Daily != 4 || claude.Weekly != 4 || claude.Monthly != 4 || claude.Total != 4 {
		t.Fatalf("claude periods: want 4/4/4/4, got %d/%d/%d/%d", claude.Daily, claude.Weekly, claude.Monthly, claude.Total)
	}
	codex := got.PeriodByKey["codex"]
	if codex.Daily != 0 || codex.Weekly != 0 || codex.Monthly != 1 || codex.Total != 1 {
		t.Fatalf("codex periods: want 0/0/1/1, got %d/%d/%d/%d", codex.Daily, codex.Weekly, codex.Monthly, codex.Total)
	}

	// A different key function re-buckets the SAME priced slice without re-pricing.
	const1 := BucketCents(priced, func(Group) string { return "all" }, windowEnd)
	if const1.ByKeyCents["all"] != 5 {
		t.Fatalf("re-bucketed total: want 5, got %d", const1.ByKeyCents["all"])
	}
}

func TestPriceGroupsNilEngine(t *testing.T) {
	if priced := PriceGroups([]Group{{Agent: "claude"}}, nil, pricing.ModeCalculate); priced != nil {
		t.Fatalf("nil engine should yield nil priced slice, got %v", priced)
	}
}
