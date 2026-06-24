package services

import (
	"testing"
	"time"

	"github.com/keithah/stint/internal/pricing"
	"github.com/keithah/stint/internal/usagestats"
)

// ApplyUsageEventCosts overwrites mapped agent rows with engine cost but leaves
// rows the engine never produces (e.g. "Unknown") on their heartbeat estimate.
func TestApplyUsageEventCostsUnmappedFallback(t *testing.T) {
	engine, err := pricing.NewFromBundled()
	if err != nil {
		t.Fatalf("load prices: %v", err)
	}
	windowEnd := time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)
	m := &AIMetrics{
		Agents: []AIStat{
			{Name: "gpt", EstimatedCostCents: 999},     // mapped -> overwritten
			{Name: "Unknown", EstimatedCostCents: 42},   // unmapped -> kept
		},
	}
	groups := []usagestats.Group{
		{Agent: "codex", Model: "gpt-5.5", Day: "2026-06-24", Input: 1000, Output: 500, CacheRead: 100000, EventCount: 1},
	}
	ApplyUsageEventCosts(m, groups, engine, windowEnd)

	// "gpt" is the mapped target for non-claude/gemini models; it gets engine cost.
	if m.Agents[0].EstimatedCostCents == 999 || m.Agents[0].EstimatedCostCents <= 0 {
		t.Fatalf("gpt cost should be overwritten with engine price, got %d", m.Agents[0].EstimatedCostCents)
	}
	// "Unknown" is never produced by the engine -> heartbeat estimate preserved.
	if m.Agents[1].EstimatedCostCents != 42 {
		t.Fatalf("Unknown cost should keep heartbeat estimate 42, got %d", m.Agents[1].EstimatedCostCents)
	}
	// Tool breakdown carries the usage_events agent name.
	if len(m.ToolCosts) != 1 || m.ToolCosts[0].Name != "codex" {
		t.Fatalf("expected one tool 'codex', got %+v", m.ToolCosts)
	}
}

func TestHeartbeatAgentForModel(t *testing.T) {
	cases := map[string]string{
		"claude-opus-4-8":      "anthropic",
		"gemini-2.5-flash":     "gemini",
		"gpt-5.5":              "gpt",
		"openai/gpt-5.3-codex": "gpt",
	}
	for model, want := range cases {
		if got := HeartbeatAgentForModel(model); got != want {
			t.Fatalf("%s: want %s, got %s", model, want, got)
		}
	}
}
