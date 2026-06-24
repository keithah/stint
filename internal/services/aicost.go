package services

import (
	"sort"
	"strings"
	"time"

	"github.com/keithah/stint/internal/pricing"
	"github.com/keithah/stint/internal/usagestats"
)

// HeartbeatAgentForModel maps a usage_events model to the coarser heartbeat agent
// taxonomy the AI panel's Agents/Cost Tracker use (gpt/anthropic/...). It is
// provider-based and deterministic, sidestepping the unreliable heartbeat↔usage
// session-id join. codex and opencode both run OpenAI-family models, so both fold
// into "gpt" — matching how heartbeats already lump them and where line counts live.
func HeartbeatAgentForModel(model string) string {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "claude"):
		return "anthropic"
	case strings.Contains(m, "gemini"):
		return "gemini"
	default:
		return "gpt"
	}
}

// AICostWindow resolves the [start, end) usage-events window for an AI cost bake
// for the given stats range. all_time spans a wide range ending at the start of
// tomorrow (the period anchor for daily/weekly/monthly buckets); named ranges
// defer to WindowForRange. ok is false for an unsupported range.
func AICostWindow(now time.Time, rangeName string) (start, end time.Time, ok bool) {
	end = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)
	if rangeName == "all_time" {
		return end.AddDate(-100, 0, 0), end, true
	}
	w, err := WindowForRange(now, rangeName)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	return w.Start, w.End, true
}

// ApplyUsageEventCosts overwrites an AIMetrics' cost fields with cache-aware list
// prices computed from usage_events (the accurate source), replacing the legacy
// heartbeat token-rate estimate. toolGroups are the usage-event groups in the
// stats window; windowEnd anchors the daily/weekly/monthly period buckets.
//
// Two taxonomies share one set of priced events: "tool" keeps the fine-grained
// usage_events agent (codex/claude/opencode); "agent" maps each event up to the
// heartbeat agent so cost lands on the rows that carry line counts. Costs use the
// equivalent-API list price (never subscription/custom discounts).
//
// Unmapped fallback: an Agents row whose name the engine never produces (e.g.
// "openai"/"pro"/"Unknown") keeps its existing heartbeat estimate rather than
// being zeroed — its spend is small and folded into the mapped rows.
func ApplyUsageEventCosts(m *AIMetrics, groups []usagestats.Group, engine *pricing.Engine, windowEnd time.Time) {
	if m == nil || engine == nil || len(groups) == 0 {
		return
	}
	// Price each event once, then bucket the same priced slice under both
	// taxonomies: by usage tool (g.Agent) and by mapped heartbeat agent. The
	// USD is identical across taxonomies — only the bucket key differs.
	priced := usagestats.PriceGroups(groups, engine, pricing.ModeCalculate)
	byTool := usagestats.BucketCents(priced, func(g usagestats.Group) string { return g.Agent }, windowEnd)
	byAgent := usagestats.BucketCents(priced, func(g usagestats.Group) string { return HeartbeatAgentForModel(g.Model) }, windowEnd)

	m.EstimatedCostCents = byTool.TotalCents
	for i := range m.Agents {
		if c, ok := byAgent.ByKeyCents[m.Agents[i].Name]; ok {
			m.Agents[i].EstimatedCostCents = c
		}
		// else: leave the heartbeat estimate already on the row (unmapped fallback).
	}
	for i := range m.Days {
		if c, ok := byTool.ByDayCents[m.Days[i].Name]; ok {
			m.Days[i].EstimatedCostCents = c
		}
	}
	m.Costs = periodsFromCosts(byAgent)
	m.ToolCosts = toolCosts(priced, byTool.ByKeyCents)
}

// toolCosts builds the per-tool breakdown (cost + token mix) from the priced
// groups, sorted by spend descending.
func toolCosts(priced []usagestats.GroupCost, costByTool map[string]int) []AIToolCost {
	tokens := map[string]*AIToolCost{}
	for _, c := range priced {
		t := tokens[c.Group.Agent]
		if t == nil {
			t = &AIToolCost{Name: c.Group.Agent, CostCents: costByTool[c.Group.Agent]}
			tokens[c.Group.Agent] = t
		}
		t.InputTokens += c.Group.Input
		t.OutputTokens += c.Group.Output
		t.CacheReadTokens += c.Group.CacheRead
	}
	out := make([]AIToolCost, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CostCents != out[j].CostCents {
			return out[i].CostCents > out[j].CostCents
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func periodsFromCosts(costs usagestats.AICostCents) []AICostPeriod {
	periods := make([]AICostPeriod, 0, len(costs.PeriodByKey))
	for agent, p := range costs.PeriodByKey {
		periods = append(periods, AICostPeriod{
			Agent:        agent,
			DailyCents:   p.Daily,
			WeeklyCents:  p.Weekly,
			MonthlyCents: p.Monthly,
			TotalCents:   p.Total,
		})
	}
	sort.Slice(periods, func(i, j int) bool {
		if periods[i].TotalCents != periods[j].TotalCents {
			return periods[i].TotalCents > periods[j].TotalCents
		}
		return periods[i].Agent < periods[j].Agent
	})
	return periods
}
