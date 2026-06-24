// Package pricing turns canonical usage events into USD using a LiteLLM-sourced
// price table. It is decoupled from ingestion and storage: it knows models and
// token types, nothing about agents. Prices come from a bundled offline
// snapshot (refreshable) plus optional user overrides; business logic never
// hardcodes a price.
package pricing

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/keithah/stint/internal/usage"
)

// Upstream price sources, refreshed by the weekly pricing job. LiteLLM is the
// primary cache-accurate table; OpenRouter is the broad fallback. Both are
// public (no key required).
const (
	LiteLLMURL    = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"
	OpenRouterURL = "https://openrouter.ai/api/v1/models"
)

//go:embed data/litellm_prices.json
var bundledPrices []byte

//go:embed data/openrouter_prices.json
var bundledOpenRouterPrices []byte

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
	// UncachedUSD is what this usage would cost with NO prompt caching: every
	// input-side token (fresh input + cache reads + cache writes) priced at the
	// full input rate. UncachedUSD - USD is the dollars saved by caching. For
	// provider-priced events (no token-level breakdown) it equals USD.
	UncachedUSD float64
	// Priced is false when the model is unknown and no provider cost exists.
	Priced bool
	// ModelResolved is true iff the model resolves in the table/overrides/fallback,
	// independent of whether USD came from a provider-reported cost. A model with a
	// provider cost but no table price is Priced but NOT ModelResolved, so callers
	// can still flag it as "unpriced" (no table coverage) without a second lookup.
	ModelResolved bool
	// Source is "provided", "calculated", or "unpriced".
	Source string
}

// engineData is the immutable lookup state swapped atomically on reload, so a
// weekly price refresh can replace the tables under live readers without locks.
type engineData struct {
	table    map[string]ModelPrice
	fallback map[string]ModelPrice
	aliases  map[string]string
}

// Engine prices events against a model table with optional overrides. Lookups
// resolve in priority order: user overrides → LiteLLM table (cache-accurate,
// ccusage parity) → OpenRouter fallback (broad coverage of proxy/free/new
// models) → unpriced. The base table lives behind an atomic pointer shared by
// all WithOverrides clones, so Reload propagates to every in-flight engine.
type Engine struct {
	data      *atomic.Pointer[engineData]
	overrides map[string]ModelPrice
}

// NewFromBundled builds an engine from the embedded offline snapshots: LiteLLM
// as the primary table and OpenRouter as the fallback layer.
func NewFromBundled() (*Engine, error) {
	table, err := parseLiteLLM(bundledPrices)
	if err != nil {
		return nil, err
	}
	fallback, err := parseOpenRouter(bundledOpenRouterPrices)
	if err != nil {
		fallback = map[string]ModelPrice{} // fallback is best-effort; never fatal
	}
	data := &atomic.Pointer[engineData]{}
	data.Store(&engineData{table: table, fallback: fallback, aliases: defaultAliases()})
	return &Engine{data: data, overrides: map[string]ModelPrice{}}, nil
}

// Reload atomically swaps the price tables from fresh upstream snapshots. A nil
// slice leaves that layer unchanged; an OpenRouter parse failure is non-fatal
// (keeps the current fallback). Safe to call concurrently with pricing.
func (e *Engine) Reload(litellm, openrouter []byte) error {
	cur := e.data.Load()
	table := cur.table
	if litellm != nil {
		t, err := parseLiteLLM(litellm)
		if err != nil {
			return err
		}
		table = t
	}
	fallback := cur.fallback
	if openrouter != nil {
		if f, err := parseOpenRouter(openrouter); err == nil {
			fallback = f
		}
	}
	e.data.Store(&engineData{table: table, fallback: fallback, aliases: cur.aliases})
	return nil
}

// ModelPriceEntry is one row of the resolved price table, tagged with its
// source layer, for the settings model-rate viewer.
type ModelPriceEntry struct {
	Model  string
	Source string // "litellm" | "openrouter"
	Price  ModelPrice
}

// Entries returns every known model price (LiteLLM table first, then OpenRouter
// fallback entries the table does not already cover) for display. Order is
// unspecified; callers sort/filter.
func (e *Engine) Entries() []ModelPriceEntry {
	d := e.data.Load()
	out := make([]ModelPriceEntry, 0, len(d.table)+len(d.fallback))
	for model, price := range d.table {
		out = append(out, ModelPriceEntry{Model: model, Source: "litellm", Price: price})
	}
	for model, price := range d.fallback {
		if _, ok := d.table[model]; ok {
			continue
		}
		out = append(out, ModelPriceEntry{Model: model, Source: "openrouter", Price: price})
	}
	return out
}

// SetOverrides installs user custom pricing on this engine for private/proxied/
// unknown models. Override keys are matched after normalization and win over the
// base table. Mutates only this engine's overrides, not the shared base table.
func (e *Engine) SetOverrides(overrides map[string]ModelPrice) {
	e.overrides = map[string]ModelPrice{}
	for model, price := range overrides {
		e.overrides[Normalize(model)] = price
	}
}

// WithOverrides returns a shallow copy of the engine that shares the atomic base
// table but carries its own overrides map. Use this for per-request custom
// pricing so the shared engine is never mutated across requests; the clone still
// observes Reload because it shares the same atomic pointer.
func (e *Engine) WithOverrides(overrides map[string]ModelPrice) *Engine {
	clone := &Engine{data: e.data, overrides: map[string]ModelPrice{}}
	for model, price := range overrides {
		clone.overrides[Normalize(model)] = price
	}
	return clone
}

// FetchLiteLLM downloads the current LiteLLM price table. Caller validates by
// parsing before persisting.
func FetchLiteLLM(ctx context.Context, client *http.Client) ([]byte, error) {
	return fetch(ctx, client, LiteLLMURL)
}

// FetchOpenRouter downloads the current OpenRouter model catalog.
func FetchOpenRouter(ctx context.Context, client *http.Client) ([]byte, error) {
	return fetch(ctx, client, OpenRouterURL)
}

func fetch(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

// CountLiteLLM and CountOpenRouter validate a snapshot by parsing it and return
// the number of priced models (0, err on invalid JSON).
func CountLiteLLM(data []byte) (int, error) {
	t, err := parseLiteLLM(data)
	if err != nil {
		return 0, err
	}
	return len(t), nil
}

func CountOpenRouter(data []byte) (int, error) {
	f, err := parseOpenRouter(data)
	if err != nil {
		return 0, err
	}
	return len(f), nil
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
	d := e.data.Load()
	norm := Normalize(model)
	if price, ok := e.overrides[norm]; ok {
		return price, true
	}
	// Raw (un-normalized) probe BEFORE the normalized one is intentional: LiteLLM
	// ships region-prefixed keys (e.g. "us.anthropic.claude-...") with their own
	// region-specific pricing. Probing raw first preserves that price before
	// Normalize strips the prefix and collapses it to the base model. Do not remove.
	if price, ok := d.table[strings.ToLower(strings.TrimSpace(model))]; ok {
		return price, true
	}
	if price, ok := d.table[norm]; ok {
		return price, true
	}
	if canonical, ok := d.aliases[norm]; ok {
		if price, ok := d.table[canonical]; ok {
			return price, true
		}
	}
	if stripped := dateSuffix.ReplaceAllString(norm, ""); stripped != norm {
		if price, ok := d.table[stripped]; ok {
			return price, true
		}
		if canonical, ok := d.aliases[stripped]; ok {
			if price, ok := d.table[canonical]; ok {
				return price, true
			}
		}
	}
	// OpenRouter fallback: broad coverage (proxy/free/new models) that LiteLLM
	// may lack. Consulted only after LiteLLM so cache-accurate prices win.
	if price, ok := d.fallback[norm]; ok {
		return price, true
	}
	return ModelPrice{}, false
}

// Calculate returns the token-based cost for an event using the ccusage formula.
// The 1h cache write falls back to input*2 when the table lacks an explicit
// above-1hr rate (matches ccusage and Anthropic's published multiplier).
func (e *Engine) Calculate(event usage.Event) (float64, bool) {
	cost, _, ok := e.calculate(event)
	return cost, ok
}

// calculate returns (cost, uncached, ok). uncached prices every input-side token
// (fresh input + cache reads + cache writes) at the full input rate — what the
// same usage would cost with no prompt caching. uncached - cost is the savings
// caching delivered, and the honest counterpoint to cost models that ignore it.
func (e *Engine) calculate(event usage.Event) (float64, float64, bool) {
	price, ok := e.lookup(event.Model)
	if !ok {
		return 0, 0, false
	}
	cache1h := price.CacheCreate1hPerToken
	// Only infer the 1h-cache rate (Anthropic's input*2 convention) for models
	// that actually have a 5m cache-create rate. A model with no cache-create
	// pricing at all (e.g. an OpenRouter fallback entry with cache-write 0) must
	// not be charged a fabricated input*2 for 1h-cache tokens.
	if cache1h == 0 && price.CacheCreate5mPerToken > 0 {
		cache1h = price.InputPerToken * 2
	}
	cost := float64(event.InputTokens)*price.InputPerToken +
		float64(event.OutputTokens)*price.OutputPerToken +
		float64(event.CacheCreate5mTokens)*price.CacheCreate5mPerToken +
		float64(event.CacheCreate1hTokens)*cache1h +
		float64(event.CacheReadTokens)*price.CacheReadPerToken +
		float64(event.ReasoningTokens)*price.OutputPerToken
	// No-cache counterfactual: cache reads and cache writes collapse to plain
	// fresh-input tokens at the input rate (no read discount, no write premium).
	uncachedInput := event.InputTokens + event.CacheCreate5mTokens +
		event.CacheCreate1hTokens + event.CacheReadTokens
	uncached := float64(uncachedInput)*price.InputPerToken +
		float64(event.OutputTokens)*price.OutputPerToken +
		float64(event.ReasoningTokens)*price.OutputPerToken
	return cost, uncached, true
}

// Price applies the cost mode to a single event. ModelResolved reports whether
// the model exists in the price table/overrides/fallback so callers can detect
// "unpriced" models without a separate Has lookup on the hot path.
func (e *Engine) Price(event usage.Event, mode Mode) Result {
	withBilling := func(usd, uncached float64, source string, resolved bool) Result {
		marginal := usd
		if event.BillingType == usage.BillingSubscription {
			marginal = 0
		}
		return Result{USD: usd, MarginalUSD: marginal, UncachedUSD: uncached, Priced: true, ModelResolved: resolved, Source: source}
	}
	switch mode {
	case ModeDisplay:
		// Display never calls Calculate, so resolve the model with a single lookup
		// to populate ModelResolved (still one lookup per event, vs. the old
		// Price+Has pair).
		_, resolved := e.lookup(event.Model)
		if event.CostUSDProvided != nil {
			return withBilling(*event.CostUSDProvided, *event.CostUSDProvided, "provided", resolved)
		}
		return Result{Priced: false, ModelResolved: resolved, Source: "unpriced"}
	case ModeCalculate:
		// Calculate resolves the model itself; a successful calc means resolved.
		if usd, uncached, ok := e.calculate(event); ok {
			return withBilling(usd, uncached, "calculated", true)
		}
		return Result{Priced: false, ModelResolved: false, Source: "unpriced"}
	default: // ModeAuto
		if event.CostUSDProvided != nil {
			// Provider cost is used, but report whether the model also resolves in
			// the table so a provider-priced-but-table-unknown model still counts
			// as "unpriced". This is the only extra lookup, replacing the old Has.
			_, resolved := e.lookup(event.Model)
			return withBilling(*event.CostUSDProvided, *event.CostUSDProvided, "provided", resolved)
		}
		if usd, uncached, ok := e.calculate(event); ok {
			return withBilling(usd, uncached, "calculated", true)
		}
		return Result{Priced: false, ModelResolved: false, Source: "unpriced"}
	}
}

// Has reports whether a model resolves to a known price (for "unpriced" alerts).
func (e *Engine) Has(model string) bool {
	_, ok := e.lookup(model)
	return ok
}

// parseOpenRouter decodes OpenRouter's GET /api/v1/models response into a price
// table keyed by normalized model id. Prices are strings in USD per token.
// Free models (price "0") are kept so they price at $0 rather than reading as
// unpriced. OpenRouter has no 5m/1h cache split, so a single cache-write rate
// fills both — acceptable for a fallback layer.
func parseOpenRouter(data []byte) (map[string]ModelPrice, error) {
	var doc struct {
		Data []struct {
			ID      string `json:"id"`
			Pricing struct {
				Prompt          string `json:"prompt"`
				Completion      string `json:"completion"`
				InputCacheRead  string `json:"input_cache_read"`
				InputCacheWrite string `json:"input_cache_write"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse openrouter prices: %w", err)
	}
	atof := func(s string) float64 {
		v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return 0
		}
		return v
	}
	table := make(map[string]ModelPrice, len(doc.Data))
	for _, m := range doc.Data {
		if m.ID == "" || (m.Pricing.Prompt == "" && m.Pricing.Completion == "") {
			continue
		}
		cacheWrite := atof(m.Pricing.InputCacheWrite)
		table[Normalize(m.ID)] = ModelPrice{
			InputPerToken:         atof(m.Pricing.Prompt),
			OutputPerToken:        atof(m.Pricing.Completion),
			CacheReadPerToken:     atof(m.Pricing.InputCacheRead),
			CacheCreate5mPerToken: cacheWrite,
			CacheCreate1hPerToken: cacheWrite,
			Provider:              "openrouter",
		}
	}
	return table, nil
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
