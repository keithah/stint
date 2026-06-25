package api

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/pricing"
	"github.com/keithah/stint/internal/services"
	"github.com/keithah/stint/internal/usage"
	"github.com/keithah/stint/internal/usagestats"
	"github.com/labstack/echo/v4"
)

const usageEventsBulkLimit = 5000

// createUsageEventsBulk ingests a batch of canonical AI usage events. Ingest is
// an upsert: re-sending an event updates the stored row to the latest token/cost
// values, so a corrected re-scan fixes totals rather than being dropped.
// Re-ingested rows are reported as duplicates (not inserts) by the store.
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

// usageEventsSummary sums token counts per pricing group in SQL (GROUP BY),
// prices each group in Go, and returns aggregated cost/token totals plus
// breakdowns by agent, model, project, and day. Summing in SQL collapses a
// window of tens of thousands of events into a few hundred groups; pricing is
// linear per token type, so the result is byte-identical to pricing each event.
// Days are bucketed in the user's profile timezone (in SQL). The agent filter
// runs in SQL so it uses the (user_id, agent, ts) index. The aggregation itself
// lives in internal/usagestats; the handler resolves params, maps db rows to
// usagestats.Group, and serializes the result (adding range/cost_mode).
func (s *Server) usageEventsSummary(c echo.Context) error {
	user := userFromContext(c)
	now := time.Now()

	start, end, rangeLabel, err := resolveUsageWindow(c, now)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	mode := usageCostMode(c)

	aggs, err := s.Store.UsageAggregatesBetween(c.Request().Context(), user.ID, start, end, c.QueryParam("agent"), "", userLocation(user).String())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}

	engine, err := s.pricingEngineForUser(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}

	billingOverride, err := s.billingOverridesForUser(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}

	groups := make([]usagestats.Group, 0, len(aggs))
	for _, a := range aggs {
		groups = append(groups, usagestats.Group{
			Agent:           a.Agent,
			Model:           a.Model,
			Project:         a.Project,
			Day:             a.Day,
			BillingType:     a.BillingType,
			HasProvided:     a.HasProvided,
			Input:           int(a.InputTokens),
			Output:          int(a.OutputTokens),
			CacheCreate5m:   int(a.CacheCreate5mTokens),
			CacheCreate1h:   int(a.CacheCreate1hTokens),
			CacheRead:       int(a.CacheReadTokens),
			Reasoning:       int(a.ReasoningTokens),
			ProvidedCostUSD: a.ProvidedCostUSD,
			EventCount:      a.EventCount,
		})
	}

	summary := usagestats.SummarizeAggregates(groups, engine, mode, billingOverride)

	return c.JSON(http.StatusOK, map[string]any{"data": struct {
		usagestats.Summary
		Range    string `json:"range"`
		CostMode string `json:"cost_mode"`
	}{Summary: summary, Range: rangeLabel, CostMode: string(mode)}})
}

// resolveUsageWindow resolves the time window for a usage request from either a
// named `range` (the supported stats ranges) or `start`/`end` params, returning
// a human-readable range label. Shared by the summary and blocks endpoints so
// they always cover the same window for the same query.
func resolveUsageWindow(c echo.Context, now time.Time) (time.Time, time.Time, string, error) {
	if rangeName := c.QueryParam("range"); rangeName != "" {
		window, err := services.WindowForRange(now, rangeName)
		if err != nil {
			return time.Time{}, time.Time{}, "", err
		}
		return window.Start, window.End, window.Range, nil
	}
	start, end, err := usageEventWindow(c.QueryParam("start"), c.QueryParam("end"), now)
	if err != nil {
		return time.Time{}, time.Time{}, "", err
	}
	return start, end, start.Format("2006-01-02") + " to " + end.Format("2006-01-02"), nil
}

func usageCostMode(c echo.Context) pricing.Mode {
	mode := c.QueryParam("cost_mode")
	if mode == "" {
		mode = string(pricing.ModeAuto)
	}
	return pricing.Mode(mode)
}

// filterEventsByAgent narrows events to a single agent in place. Empty agent is
// a no-op. Shared so per-agent drill-down behaves identically across endpoints.
func filterEventsByAgent(events []usage.Event, agent string) []usage.Event {
	if agent == "" {
		return events
	}
	filtered := events[:0:0]
	for _, e := range events {
		if e.Agent == agent {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// pricingEngineForUser returns a pricing engine with the user's custom pricing
// applied as per-request overrides. s.Pricing is shared across requests and is
// never mutated: WithOverrides returns a shallow copy that shares the base table
// but carries its own overrides map. Returns the shared engine unchanged when
// the user has no overrides.
func (s *Server) pricingEngineForUser(ctx context.Context, userID uuid.UUID) (*pricing.Engine, error) {
	if s.Pricing == nil {
		return nil, nil
	}
	custom, err := s.Store.ListCustomPricing(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(custom) == 0 {
		return s.Pricing, nil
	}
	overrides := make(map[string]pricing.ModelPrice, len(custom))
	for _, p := range custom {
		overrides[p.Model] = pricing.ModelPrice{
			InputPerToken:         p.InputPerMillionUSD / 1e6,
			OutputPerToken:        p.OutputPerMillionUSD / 1e6,
			CacheCreate5mPerToken: p.CacheWritePerMillionUSD / 1e6,
			CacheCreate1hPerToken: p.CacheWritePerMillionUSD / 1e6,
			CacheReadPerToken:     p.CacheReadPerMillionUSD / 1e6,
		}
	}
	return s.Pricing.WithOverrides(overrides), nil
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
