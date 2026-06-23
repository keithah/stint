// Pure validation helpers for the custom AI pricing form. Kept dependency-free
// so it can be unit-tested directly with sucrase-node (see custom-pricing-validation.test.ts).

export type CustomPricingDraft = {
  model: string;
  inputPerMillion: number;
  outputPerMillion: number;
  cacheWritePerMillion: number;
  cacheReadPerMillion: number;
};

// customPricingError returns a human-readable error string for an invalid draft,
// or null when the draft is valid and safe to submit. Mirrors the server-side
// validation: non-empty model, finite non-negative per-million prices.
export function customPricingError(draft: CustomPricingDraft): string | null {
  if (draft.model.trim().length === 0) {
    return "Model is required";
  }
  const prices = [
    draft.inputPerMillion,
    draft.outputPerMillion,
    draft.cacheWritePerMillion,
    draft.cacheReadPerMillion
  ];
  for (const price of prices) {
    if (!Number.isFinite(price)) {
      return "Prices must be numbers";
    }
    if (price < 0) {
      return "Prices must be non-negative";
    }
  }
  return null;
}

// canSaveCustomPricing is the boolean convenience wrapper used to gate the form.
export function canSaveCustomPricing(draft: CustomPricingDraft): boolean {
  return customPricingError(draft) === null;
}
