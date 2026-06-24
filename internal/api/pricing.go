package api

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/keithah/stint/internal/pricing"
	"github.com/labstack/echo/v4"
)

// pricingSourceResponse describes one upstream price source for the settings
// "where our prices come from" panel: a link to its site, when it was last
// fetched, how many models it priced, and freshness status.
type pricingSourceResponse struct {
	Source     string `json:"source"`
	Label      string `json:"label"`
	URL        string `json:"url"`
	SiteURL    string `json:"site_url"`
	ModelCount int    `json:"model_count"`
	Status     string `json:"status"` // "ok" | "error" | "bundled"
	Error      string `json:"error,omitempty"`
	FetchedAt  string `json:"fetched_at,omitempty"`
}

// pricingModelResponse is one model's resolved rate (per-million USD, for human
// display), its source layer, and whether the user has overridden it.
type pricingModelResponse struct {
	Model                  string  `json:"model"`
	Source                 string  `json:"source"`
	Provider               string  `json:"provider,omitempty"`
	InputPerMillionUSD     float64 `json:"input_per_million_usd"`
	OutputPerMillionUSD    float64 `json:"output_per_million_usd"`
	CacheReadPerMillionUSD float64 `json:"cache_read_per_million_usd"`
	Overridden             bool    `json:"overridden"`
}

var pricingSiteURLs = map[string]string{
	"litellm":    "https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json",
	"openrouter": "https://openrouter.ai/models",
}

var pricingSourceLabels = map[string]string{
	"litellm":    "LiteLLM",
	"openrouter": "OpenRouter",
}

// listPricingSources reports each price source's link and freshness. Sources
// never fetched are shown as "bundled" (served from the embedded snapshot).
//
//	GET /api/v1/users/current/pricing/sources
func (s *Server) listPricingSources(c echo.Context) error {
	defaults := map[string]pricingSourceResponse{
		"litellm":    {Source: "litellm", Label: "LiteLLM", URL: pricing.LiteLLMURL, SiteURL: pricingSiteURLs["litellm"], Status: "bundled"},
		"openrouter": {Source: "openrouter", Label: "OpenRouter", URL: pricing.OpenRouterURL, SiteURL: pricingSiteURLs["openrouter"], Status: "bundled"},
	}
	rows, err := s.Store.ListPricingSnapshotMeta(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorBody(err.Error()))
	}
	for _, r := range rows {
		out := defaults[r.Source]
		if out.Source == "" {
			out = pricingSourceResponse{Source: r.Source, Label: pricingSourceLabels[r.Source], SiteURL: pricingSiteURLs[r.Source]}
		}
		out.URL = r.URL
		out.ModelCount = r.ModelCount
		out.Status = r.Status
		out.Error = r.Error
		if !r.FetchedAt.IsZero() {
			out.FetchedAt = r.FetchedAt.UTC().Format(time.RFC3339)
		}
		defaults[r.Source] = out
	}
	ordered := []pricingSourceResponse{defaults["litellm"], defaults["openrouter"]}
	return c.JSON(http.StatusOK, dataArray(ordered))
}

// listPricingModels returns the resolved per-model rates (per-million USD) the
// engine would apply, tagged by source and whether the user overrides each.
// Powers the settings model-rate viewer.
//
//	GET /api/v1/users/current/pricing/models
func (s *Server) listPricingModels(c echo.Context) error {
	if s.Pricing == nil {
		return c.JSON(http.StatusOK, dataArray([]pricingModelResponse{}))
	}
	user := userFromContext(c)
	overrides := map[string]struct{}{}
	if custom, err := s.Store.ListCustomPricing(c.Request().Context(), user.ID); err == nil {
		for _, p := range custom {
			overrides[pricing.Normalize(p.Model)] = struct{}{}
		}
	}
	entries := s.Pricing.Entries()
	out := make([]pricingModelResponse, 0, len(entries))
	for _, e := range entries {
		_, overridden := overrides[pricing.Normalize(e.Model)]
		out = append(out, pricingModelResponse{
			Model:                  e.Model,
			Source:                 e.Source,
			Provider:               e.Price.Provider,
			InputPerMillionUSD:     e.Price.InputPerToken * 1e6,
			OutputPerMillionUSD:    e.Price.OutputPerToken * 1e6,
			CacheReadPerMillionUSD: e.Price.CacheReadPerToken * 1e6,
			Overridden:             overridden,
		})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Model) < strings.ToLower(out[j].Model) })
	return c.JSON(http.StatusOK, dataArray(out))
}
