package api

import (
	"net/http"
	"time"

	"github.com/keithah/stint/internal/usagestats"
	"github.com/labstack/echo/v4"
)

// usageEventsBlocks groups recent events into ccusage-style 5-hour billing
// blocks and, for the currently active block, reports burn rate and
// projections at the current rate. Window defaults to the last 30 days but
// accepts range/start/end like the summary handler. The block-building and
// projection math live in internal/usagestats; the handler resolves params,
// loads events, and serializes the result.
func (s *Server) usageEventsBlocks(c echo.Context) error {
	user := userFromContext(c)
	now := time.Now()

	start, end, _, err := resolveUsageWindow(c, now)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	mode := usageCostMode(c)

	events, err := s.Store.UsageEventsBetween(c.Request().Context(), user.ID, start, end)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	events = filterEventsByAgent(events, c.QueryParam("agent"))

	// Use the same custom-pricing-aware engine as the summary so the burn-rate
	// panel and the summary card never disagree on a custom-priced model.
	engine, err := s.pricingEngineForUser(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}

	billingOverride, err := s.billingOverridesForUser(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}

	blocks, current := usagestats.Blocks(events, now, mode, engine, userLocation(user), billingOverride)

	blocksOut := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		blocksOut = append(blocksOut, map[string]any{
			"start":       b.Start.UTC().Format(time.RFC3339),
			"end":         b.End.UTC().Format(time.RFC3339),
			"is_active":   b.IsActive,
			"cost_usd":    b.CostUSD,
			"tokens":      b.Tokens,
			"event_count": b.EventCount,
		})
	}

	data := map[string]any{
		"cost_mode": string(mode),
		"blocks":    blocksOut,
		"current":   current,
	}

	return c.JSON(http.StatusOK, map[string]any{"data": data})
}
