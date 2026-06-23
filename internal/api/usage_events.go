package api

import (
	"net/http"
	"sort"
	"time"

	"github.com/keithah/stint/internal/pricing"
	"github.com/keithah/stint/internal/services"
	"github.com/keithah/stint/internal/usage"
	"github.com/labstack/echo/v4"
)

const usageEventsBulkLimit = 5000

// createUsageEventsBulk ingests a batch of canonical AI usage events. Ingest is
// idempotent: re-sending the same events collapses to duplicates via the store
// dedup, so totals stay stable across re-scans.
func (s *Server) createUsageEventsBulk(c echo.Context) error {
	user := userFromContext(c)
	var events []usage.Event
	if err := c.Bind(&events); err != nil {
		return c.JSON(http.StatusBadRequest, wakaError("invalid usage events JSON"))
	}
	if len(events) > usageEventsBulkLimit {
		return c.JSON(http.StatusBadRequest, wakaError("bulk usage event limit is 5000"))
	}
	result, err := s.Store.InsertUsageEvents(c.Request().Context(), user.ID, events)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, map[string]any{"data": result})
}

// listUsageEvents exports stored usage events in a time window for the
// authenticated user. Window defaults to the last 30 days.
func (s *Server) listUsageEvents(c echo.Context) error {
	user := userFromContext(c)
	start, end, err := usageEventWindow(c.QueryParam("start"), c.QueryParam("end"), time.Now())
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	events, err := s.Store.UsageEventsBetween(c.Request().Context(), user.ID, start, end)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	if events == nil {
		events = []usage.Event{}
	}
	return c.JSON(http.StatusOK, map[string]any{"data": events})
}

// usageEventsSummary loads events for the window, prices each one, and returns
// aggregated cost/token totals plus breakdowns by agent, model, project, and
// day. Days are bucketed in the user's profile timezone.
func (s *Server) usageEventsSummary(c echo.Context) error {
	user := userFromContext(c)
	now := time.Now()

	rangeName := c.QueryParam("range")
	var start, end time.Time
	var rangeLabel string
	if rangeName != "" {
		window, err := services.WindowForRange(now, rangeName)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
		}
		start, end, rangeLabel = window.Start, window.End, window.Range
	} else {
		var err error
		start, end, err = usageEventWindow(c.QueryParam("start"), c.QueryParam("end"), now)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
		}
		rangeLabel = start.Format("2006-01-02") + " to " + end.Format("2006-01-02")
	}

	costMode := c.QueryParam("cost_mode")
	if costMode == "" {
		costMode = string(pricing.ModeAuto)
	}
	mode := pricing.Mode(costMode)

	events, err := s.Store.UsageEventsBetween(c.Request().Context(), user.ID, start, end)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}

	// Optional single-agent filter (per-agent drill-down + ccusage cross-check).
	if agent := c.QueryParam("agent"); agent != "" {
		filtered := events[:0:0]
		for _, e := range events {
			if e.Agent == agent {
				filtered = append(filtered, e)
			}
		}
		events = filtered
	}

	location := userLocation(user)
	summary := summarizeUsageEvents(events, s.Pricing, mode, location)
	summary["range"] = rangeLabel
	summary["cost_mode"] = costMode

	return c.JSON(http.StatusOK, map[string]any{"data": summary})
}

// usageEventWindow parses start/end query params (RFC3339 or YYYY-MM-DD),
// defaulting to the last 30 days when both are empty.
func usageEventWindow(startParam, endParam string, now time.Time) (time.Time, time.Time, error) {
	end := now
	if endParam != "" {
		parsed, err := parseUsageTime(endParam, true)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		end = parsed
	}
	start := end.AddDate(0, 0, -30)
	if startParam != "" {
		parsed, err := parseUsageTime(startParam, false)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		start = parsed
	}
	return start.UTC(), end.UTC(), nil
}

// parseUsageTime accepts an RFC3339 timestamp or a YYYY-MM-DD date. For a bare
// date used as an end bound, endOfDay extends it to the start of the next day so
// the range is inclusive of the named day.
func parseUsageTime(value string, endOfDay bool) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, err
	}
	parsed = parsed.UTC()
	if endOfDay {
		parsed = parsed.AddDate(0, 0, 1)
	}
	return parsed, nil
}

type usageBucket struct {
	name        string
	costUSD     float64
	marginalUSD float64
	tokens      int
	eventCount  int
}

// summarizeUsageEvents prices and aggregates the events. The returned map omits
// "range" and "cost_mode"; the handler fills those in.
func summarizeUsageEvents(events []usage.Event, engine *pricing.Engine, mode pricing.Mode, location *time.Location) map[string]any {
	if location == nil {
		location = time.UTC
	}

	var (
		totalCost, totalMarginal                                              float64
		inputTokens, outputTokens, cacheCreate, cacheRead, reasoning, evCount int
	)
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
		evCount++
		inputTokens += event.InputTokens
		outputTokens += event.OutputTokens
		cacheCreate += event.CacheCreate5mTokens + event.CacheCreate1hTokens
		cacheRead += event.CacheReadTokens
		reasoning += event.ReasoningTokens
		tokens := event.TotalTokens()

		var result pricing.Result
		if engine != nil {
			result = engine.Price(event, mode)
		}
		totalCost += result.USD
		totalMarginal += result.MarginalUSD

		if engine == nil || !engine.Has(event.Model) {
			if event.Model != "" {
				unpricedSet[event.Model] = struct{}{}
			}
		}

		bump(byAgent, event.Agent, result.USD, result.MarginalUSD, tokens)
		bump(byModel, event.Model, result.USD, result.MarginalUSD, tokens)
		bump(byProject, event.Project, result.USD, result.MarginalUSD, tokens)

		day := usageEventDay(event, location)
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
	byDayOut := make([]map[string]any, 0, len(days))
	for _, day := range days {
		d := byDay[day]
		byDayOut = append(byDayOut, map[string]any{
			"date":         day,
			"cost_usd":     d.cost,
			"marginal_usd": d.marginal,
			"tokens":       d.tokens,
		})
	}

	return map[string]any{
		"total": map[string]any{
			"cost_usd":            totalCost,
			"marginal_usd":        totalMarginal,
			"event_count":         evCount,
			"input_tokens":        inputTokens,
			"output_tokens":       outputTokens,
			"cache_create_tokens": cacheCreate,
			"cache_read_tokens":   cacheRead,
			"reasoning_tokens":    reasoning,
		},
		"by_agent":        sortedBuckets(byAgent),
		"by_model":        sortedBuckets(byModel),
		"by_project":      sortedBuckets(byProject),
		"by_day":          byDayOut,
		"unpriced_models": unpriced,
	}
}

func usageEventDay(event usage.Event, location *time.Location) string {
	ts, err := time.Parse(time.RFC3339, event.Timestamp)
	if err != nil {
		return "unknown"
	}
	return ts.In(location).Format("2006-01-02")
}

func sortedBuckets(buckets map[string]*usageBucket) []map[string]any {
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
	result := make([]map[string]any, 0, len(out))
	for _, b := range out {
		result = append(result, map[string]any{
			"name":         b.name,
			"cost_usd":     b.costUSD,
			"marginal_usd": b.marginalUSD,
			"tokens":       b.tokens,
			"event_count":  b.eventCount,
		})
	}
	return result
}
