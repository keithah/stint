package api

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/keithah/stint/internal/db"
	"github.com/labstack/echo/v4"
)

const customPricingLimit = 200

// listCustomPricing returns the authenticated user's custom model prices.
//
// Suggested route (register with requireLocalAccountAccess):
//
//	GET /api/v1/users/current/custom_pricing
func (s *Server) listCustomPricing(c echo.Context) error {
	user := userFromContext(c)
	prices, err := s.Store.ListCustomPricing(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, dataArray(prices))
}

// upsertCustomPricing inserts or updates one custom model price (per-million USD).
//
// Suggested route (register with requireLocalAccountAccess):
//
//	PUT /api/v1/users/current/custom_pricing
func (s *Server) upsertCustomPricing(c echo.Context) error {
	user := userFromContext(c)
	var price db.CustomPricing
	if err := c.Bind(&price); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("invalid JSON body"))
	}
	price.Model = strings.TrimSpace(price.Model)
	if err := db.ValidateCustomPricing(price); err != nil {
		return c.JSON(http.StatusBadRequest, errorBody(err.Error()))
	}
	existing, err := s.Store.ListCustomPricing(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	isNew := true
	for _, p := range existing {
		if p.Model == price.Model {
			isNew = false
			break
		}
	}
	if isNew && len(existing) >= customPricingLimit {
		return c.JSON(http.StatusBadRequest, errorBody("custom pricing limit reached"))
	}
	if err := s.Store.UpsertCustomPricing(c.Request().Context(), user.ID, price); err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	prices, err := s.Store.ListCustomPricing(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.JSON(http.StatusOK, dataArray(prices))
}

// deleteCustomPricing removes one custom model price by model id.
//
// Suggested route (register with requireLocalAccountAccess):
//
//	DELETE /api/v1/users/current/custom_pricing/:model
func (s *Server) deleteCustomPricing(c echo.Context) error {
	user := userFromContext(c)
	model, err := url.PathUnescape(c.Param("model"))
	if err != nil {
		model = c.Param("model")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return c.JSON(http.StatusBadRequest, errorBody("model is required"))
	}
	if err := s.Store.DeleteCustomPricing(c.Request().Context(), user.ID, model); err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	return c.NoContent(http.StatusNoContent)
}
