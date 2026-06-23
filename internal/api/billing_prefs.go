package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/usage"
	"github.com/labstack/echo/v4"
)

const billingPrefsLimit = 200

// listBillingPrefs returns the authenticated user's per-agent billing overrides.
//
// Suggested route (register with readLimit):
//
//	GET /api/v1/users/current/billing_prefs
func (s *Server) listBillingPrefs(c echo.Context) error {
	user := userFromContext(c)
	prefs, err := s.Store.ListBillingPrefs(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, dataArray(prefs))
}

// upsertBillingPref inserts or updates one per-agent billing override.
//
// Suggested route (register with requireLocalAccountAccess):
//
//	PUT /api/v1/users/current/billing_prefs
func (s *Server) upsertBillingPref(c echo.Context) error {
	user := userFromContext(c)
	var pref db.BillingPref
	if err := c.Bind(&pref); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid JSON body"))
	}
	pref.Agent = strings.TrimSpace(pref.Agent)
	if err := db.ValidateBillingPref(pref); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	existing, err := s.Store.ListBillingPrefs(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	isNew := true
	for _, p := range existing {
		if p.Agent == pref.Agent {
			isNew = false
			break
		}
	}
	if isNew && len(existing) >= billingPrefsLimit {
		return c.JSON(http.StatusBadRequest, errorBody("billing prefs limit reached"))
	}
	if err := s.Store.UpsertBillingPref(c.Request().Context(), user.ID, pref); err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	prefs, err := s.Store.ListBillingPrefs(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, dataArray(prefs))
}

// deleteBillingPref removes one per-agent billing override by agent.
//
// Suggested route (register with requireLocalAccountAccess):
//
//	DELETE /api/v1/users/current/billing_prefs/:agent
func (s *Server) deleteBillingPref(c echo.Context) error {
	user := userFromContext(c)
	agent, err := url.PathUnescape(c.Param("agent"))
	if err != nil {
		agent = c.Param("agent")
	}
	agent = strings.TrimSpace(agent)
	if agent == "" {
		return c.JSON(http.StatusBadRequest, errorBody("agent is required"))
	}
	if err := s.Store.DeleteBillingPref(c.Request().Context(), user.ID, agent); err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.NoContent(http.StatusNoContent)
}

// billingOverridesForUser loads the user's per-agent billing overrides into a
// map[agent]BillingType for the pricing path. Returns an empty (non-nil) map
// when the user has no overrides so callers can index it unconditionally.
func (s *Server) billingOverridesForUser(ctx context.Context, userID uuid.UUID) (map[string]usage.BillingType, error) {
	prefs, err := s.Store.ListBillingPrefs(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]usage.BillingType, len(prefs))
	for _, p := range prefs {
		out[p.Agent] = usage.BillingType(p.BillingType)
	}
	return out, nil
}
