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
	CostUSD           float64 `json:"cost_usd"`
	MarginalUSD       float64 `json:"marginal_usd"`
	EventCount        int     `json:"event_count"`
	InputTokens       int     `json:"input_tokens"`
	OutputTokens      int     `json:"output_tokens"`
	CacheCreateTokens int     `json:"cache_create_tokens"`
	CacheReadTokens   int     `json:"cache_read_tokens"`
	ReasoningTokens   int     `json:"reasoning_tokens"`
}

// Bucket is a single named aggregation row (one agent, model, or project).
type Bucket struct {
	Name        string  `json:"name"`
	CostUSD     float64 `json:"cost_usd"`
	MarginalUSD float64 `json:"marginal_usd"`
	Tokens      int     `json:"tokens"`
	EventCount  int     `json:"event_count"`
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
}

// Summarize prices and aggregates the events into a Summary. Days are bucketed
// in loc (defaulting to UTC). A nil engine leaves costs at zero and records
// every non-empty model as unpriced.
func Summarize(events []usage.Event, engine *pricing.Engine, mode pricing.Mode, loc *time.Location) Summary {
	if loc == nil {
		loc = time.UTC
	}

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

	bump := func(buckets map[string]*usageBucket, key string, cost, marginal float64, tokens int) {
		b := buckets[key]
		if b == nil {
			b = &usageBucket{name: key}
			buckets[key] = b
		}
		b.costUSD += cost
		b.marginalUSD += marginal
		b.tokens += tokens
		b.eventCount++
	}

	for _, event := range events {
		total.EventCount++
		total.InputTokens += event.InputTokens
		total.OutputTokens += event.OutputTokens
		total.CacheCreateTokens += event.CacheCreate5mTokens + event.CacheCreate1hTokens
		total.CacheReadTokens += event.CacheReadTokens
		total.ReasoningTokens += event.ReasoningTokens
		tokens := event.TotalTokens()

		var result pricing.Result
		if engine != nil {
			result = engine.Price(event, mode)
		}
		total.CostUSD += result.USD
		total.MarginalUSD += result.MarginalUSD

		// Price already resolved the model (result.ModelResolved); reuse it instead
		// of a second engine.Has lookup per event on this polled hot path.
		if engine == nil || !result.ModelResolved {
			if event.Model != "" {
				unpricedSet[event.Model] = struct{}{}
			}
		}

		bump(byAgent, event.Agent, result.USD, result.MarginalUSD, tokens)
		bump(byModel, event.Model, result.USD, result.MarginalUSD, tokens)
		bump(byProject, event.Project, result.USD, result.MarginalUSD, tokens)

		day := usageEventDay(event, loc)
		d := byDay[day]
		if d == nil {
			d = &dayBucket{}
			byDay[day] = d
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
		})
	}
	return result
}
