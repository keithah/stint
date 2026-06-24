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
func ApplyUsageEventCosts(m *AIMetrics, toolGroups []usagestats.Group, engine *pricing.Engine, windowEnd time.Time) {
	if m == nil || engine == nil || len(toolGroups) == 0 {
		return
	}
	agentGroups := make([]usagestats.Group, len(toolGroups))
	toolTokens := map[string]*AIToolCost{}
	for i, g := range toolGroups {
		ag := g
		ag.Agent = HeartbeatAgentForModel(g.Model)
		agentGroups[i] = ag

		t := toolTokens[g.Agent]
		if t == nil {
			t = &AIToolCost{Name: g.Agent}
			toolTokens[g.Agent] = t
		}
		t.InputTokens += g.Input
		t.OutputTokens += g.Output
		t.CacheReadTokens += g.CacheRead
	}

	toolCosts := usagestats.AICostsFromGroups(toolGroups, engine, pricing.ModeCalculate, windowEnd)
	agentCosts := usagestats.AICostsFromGroups(agentGroups, engine, pricing.ModeCalculate, windowEnd)

	m.EstimatedCostCents = toolCosts.TotalCents
	for i := range m.Agents {
		if c, ok := agentCosts.ByAgentCents[m.Agents[i].Name]; ok {
			m.Agents[i].EstimatedCostCents = c
		}
		// else: leave the heartbeat estimate already on the row (unmapped fallback).
	}
	for i := range m.Days {
		if c, ok := toolCosts.ByDayCents[m.Days[i].Name]; ok {
			m.Days[i].EstimatedCostCents = c
		}
	}
	m.Costs = periodsFromCosts(agentCosts)

	tools := make([]AIToolCost, 0, len(toolTokens))
	for name, t := range toolTokens {
		t.CostCents = toolCosts.ByAgentCents[name]
		tools = append(tools, *t)
	}
	sort.Slice(tools, func(i, j int) bool {
		if tools[i].CostCents != tools[j].CostCents {
			return tools[i].CostCents > tools[j].CostCents
		}
		return tools[i].Name < tools[j].Name
	})
	m.ToolCosts = tools
}

func periodsFromCosts(costs usagestats.AICostCents) []AICostPeriod {
	periods := make([]AICostPeriod, 0, len(costs.PeriodByAgent))
	for agent, p := range costs.PeriodByAgent {
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
