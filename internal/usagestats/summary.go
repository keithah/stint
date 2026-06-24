// Package usagestats holds the pure pricing/aggregation computation for usage
// events, kept out of the HTTP layer. It imports both internal/usage (event
// shape) and internal/pricing (cost engine); pricing already imports usage, so
// this package sits above both without creating an import cycle.
package usagestats

import (
	"sort"
	"time"

	"github.com/keithah/stint/internal/pricing"
	"github.com/keithah/stint/internal/usage"
)

// Totals are the aggregate cost/token figures across all events in the window.
type Totals struct {
	CostUSD     float64 `json:"cost_usd"`
	MarginalUSD float64 `json:"marginal_usd"`
	// UncachedCostUSD is what these tokens would cost with no prompt caching
	// (cache reads/writes priced at the full input rate). The gap to CostUSD is
	// the savings caching delivered — and why naive trackers over-report cost.
	UncachedCostUSD   float64 `json:"uncached_cost_usd"`
	EventCount        int     `json:"event_count"`
	InputTokens       int     `json:"input_tokens"`
	OutputTokens      int     `json:"output_tokens"`
	CacheCreateTokens int     `json:"cache_create_tokens"`
	CacheReadTokens   int     `json:"cache_read_tokens"`
	ReasoningTokens   int     `json:"reasoning_tokens"`
}

// Bucket is a single named aggregation row (one agent, model, or project).
// BillingType is the authoritative effective billing mode (post-override) and is
// only set for per-agent rows, where it is well-defined; it is "mixed" if an
// agent's events somehow span billing types, and empty for model/project rows.
type Bucket struct {
	Name        string  `json:"name"`
	CostUSD     float64 `json:"cost_usd"`
	MarginalUSD float64 `json:"marginal_usd"`
	Tokens      int     `json:"tokens"`
	EventCount  int     `json:"event_count"`
	BillingType string  `json:"billing_type,omitempty"`
}

// DayTotal is the priced total for one calendar day (in the user's timezone).
type DayTotal struct {
	Date        string  `json:"date"`
	CostUSD     float64 `json:"cost_usd"`
	MarginalUSD float64 `json:"marginal_usd"`
	Tokens      int     `json:"tokens"`
}

// Summary is the aggregated result of pricing a window of usage events. JSON
// tags match the client-facing response shape exactly; the handler adds the
// "range" and "cost_mode" fields around it.
type Summary struct {
	Total          Totals     `json:"total"`
	ByAgent        []Bucket   `json:"by_agent"`
	ByModel        []Bucket   `json:"by_model"`
	ByProject      []Bucket   `json:"by_project"`
	ByDay          []DayTotal `json:"by_day"`
	UnpricedModels []string   `json:"unpriced_models"`
}

// usageBucket is the mutable accumulator used while aggregating; it is converted
// to the exported Bucket on output.
type usageBucket struct {
	name        string
	costUSD     float64
	marginalUSD float64
	tokens      int
	eventCount  int
	billingType string // set only for per-agent buckets ("mixed" on conflict)
}

// Group is one pre-summed set of usage events that is homogeneous in every
// field that affects pricing (model, billing type, and whether a provider cost
// was present). Because pricing is linear per token type, pricing a group of
// summed tokens equals summing the per-event prices, so the SQL GROUP BY +
// per-group pricing collapses tens of thousands of events into a few hundred
// groups while producing byte-identical numbers. Day is already bucketed into
// the user's timezone (YYYY-MM-DD).
type Group struct {
	Agent       string
	Model       string
	Project     string
	Day         string
	BillingType string
	HasProvided bool

	Input         int
	Output        int
	CacheCreate5m int
	CacheCreate1h int
	CacheRead     int
	Reasoning     int

	ProvidedCostUSD float64
	EventCount      int
}

func (g Group) tokens() int {
	return g.Input + g.Output + g.CacheCreate5m + g.CacheCreate1h + g.CacheRead + g.Reasoning
}

// syntheticEvent rebuilds the usage.Event the pricing engine expects from a
// group's summed tokens, so the per-group pricing call is the exact same code
// path as the per-event one (cost/marginal/ModelResolved semantics unchanged).
// A per-agent billingOverride wins over the group's stored billing type so the
// user can reclassify an agent as subscription (zero marginal) or api at view
// time without re-collecting events.
func (g Group) syntheticEvent(billingOverride map[string]usage.BillingType) usage.Event {
	billing := usage.BillingType(g.BillingType)
	if override, ok := billingOverride[g.Agent]; ok {
		billing = override
	}
	e := usage.Event{
		Agent:               g.Agent,
		Model:               g.Model,
		Project:             g.Project,
		BillingType:         billing,
		InputTokens:         g.Input,
		OutputTokens:        g.Output,
		CacheCreate5mTokens: g.CacheCreate5m,
		CacheCreate1hTokens: g.CacheCreate1h,
		CacheReadTokens:     g.CacheRead,
		ReasoningTokens:     g.Reasoning,
	}
	if g.HasProvided {
		cost := g.ProvidedCostUSD
		e.CostUSDProvided = &cost
	}
	return e
}

// Summarize prices and aggregates the events into a Summary. Days are bucketed
// in loc (defaulting to UTC). A nil engine leaves costs at zero and records
// every non-empty model as unpriced. billingOverride, keyed by agent, reclasses
// an agent's events as subscription/api at view time (nil = use stored billing).
// It groups events in Go (by the same key SQL uses) then defers to
// SummarizeAggregates, so the accumulation logic lives in one place.
func Summarize(events []usage.Event, engine *pricing.Engine, mode pricing.Mode, loc *time.Location, billingOverride map[string]usage.BillingType) Summary {
	if loc == nil {
		loc = time.UTC
	}

	type groupKey struct {
		agent, model, project, day, billing string
		hasProvided                         bool
	}
	order := make([]groupKey, 0)
	groups := map[groupKey]*Group{}
	for _, event := range events {
		key := groupKey{
			agent:       event.Agent,
			model:       event.Model,
			project:     event.Project,
			day:         usageEventDay(event, loc),
			billing:     string(event.BillingType),
			hasProvided: event.CostUSDProvided != nil,
		}
		g := groups[key]
		if g == nil {
			g = &Group{
				Agent:       key.agent,
				Model:       key.model,
				Project:     key.project,
				Day:         key.day,
				BillingType: key.billing,
				HasProvided: key.hasProvided,
			}
			groups[key] = g
			order = append(order, key)
		}
		g.Input += event.InputTokens
		g.Output += event.OutputTokens
		g.CacheCreate5m += event.CacheCreate5mTokens
		g.CacheCreate1h += event.CacheCreate1hTokens
		g.CacheRead += event.CacheReadTokens
		g.Reasoning += event.ReasoningTokens
		if event.CostUSDProvided != nil {
			g.ProvidedCostUSD += *event.CostUSDProvided
		}
		g.EventCount++
	}

	out := make([]Group, 0, len(order))
	for _, key := range order {
		out = append(out, *groups[key])
	}
	return SummarizeAggregates(out, engine, mode, billingOverride)
}

// SummarizeAggregates prices each pre-summed group and accumulates the result
// into a Summary. Days are already bucketed (g.Day). A nil engine leaves costs
// at zero and records every non-empty model as unpriced. billingOverride, keyed
// by agent, reclasses an agent's billing type at view time (nil = use stored).
func SummarizeAggregates(groups []Group, engine *pricing.Engine, mode pricing.Mode, billingOverride map[string]usage.BillingType) Summary {
	var total Totals
	byAgent := map[string]*usageBucket{}
	byModel := map[string]*usageBucket{}
	byProject := map[string]*usageBucket{}
	type dayBucket struct {
		cost, marginal float64
		tokens         int
	}
	byDay := map[string]*dayBucket{}
	unpricedSet := map[string]struct{}{}

	bump := func(buckets map[string]*usageBucket, key string, cost, marginal float64, tokens, count int) {
		b := buckets[key]
		if b == nil {
			b = &usageBucket{name: key}
			buckets[key] = b
		}
		b.costUSD += cost
		b.marginalUSD += marginal
		b.tokens += tokens
		b.eventCount += count
	}

	for _, g := range groups {
		total.EventCount += g.EventCount
		total.InputTokens += g.Input
		total.OutputTokens += g.Output
		total.CacheCreateTokens += g.CacheCreate5m + g.CacheCreate1h
		total.CacheReadTokens += g.CacheRead
		total.ReasoningTokens += g.Reasoning
		tokens := g.tokens()

		var result pricing.Result
		if engine != nil {
			result = engine.Price(g.syntheticEvent(billingOverride), mode)
		}
		total.CostUSD += result.USD
		total.MarginalUSD += result.MarginalUSD
		total.UncachedCostUSD += result.UncachedUSD

		// Price already resolved the model (result.ModelResolved); reuse it instead
		// of a second engine.Has lookup on this polled hot path.
		if engine == nil || !result.ModelResolved {
			if g.Model != "" {
				unpricedSet[g.Model] = struct{}{}
			}
		}

		bump(byAgent, g.Agent, result.USD, result.MarginalUSD, tokens, g.EventCount)
		bump(byModel, g.Model, result.USD, result.MarginalUSD, tokens, g.EventCount)
		bump(byProject, g.Project, result.USD, result.MarginalUSD, tokens, g.EventCount)

		// Record the effective (post-override) billing mode on the agent bucket so
		// the client badges from ground truth instead of inferring it from the
		// cost/marginal ratio. An agent's groups share one billing type; "mixed"
		// guards the unexpected case where they don't.
		effBilling := g.BillingType
		if override, ok := billingOverride[g.Agent]; ok {
			effBilling = string(override)
		}
		if ab := byAgent[g.Agent]; ab.billingType == "" {
			ab.billingType = effBilling
		} else if ab.billingType != effBilling && effBilling != "" {
			ab.billingType = "mixed"
		}

		d := byDay[g.Day]
		if d == nil {
			d = &dayBucket{}
			byDay[g.Day] = d
		}
		d.cost += result.USD
		d.marginal += result.MarginalUSD
		d.tokens += tokens
	}

	unpriced := make([]string, 0, len(unpricedSet))
	for model := range unpricedSet {
		unpriced = append(unpriced, model)
	}
	sort.Strings(unpriced)

	days := make([]string, 0, len(byDay))
	for day := range byDay {
		days = append(days, day)
	}
	sort.Strings(days)
	byDayOut := make([]DayTotal, 0, len(days))
	for _, day := range days {
		d := byDay[day]
		byDayOut = append(byDayOut, DayTotal{
			Date:        day,
			CostUSD:     d.cost,
			MarginalUSD: d.marginal,
			Tokens:      d.tokens,
		})
	}

	return Summary{
		Total:          total,
		ByAgent:        sortedBuckets(byAgent),
		ByModel:        sortedBuckets(byModel),
		ByProject:      sortedBuckets(byProject),
		ByDay:          byDayOut,
		UnpricedModels: unpriced,
	}
}

func usageEventDay(event usage.Event, location *time.Location) string {
	ts, err := time.Parse(time.RFC3339, event.Timestamp)
	if err != nil {
		return "unknown"
	}
	return ts.In(location).Format("2006-01-02")
}

func sortedBuckets(buckets map[string]*usageBucket) []Bucket {
	out := make([]*usageBucket, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].costUSD != out[j].costUSD {
			return out[i].costUSD > out[j].costUSD
		}
		return out[i].name < out[j].name
	})
	result := make([]Bucket, 0, len(out))
	for _, b := range out {
		result = append(result, Bucket{
			Name:        b.name,
			CostUSD:     b.costUSD,
			MarginalUSD: b.marginalUSD,
			Tokens:      b.tokens,
			EventCount:  b.eventCount,
			BillingType: b.billingType,
		})
	}
	return result
}
