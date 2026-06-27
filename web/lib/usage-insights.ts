import type { UsageDay, UsageSlice, UsageTotal } from "@/lib/usage-api";

export type CacheEfficiency = {
  cacheHitRatio: number;
  cacheReadTokens: number;
  freshInputTokens: number;
  hasData: boolean;
};

// Cache-hit ratio = cache_read_tokens / (cache_read_tokens + input_tokens).
// Fresh input tokens are billed at full price; cache reads are cheap, so a high
// ratio means most context is being served from cache.
export function cacheEfficiency(total: Pick<UsageTotal, "cache_read_tokens" | "input_tokens">): CacheEfficiency {
  const cacheReadTokens = Math.max(0, total.cache_read_tokens);
  const freshInputTokens = Math.max(0, total.input_tokens);
  const denom = cacheReadTokens + freshInputTokens;
  return {
    cacheHitRatio: denom > 0 ? cacheReadTokens / denom : 0,
    cacheReadTokens,
    freshInputTokens,
    hasData: denom > 0
  };
}

// Real dollar savings from prompt caching, using the server-computed
// uncached-equivalent cost (every input-side token at the full input rate).
// savedUSD = uncached - actual; savingsRatio is that saving as a fraction of the
// uncached cost. This is the honest counter to trackers that price cache reads at
// full freight and so over-report cost several-fold.
export function cachingDollarSavings(
  total: Pick<UsageTotal, "cost_usd" | "uncached_cost_usd">
): { savedUSD: number; uncachedUSD: number; savingsRatio: number; hasData: boolean } {
  const uncachedUSD = Math.max(0, total.uncached_cost_usd ?? 0);
  const savedUSD = Math.max(0, uncachedUSD - Math.max(0, total.cost_usd));
  return {
    savedUSD,
    uncachedUSD,
    savingsRatio: uncachedUSD > 0 ? savedUSD / uncachedUSD : 0,
    hasData: uncachedUSD > 0
  };
}

// Most- and least-expensive priced model by cost_usd. A $0-but-priced model
// (OpenRouter free tier, a $0 custom price) is a legitimate "cheapest", so any
// model with a non-negative cost counts — only truly absent data yields null.
export function modelCostExtremes(byModel: UsageSlice[]): { mostExpensive: UsageSlice | null; cheapest: UsageSlice | null } {
  const priced = byModel.filter((row) => row.cost_usd >= 0);
  if (priced.length === 0) {
    return { mostExpensive: null, cheapest: null };
  }
  let mostExpensive = priced[0];
  let cheapest = priced[0];
  for (const row of priced) {
    if (row.cost_usd > mostExpensive.cost_usd) {
      mostExpensive = row;
    }
    if (row.cost_usd < cheapest.cost_usd) {
      cheapest = row;
    }
  }
  return { mostExpensive, cheapest };
}

// Reasoning-token share of total tokens.
export function reasoningShare(total: Pick<UsageTotal, "reasoning_tokens" | "input_tokens" | "output_tokens" | "cache_create_tokens" | "cache_read_tokens">): number {
  const all =
    Math.max(0, total.input_tokens) +
    Math.max(0, total.output_tokens) +
    Math.max(0, total.cache_create_tokens) +
    Math.max(0, total.cache_read_tokens) +
    Math.max(0, total.reasoning_tokens);
  return all > 0 ? Math.max(0, total.reasoning_tokens) / all : 0;
}

export type BurnRate = {
  todayCost: number;
  averageCost: number;
  multiple: number;
  priorDayCount: number;
  isAnomaly: boolean;
};

// Compares the latest day's cost against the average of the preceding days in
// the series. `multiple` is "today is Nx your average". Anomaly when today is
// >= 1.5x the trailing average (and there is a baseline to compare against).
export function todayVsAverage(byDay: UsageDay[], anomalyThreshold = 1.5): BurnRate | null {
  const today = latestDay(byDay);
  if (!today) {
    return null;
  }
  // The latest day is "today"; every other day is the trailing baseline. We
  // never need the full sorted order, just the max-by-date and the prior total.
  let priorTotal = 0;
  let priorCount = 0;
  for (const day of byDay) {
    if (day === today) {
      continue;
    }
    priorTotal += Math.max(0, day.cost_usd);
    priorCount++;
  }
  const averageCost = priorCount > 0 ? priorTotal / priorCount : 0;
  const multiple = averageCost > 0 ? today.cost_usd / averageCost : 0;
  return {
    todayCost: Math.max(0, today.cost_usd),
    averageCost,
    multiple,
    priorDayCount: priorCount,
    isAnomaly: priorCount > 0 && averageCost > 0 && multiple >= anomalyThreshold
  };
}

// Latest day in a by_day series (by date), or null when empty. Single O(n)
// pass over the date strings — no array copy or full sort.
export function latestDay(byDay: UsageDay[]): UsageDay | null {
  if (byDay.length === 0) {
    return null;
  }
  return byDay.reduce((latest, day) => (day.date.localeCompare(latest.date) > 0 ? day : latest));
}

// Highest-cost project-day rate: total project cost divided by the number of
// distinct days with usage, giving an approximate "$ per project per day".
export function costPerProjectPerDay(byProject: UsageSlice[], dayCount: number): Array<{ name: string; perDay: number }> {
  const days = Math.max(1, dayCount);
  return byProject
    .map((row) => ({ name: row.name, perDay: row.cost_usd / days }))
    .sort((a, b) => b.perDay - a.perDay);
}
