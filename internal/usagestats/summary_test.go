package usagestats

import (
	"reflect"
	"testing"
	"time"

	"github.com/keithah/stint/internal/pricing"
	"github.com/keithah/stint/internal/usage"
)

// TestSummarizeAggregatesEqualsSummarize proves the SQL-aggregation path is
// byte-identical to the per-event path: pricing pre-summed groups yields the
// same Summary as pricing the equivalent raw events. Covered for both
// ModeCalculate (recompute from tokens) and ModeAuto with provider-reported
// cost present, since has_provided is part of the group key.
func TestSummarizeAggregatesEqualsSummarize(t *testing.T) {
	engine := newEngine(t)
	cost := func(v float64) *float64 { return &v }

	// Two events share an (agent, model, project, day, billing, has_provided)
	// group; the third differs by model, the fourth differs by has_provided.
	events := []usage.Event{
		{Agent: "claude-code", Model: "claude-sonnet-4-6", Project: "stint", Timestamp: "2026-06-23T10:00:00Z", BillingType: usage.BillingAPI, InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 200, CostUSDProvided: cost(0.25)},
		{Agent: "claude-code", Model: "claude-sonnet-4-6", Project: "stint", Timestamp: "2026-06-23T12:00:00Z", BillingType: usage.BillingAPI, InputTokens: 2000, OutputTokens: 1000, CacheReadTokens: 100, CostUSDProvided: cost(0.5)},
		{Agent: "claude-code", Model: "claude-sonnet-4-6", Project: "stint", Timestamp: "2026-06-23T13:00:00Z", BillingType: usage.BillingAPI, InputTokens: 300, OutputTokens: 150},
		{Agent: "codex", Model: "claude-sonnet-4-6", Project: "other", Timestamp: "2026-06-24T09:00:00Z", BillingType: usage.BillingAPI, InputTokens: 700, OutputTokens: 200, ReasoningTokens: 50},
	}

	// Equivalent pre-summed groups (what UsageAggregatesBetween produces).
	groups := []Group{
		{Agent: "claude-code", Model: "claude-sonnet-4-6", Project: "stint", Day: "2026-06-23", BillingType: string(usage.BillingAPI), HasProvided: true, Input: 3000, Output: 1500, CacheRead: 300, ProvidedCostUSD: 0.75, EventCount: 2},
		{Agent: "claude-code", Model: "claude-sonnet-4-6", Project: "stint", Day: "2026-06-23", BillingType: string(usage.BillingAPI), HasProvided: false, Input: 300, Output: 150, EventCount: 1},
		{Agent: "codex", Model: "claude-sonnet-4-6", Project: "other", Day: "2026-06-24", BillingType: string(usage.BillingAPI), HasProvided: false, Input: 700, Output: 200, Reasoning: 50, EventCount: 1},
	}

	for _, mode := range []pricing.Mode{pricing.ModeCalculate, pricing.ModeAuto} {
		fromEvents := Summarize(events, engine, mode, time.UTC, nil)
		fromGroups := SummarizeAggregates(groups, engine, mode, nil)
		if !reflect.DeepEqual(fromEvents, fromGroups) {
			t.Fatalf("mode %s: aggregates summary != events summary\n events=%+v\n groups=%+v", mode, fromEvents, fromGroups)
		}
	}
}

// TestBillingOverrideZeroesMarginalKeepsCost proves the per-agent override
// reclasses a stored-api agent as subscription at view time: marginal_usd drops
// to 0 while cost_usd (the equivalent-API figure) is unchanged.
func TestBillingOverrideZeroesMarginalKeepsCost(t *testing.T) {
	engine := newEngine(t)

	// Both groups are stored as api; only claude-code is overridden.
	groups := []Group{
		{Agent: "claude-code", Model: "claude-sonnet-4-6", Project: "stint", Day: "2026-06-23", BillingType: string(usage.BillingAPI), Input: 1_000_000, Output: 500_000, EventCount: 3},
		{Agent: "codex", Model: "claude-sonnet-4-6", Project: "other", Day: "2026-06-23", BillingType: string(usage.BillingAPI), Input: 200_000, Output: 100_000, EventCount: 1},
	}

	base := SummarizeAggregates(groups, engine, pricing.ModeCalculate, nil)
	if base.Total.CostUSD <= 0 {
		t.Fatalf("expected positive cost, got %f", base.Total.CostUSD)
	}
	if !approx(base.Total.MarginalUSD, base.Total.CostUSD) {
		t.Fatalf("without override marginal should equal cost, got marginal=%f cost=%f", base.Total.MarginalUSD, base.Total.CostUSD)
	}

	override := map[string]usage.BillingType{"claude-code": usage.BillingSubscription}
	got := SummarizeAggregates(groups, engine, pricing.ModeCalculate, override)

	// cost_usd unchanged by the override.
	if !approx(got.Total.CostUSD, base.Total.CostUSD) {
		t.Fatalf("override changed cost_usd: got %f want %f", got.Total.CostUSD, base.Total.CostUSD)
	}

	// claude-code marginal is now 0; codex marginal still equals its cost.
	var claude, codex Bucket
	for _, b := range got.ByAgent {
		switch b.Name {
		case "claude-code":
			claude = b
		case "codex":
			codex = b
		}
	}
	if !approx(claude.MarginalUSD, 0) {
		t.Fatalf("overridden agent marginal should be 0, got %f", claude.MarginalUSD)
	}
	if !approx(claude.CostUSD, base.ByAgent[0].CostUSD) && claude.CostUSD <= 0 {
		t.Fatalf("overridden agent cost should be unchanged/positive, got %f", claude.CostUSD)
	}
	if !approx(codex.MarginalUSD, codex.CostUSD) || codex.CostUSD <= 0 {
		t.Fatalf("non-overridden agent marginal should equal cost, got marginal=%f cost=%f", codex.MarginalUSD, codex.CostUSD)
	}

	// Total marginal == codex cost only (claude zeroed).
	if !approx(got.Total.MarginalUSD, codex.CostUSD) {
		t.Fatalf("total marginal should equal codex cost, got %f want %f", got.Total.MarginalUSD, codex.CostUSD)
	}
}
