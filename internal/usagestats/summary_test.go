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
		fromEvents := Summarize(events, engine, mode, time.UTC)
		fromGroups := SummarizeAggregates(groups, engine, mode)
		if !reflect.DeepEqual(fromEvents, fromGroups) {
			t.Fatalf("mode %s: aggregates summary != events summary\n events=%+v\n groups=%+v", mode, fromEvents, fromGroups)
		}
	}
}
