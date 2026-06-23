// Package pricing turns canonical usage events into USD using a LiteLLM-sourced
// price table. It is decoupled from ingestion and storage: it knows models and
// token types, nothing about agents. Prices come from a bundled offline
// snapshot (refreshable) plus optional user overrides; business logic never
// hardcodes a price.
package pricing

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

//go:embed data/litellm_prices.json
var bundledPrices []byte

// Mode selects how cost is derived (mirrors ccusage).
type Mode string

const (
	// ModeAuto uses the provider-reported cost when present, else calculates.
	ModeAuto Mode = "auto"
	// ModeCalculate always recomputes from tokens at current prices.
	ModeCalculate Mode = "calculate"
	// ModeDisplay only shows provider-reported cost; never estimates.
	ModeDisplay Mode = "display"
)

// ModelPrice is the per-token USD cost for each token type.
type ModelPrice struct {
	InputPerToken         float64 `json:"input_cost_per_token"`
	OutputPerToken        float64 `json:"output_cost_per_token"`
	CacheCreate5mPerToken float64 `json:"cache_creation_input_token_cost"`
	CacheCreate1hPerToken float64 `json:"cache_creation_input_token_cost_above_1hr"`
	CacheReadPerToken     float64 `json:"cache_read_input_token_cost"`
	Provider              string  `json:"litellm_provider"`
}

// Result is the outcome of pricing one event.
type Result struct {
	// USD is the equivalent-API cost: provider-reported or calculated per Mode.
	USD float64
	// MarginalUSD is real out-of-pocket spend: 0 for subscription usage.
	MarginalUSD float64
	// Priced is false when the model is unknown and no provider cost exists.
	Priced bool
	// Source is "provided", "calculated", or "unpriced".
	Source string
}

// Engine prices events against a model table with optional overrides.
type Engine struct {
	table     map[string]ModelPrice
	overrides map[string]ModelPrice
	aliases   map[string]string
}

// NewFromBundled builds an engine from the embedded offline snapshot.
func NewFromBundled() (*Engine, error) {
	table, err := parseLiteLLM(bundledPrices)
	if err != nil {
		return nil, err
	}
	return &Engine{table: table, overrides: map[string]ModelPrice{}, aliases: defaultAliases()}, nil
}

// New builds an engine from a caller-provided LiteLLM JSON snapshot (e.g. a
// freshly refreshed download), falling back to nothing if it fails to parse.
func New(snapshot []byte) (*Engine, error) {
	table, err := parseLiteLLM(snapshot)
	if err != nil {
		return nil, err
	}
	return &Engine{table: table, overrides: map[string]ModelPrice{}, aliases: defaultAliases()}, nil
}

// SetOverrides installs user custom pricing for private/proxied/unknown models.
// Override keys are matched after normalization and win over the base table.
func (e *Engine) SetOverrides(overrides map[string]ModelPrice) {
	e.overrides = map[string]ModelPrice{}
	for model, price := range overrides {
		e.overrides[strings.ToLower(strings.TrimSpace(model))] = price
	}
}

var dateSuffix = regexp.MustCompile(`-20\d{6}$`)

// Normalize maps a raw provider/proxy model string to a canonical id used for
// lookup: lowercased, region/provider prefixes stripped, trailing date removed.
func Normalize(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return ""
	}
	// proxy prefixes like "openrouter/anthropic/claude-..." or "bedrock/..."
	if idx := strings.LastIndex(m, "/"); idx >= 0 {
		m = m[idx+1:]
	}
	// region/provider dotted prefixes: us.anthropic.claude-... , global.anthropic....
	for {
		changed := false
		for _, prefix := range []string{"anthropic.", "us.", "eu.", "au.", "apac.", "global."} {
			if strings.HasPrefix(m, prefix) {
				m = strings.TrimPrefix(m, prefix)
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return m
}

// lookup resolves a raw model string to a price, trying (in order) overrides,
// exact table hit, normalized hit, alias, and date-stripped normalized hit.
func (e *Engine) lookup(model string) (ModelPrice, bool) {
	norm := Normalize(model)
	if price, ok := e.overrides[norm]; ok {
		return price, true
	}
	if price, ok := e.table[strings.ToLower(strings.TrimSpace(model))]; ok {
		return price, true
	}
	if price, ok := e.table[norm]; ok {
		return price, true
	}
	if canonical, ok := e.aliases[norm]; ok {
		if price, ok := e.table[canonical]; ok {
			return price, true
		}
	}
	if stripped := dateSuffix.ReplaceAllString(norm, ""); stripped != norm {
		if price, ok := e.table[stripped]; ok {
			return price, true
		}
		if canonical, ok := e.aliases[stripped]; ok {
			if price, ok := e.table[canonical]; ok {
				return price, true
			}
		}
	}
	return ModelPrice{}, false
}

// Calculate returns the token-based cost for an event using the ccusage formula.
// The 1h cache write falls back to input*2 when the table lacks an explicit
// above-1hr rate (matches ccusage and Anthropic's published multiplier).
func (e *Engine) Calculate(event usage.Event) (float64, bool) {
	price, ok := e.lookup(event.Model)
	if !ok {
		return 0, false
	}
	cache1h := price.CacheCreate1hPerToken
	if cache1h == 0 {
		cache1h = price.InputPerToken * 2
	}
	cost := float64(event.InputTokens)*price.InputPerToken +
		float64(event.OutputTokens)*price.OutputPerToken +
		float64(event.CacheCreate5mTokens)*price.CacheCreate5mPerToken +
		float64(event.CacheCreate1hTokens)*cache1h +
		float64(event.CacheReadTokens)*price.CacheReadPerToken +
		float64(event.ReasoningTokens)*price.OutputPerToken
	return cost, true
}

// Price applies the cost mode to a single event.
func (e *Engine) Price(event usage.Event, mode Mode) Result {
	withBilling := func(usd float64, source string) Result {
		marginal := usd
		if event.BillingType == usage.BillingSubscription {
			marginal = 0
		}
		return Result{USD: usd, MarginalUSD: marginal, Priced: true, Source: source}
	}
	switch mode {
	case ModeDisplay:
		if event.CostUSDProvided != nil {
			return withBilling(*event.CostUSDProvided, "provided")
		}
		return Result{Priced: false, Source: "unpriced"}
	case ModeCalculate:
		if usd, ok := e.Calculate(event); ok {
			return withBilling(usd, "calculated")
		}
		return Result{Priced: false, Source: "unpriced"}
	default: // ModeAuto
		if event.CostUSDProvided != nil {
			return withBilling(*event.CostUSDProvided, "provided")
		}
		if usd, ok := e.Calculate(event); ok {
			return withBilling(usd, "calculated")
		}
		return Result{Priced: false, Source: "unpriced"}
	}
}

// Has reports whether a model resolves to a known price (for "unpriced" alerts).
func (e *Engine) Has(model string) bool {
	_, ok := e.lookup(model)
	return ok
}

// parseLiteLLM decodes the model_prices_and_context_window.json shape, skipping
// the non-model "sample_spec" sentinel and entries without token pricing.
func parseLiteLLM(data []byte) (map[string]ModelPrice, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse litellm prices: %w", err)
	}
	table := make(map[string]ModelPrice, len(raw))
	for key, value := range raw {
		if key == "sample_spec" {
			continue
		}
		var price ModelPrice
		if err := json.Unmarshal(value, &price); err != nil {
			continue // schema drift on one entry must not break the table
		}
		if price.InputPerToken == 0 && price.OutputPerToken == 0 && price.CacheReadPerToken == 0 {
			continue
		}
		table[strings.ToLower(key)] = price
	}
	return table, nil
}

// defaultAliases maps common dated/proxy ids that don't strip cleanly to their
// canonical table key. Extend as agents surface new strings.
func defaultAliases() map[string]string {
	return map[string]string{
		"claude-3-5-sonnet": "claude-3-5-sonnet-20241022",
		"claude-3-5-haiku":  "claude-3-5-haiku-20241022",
		"claude-3-opus":     "claude-3-opus-20240229",
	}
}
