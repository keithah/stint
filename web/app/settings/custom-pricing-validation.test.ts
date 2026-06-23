import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { canSaveCustomPricing, customPricingError } from "./custom-pricing-validation";

// Pure-logic: a complete, non-negative draft is valid.
assert.equal(
  customPricingError({
    model: "opencode/big-pickle",
    inputPerMillion: 1.5,
    outputPerMillion: 6,
    cacheWritePerMillion: 1.875,
    cacheReadPerMillion: 0.15
  }),
  null
);
assert.equal(
  canSaveCustomPricing({
    model: "opencode/big-pickle",
    inputPerMillion: 0,
    outputPerMillion: 0,
    cacheWritePerMillion: 0,
    cacheReadPerMillion: 0
  }),
  true
);

// Empty model is rejected (matches server validation).
assert.equal(
  customPricingError({
    model: "   ",
    inputPerMillion: 1,
    outputPerMillion: 1,
    cacheWritePerMillion: 0,
    cacheReadPerMillion: 0
  }),
  "Model is required"
);

// Negative prices are rejected.
assert.equal(
  customPricingError({
    model: "m",
    inputPerMillion: -1,
    outputPerMillion: 1,
    cacheWritePerMillion: 0,
    cacheReadPerMillion: 0
  }),
  "Prices must be non-negative"
);

// Non-finite prices (e.g. NaN from an empty number input) are rejected.
assert.equal(
  customPricingError({
    model: "m",
    inputPerMillion: Number.NaN,
    outputPerMillion: 1,
    cacheWritePerMillion: 0,
    cacheReadPerMillion: 0
  }),
  "Prices must be numbers"
);

// Wiring: the settings page imports and gates on the helper, and the test is
// registered in the package test chain.
const source = readFileSync("components/settings/custom-pricing-card.tsx", "utf8");
const packageJSON = readFileSync("package.json", "utf8");
assert.match(source, /customPricingError\(\{/);
assert.match(source, /disabled=\{savePricing\.isPending \|\| !canSaveCustomPricing\}/);
assert.match(source, /upsertCustomPricing\(\{/);
assert.match(source, /removePricing\.mutate\(price\.model\)/);
assert.match(packageJSON, /app\/settings\/custom-pricing-validation\.test\.ts/);

console.log("custom-pricing-validation.test.ts passed");
