import type { UsageCurrentBlock, UsageSlice, UsageTotal } from "@/lib/usage-api";

// A subscription-billed slice has an equivalent-API cost (cost_usd) but little or
// no out-of-pocket marginal cost. An API-billed slice pays roughly its full
// equivalent cost out of pocket (cost ≈ marginal). We classify by the share of
// equivalent cost that is actually billed marginally.
export type EffectiveBilling = "subscription" | "api" | "mixed" | "free";

// Classify a row's effective billing from its equivalent (cost_usd) vs
// out-of-pocket (marginal_usd) cost.
//   - free: no equivalent cost recorded.
//   - subscription: marginal is a small fraction of equivalent (flat-rate plan).
//   - api: marginal ≈ equivalent (metered, paid per call).
//   - mixed: somewhere in between (e.g. a plan with overage).
export function effectiveBilling(
  row: Pick<UsageSlice, "cost_usd" | "marginal_usd">,
  subscriptionMaxRatio = 0.15,
  apiMinRatio = 0.85
): EffectiveBilling {
  const cost = Math.max(0, row.cost_usd);
  const marginal = Math.max(0, row.marginal_usd);
  if (cost <= 0) {
    return "free";
  }
  const ratio = marginal / cost;
  if (ratio <= subscriptionMaxRatio) {
    return "subscription";
  }
  if (ratio >= apiMinRatio) {
    return "api";
  }
  return "mixed";
}

export type BillingBadge = {
  kind: EffectiveBilling;
  label: string;
  // Tailwind utility classes for the badge chip.
  className: string;
};

const badgeStyles: Record<EffectiveBilling, { label: string; className: string }> = {
  subscription: { label: "Subscription", className: "border-moss/40 bg-moss/10 text-moss" },
  api: { label: "API", className: "border-accent/40 bg-accent/10 text-accent" },
  mixed: { label: "Mixed", className: "border-ember/40 bg-ember/10 text-ember" },
  free: { label: "Free", className: "border-line bg-white/5 text-zinc-400" }
};

export function billingBadge(row: Pick<UsageSlice, "cost_usd" | "marginal_usd">): BillingBadge {
  const kind = effectiveBilling(row);
  return { kind, ...badgeStyles[kind] };
}

// Whole-account split: is there a meaningful gap between equivalent and
// out-of-pocket cost? Used to decide whether to surface the dual-cost framing.
export function hasSubscriptionSavings(total: Pick<UsageTotal, "cost_usd" | "marginal_usd">, minGapUsd = 0.005): boolean {
  return total.cost_usd - total.marginal_usd > minGapUsd;
}

// Equivalent cost the subscription covered (the spread between the API-equivalent
// value and what was actually paid out of pocket), clamped at zero.
export function subscriptionCovered(total: Pick<UsageTotal, "cost_usd" | "marginal_usd">): number {
  return Math.max(0, total.cost_usd - total.marginal_usd);
}

export type BlockProgress = {
  // Fraction of the 5-hour block elapsed, clamped to [0, 1].
  fraction: number;
  // Percentage 0–100 for direct width styling.
  percent: number;
  elapsedMinutes: number;
  remainingMinutes: number;
  totalMinutes: number;
};

// 5-hour usage blocks are 300 minutes. Compute elapsed/remaining progress from
// the reported elapsed_minutes, clamped so a stale/over-running block never
// produces a >100% bar or negative remainder.
export function blockProgress(block: Pick<UsageCurrentBlock, "elapsed_minutes">, totalMinutes = 300): BlockProgress {
  const total = Math.max(1, totalMinutes);
  const elapsed = Math.min(total, Math.max(0, block.elapsed_minutes));
  const fraction = elapsed / total;
  return {
    fraction,
    percent: fraction * 100,
    elapsedMinutes: Math.round(elapsed),
    remainingMinutes: Math.round(total - elapsed),
    totalMinutes: total
  };
}
