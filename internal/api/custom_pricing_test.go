package api

import (
	"testing"
	"time"

	"github.com/keithah/stint/internal/pricing"
	"github.com/keithah/stint/internal/usage"
	"github.com/keithah/stint/internal/usagestats"
)

// TestCustomPricingOverridesPriceUnpricedModel proves the per-request override
// path: a private model the bundled table does not know becomes priced once the
// user's custom per-million prices are converted to per-token and installed via
// WithOverrides, and the shared base engine is never mutated.
func TestCustomPricingOverridesPriceUnpricedModel(t *testing.T) {
	base, err := pricing.NewFromBundled()
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	const model = "opencode/big-pickle"
	if base.Has(model) {
		t.Fatalf("expected %q to be unpriced in the bundled table", model)
	}

	// User enters $3/1M input, $15/1M output. Convert per-million -> per-token.
	overrides := map[string]pricing.ModelPrice{
		model: {
			InputPerToken:  3.0 / 1e6,
			OutputPerToken: 15.0 / 1e6,
		},
	}
	engine := base.WithOverrides(overrides)

	if !engine.Has(model) {
		t.Fatalf("expected override engine to price %q", model)
	}
	// Shared base engine must remain unmutated.
	if base.Has(model) {
		t.Fatalf("base engine was mutated by WithOverrides")
	}

	ts := time.Now().UTC().Format(time.RFC3339)
	events := []usage.Event{{
		Agent:        "opencode",
		Model:        model,
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
		Timestamp:    ts,
		BillingType:  usage.BillingAPI,
	}}

	summary := usagestats.Summarize(events, engine, pricing.ModeCalculate, time.UTC, nil)
	cost := summary.Total.CostUSD
	// 1M input * $3/1M + 1M output * $15/1M = $18.
	if cost < 17.999 || cost > 18.001 {
		t.Fatalf("expected cost ~18.0, got %f", cost)
	}
	if len(summary.UnpricedModels) != 0 {
		t.Fatalf("expected no unpriced models with override, got %v", summary.UnpricedModels)
	}

	// Without the override the same model is unpriced.
	bare := usagestats.Summarize(events, base, pricing.ModeCalculate, time.UTC, nil)
	if len(bare.UnpricedModels) != 1 || bare.UnpricedModels[0] != model {
		t.Fatalf("expected %q unpriced without override, got %v", model, bare.UnpricedModels)
	}
}
