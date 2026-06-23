package pricing

import (
	"math"
	"testing"

	"github.com/keithah/stint/internal/usage"
)

func engine(t *testing.T) *Engine {
	t.Helper()
	e, err := NewFromBundled()
	if err != nil {
		t.Fatalf("load bundled prices: %v", err)
	}
	return e
}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// Golden values are derived from the pinned LiteLLM snapshot for
// claude-sonnet-4-5: input 3e-6, output 1.5e-5, cacheCreate5m 3.75e-6,
// cacheRead 3e-7 per token. A snapshot change that alters these will fail here.
func TestCalculateGoldenSonnet(t *testing.T) {
	e := engine(t)
	event := usage.Event{Model: "claude-sonnet-4-5", InputTokens: 1000, OutputTokens: 500, CacheCreate5mTokens: 2000, CacheReadTokens: 10000}
	got, ok := e.Calculate(event)
	if !ok {
		t.Fatal("expected sonnet to be priced")
	}
	want := 1000*3e-6 + 500*1.5e-5 + 2000*3.75e-6 + 10000*3e-7 // 0.021
	if !approx(got, want) {
		t.Fatalf("expected %.9f, got %.9f", want, got)
	}
}

func TestNormalizeAndRegionalDatedLookup(t *testing.T) {
	e := engine(t)
	if Normalize("us.anthropic.claude-sonnet-4-5-20250929") != "claude-sonnet-4-5-20250929" {
		t.Fatalf("unexpected normalize: %q", Normalize("us.anthropic.claude-sonnet-4-5-20250929"))
	}
	if !e.Has("us.anthropic.claude-sonnet-4-5-20250929") {
		t.Fatal("regional + dated claude string should resolve via prefix strip + date strip")
	}
	if !e.Has("openrouter/anthropic/claude-opus-4-1") {
		t.Fatal("proxy-prefixed model should resolve")
	}
}

func TestCostModes(t *testing.T) {
	e := engine(t)
	provided := 0.5
	event := usage.Event{Model: "claude-sonnet-4-5", InputTokens: 1000, CostUSDProvided: &provided}

	if r := e.Price(event, ModeDisplay); r.Source != "provided" || !approx(r.USD, 0.5) {
		t.Fatalf("display should show provided cost, got %+v", r)
	}
	if r := e.Price(event, ModeAuto); r.Source != "provided" || !approx(r.USD, 0.5) {
		t.Fatalf("auto should prefer provided cost, got %+v", r)
	}
	if r := e.Price(event, ModeCalculate); r.Source != "calculated" || !approx(r.USD, 1000*3e-6) {
		t.Fatalf("calculate should recompute from tokens, got %+v", r)
	}
	// display mode with no provided cost = unpriced (no estimation)
	noCost := usage.Event{Model: "claude-sonnet-4-5", InputTokens: 1000}
	if r := e.Price(noCost, ModeDisplay); r.Priced {
		t.Fatalf("display with no provided cost must be unpriced, got %+v", r)
	}
}

func TestSubscriptionMarginalIsZero(t *testing.T) {
	e := engine(t)
	event := usage.Event{Model: "claude-sonnet-4-5", InputTokens: 1000, BillingType: usage.BillingSubscription}
	r := e.Price(event, ModeCalculate)
	if !approx(r.MarginalUSD, 0) {
		t.Fatalf("subscription marginal cost must be 0, got %f", r.MarginalUSD)
	}
	if !approx(r.USD, 1000*3e-6) {
		t.Fatalf("subscription equivalent-API cost should still compute, got %f", r.USD)
	}
}

func TestOpenRouterFallbackPricesModelsLiteLLMLacks(t *testing.T) {
	e := engine(t)
	// A model only present in the OpenRouter catalog (a :free model) should
	// resolve via the fallback layer rather than reading as unpriced.
	if !e.Has("cohere/north-mini-code:free") {
		t.Skip("bundled OpenRouter snapshot does not contain the sample free model; snapshot may have rotated")
	}
	r := e.Price(usage.Event{Model: "cohere/north-mini-code:free", InputTokens: 1000, OutputTokens: 1000}, ModeCalculate)
	if !r.Priced {
		t.Fatal("a free OpenRouter model must resolve (priced, at $0) not read as unpriced")
	}
}

func TestLiteLLMWinsOverOpenRouterForCacheAccuracy(t *testing.T) {
	e := engine(t)
	// claude-sonnet-4-5 is in both tables; the LiteLLM entry (with the explicit
	// 1h cache rate) must take precedence over the OpenRouter fallback.
	event := usage.Event{Model: "claude-sonnet-4-5", CacheCreate1hTokens: 1000}
	got, ok := e.Calculate(event)
	if !ok {
		t.Fatal("expected sonnet priced")
	}
	// LiteLLM 1h rate is 6e-6 (input*2); a wrong fallback would use the 5m write rate.
	if !approx(got, 1000*6e-6) {
		t.Fatalf("expected LiteLLM 1h cache rate, got %.9f", got)
	}
}

func TestUnknownModelUnpricedUnlessOverridden(t *testing.T) {
	e := engine(t)
	event := usage.Event{Model: "my-private-proxy-model", InputTokens: 1000, OutputTokens: 1000}
	if e.Has(event.Model) {
		t.Fatal("unknown model should not resolve before override")
	}
	if r := e.Price(event, ModeCalculate); r.Priced {
		t.Fatalf("unknown model must be unpriced (not $0), got %+v", r)
	}
	e.SetOverrides(map[string]ModelPrice{"my-private-proxy-model": {InputPerToken: 1e-6, OutputPerToken: 2e-6}})
	r := e.Price(event, ModeCalculate)
	if !r.Priced || !approx(r.USD, 1000*1e-6+1000*2e-6) {
		t.Fatalf("override should price the model, got %+v", r)
	}
}
