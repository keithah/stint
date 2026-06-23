import {
  cacheEfficiency,
  cacheSavingsEstimate,
  costPerProjectPerDay,
  latestDay,
  modelCostExtremes,
  reasoningShare,
  todayVsAverage
} from "./usage-insights";
import type { UsageDay, UsageSlice } from "./api";

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}

function assertClose(name: string, got: number, want: number, epsilon = 1e-9) {
  if (Math.abs(got - want) > epsilon) {
    throw new Error(`${name}: expected ~${want}, got ${got}`);
  }
}

// cacheEfficiency
const eff = cacheEfficiency({ cache_read_tokens: 900, input_tokens: 100 });
assertClose("cache hit ratio", eff.cacheHitRatio, 0.9);
assertEqual("cache efficiency has data", eff.hasData, true);

const noData = cacheEfficiency({ cache_read_tokens: 0, input_tokens: 0 });
assertClose("empty cache ratio is zero", noData.cacheHitRatio, 0);
assertEqual("empty cache reports no data", noData.hasData, false);

const negative = cacheEfficiency({ cache_read_tokens: -50, input_tokens: 100 });
assertClose("negative cache reads clamp to zero", negative.cacheHitRatio, 0);

// cacheSavingsEstimate
const savings = cacheSavingsEstimate({ cache_read_tokens: 1000, input_tokens: 0 });
assertClose("saved token equivalent", savings.savedTokenEquivalent, 900);
assertClose("savings ratio", savings.savingsRatio, 0.9);

const noSavings = cacheSavingsEstimate({ cache_read_tokens: 0, input_tokens: 0 });
assertClose("no billable tokens means zero savings ratio", noSavings.savingsRatio, 0);

// modelCostExtremes
const models: UsageSlice[] = [
  { name: "opus", cost_usd: 12, marginal_usd: 12, tokens: 100, event_count: 5 },
  { name: "haiku", cost_usd: 1, marginal_usd: 1, tokens: 200, event_count: 9 },
  { name: "free", cost_usd: 0, marginal_usd: 0, tokens: 5, event_count: 1 }
];
const extremes = modelCostExtremes(models);
assertEqual("most expensive model", extremes.mostExpensive?.name, "opus");
assertEqual("cheapest priced model", extremes.cheapest?.name, "free");

const emptyExtremes = modelCostExtremes([]);
assertEqual("empty byModel yields null most expensive", emptyExtremes.mostExpensive, null);
assertEqual("empty byModel yields null cheapest", emptyExtremes.cheapest, null);

// A $0-but-priced model (e.g. OpenRouter free tier) is the legitimate cheapest,
// not "absent" — an all-$0 set still returns extremes.
const allFree: UsageSlice[] = [
  { name: "free-a", cost_usd: 0, marginal_usd: 0, tokens: 10, event_count: 2 },
  { name: "free-b", cost_usd: 0, marginal_usd: 0, tokens: 5, event_count: 1 }
];
const freeExtremes = modelCostExtremes(allFree);
assertEqual("all-$0 set has a most expensive", freeExtremes.mostExpensive?.name, "free-a");
assertEqual("all-$0 set has a cheapest", freeExtremes.cheapest?.name, "free-a");

// reasoningShare
assertClose(
  "reasoning share",
  reasoningShare({ reasoning_tokens: 250, input_tokens: 250, output_tokens: 250, cache_create_tokens: 0, cache_read_tokens: 250 }),
  0.25
);
assertClose(
  "zero tokens means zero reasoning share",
  reasoningShare({ reasoning_tokens: 0, input_tokens: 0, output_tokens: 0, cache_create_tokens: 0, cache_read_tokens: 0 }),
  0
);

// todayVsAverage
const days: UsageDay[] = [
  { date: "2026-06-20", cost_usd: 2, marginal_usd: 2, tokens: 10 },
  { date: "2026-06-21", cost_usd: 4, marginal_usd: 4, tokens: 20 },
  { date: "2026-06-22", cost_usd: 12, marginal_usd: 12, tokens: 60 }
];
const burn = todayVsAverage(days);
if (!burn) {
  throw new Error("expected a burn rate result");
}
assertClose("today cost", burn.todayCost, 12);
assertClose("trailing average", burn.averageCost, 3);
assertClose("multiple", burn.multiple, 4);
assertEqual("anomaly detected", burn.isAnomaly, true);
assertEqual("prior day count", burn.priorDayCount, 2);

assertEqual("empty series has no burn rate", todayVsAverage([]), null);

const calmDays: UsageDay[] = [
  { date: "2026-06-21", cost_usd: 10, marginal_usd: 10, tokens: 20 },
  { date: "2026-06-22", cost_usd: 10, marginal_usd: 10, tokens: 20 }
];
assertEqual("steady spend is not an anomaly", todayVsAverage(calmDays)?.isAnomaly, false);

// latestDay
assertEqual("latest day by date", latestDay(days)?.date, "2026-06-22");
assertEqual("latest day of empty series", latestDay([]), null);

// out-of-order input still resolves latest
const shuffled: UsageDay[] = [days[2], days[0], days[1]];
assertEqual("latest day from unsorted series", latestDay(shuffled)?.date, "2026-06-22");

// costPerProjectPerDay
const projects: UsageSlice[] = [
  { name: "alpha", cost_usd: 30, marginal_usd: 30, tokens: 100, event_count: 10 },
  { name: "beta", cost_usd: 6, marginal_usd: 6, tokens: 50, event_count: 4 }
];
const perDay = costPerProjectPerDay(projects, 3);
assertEqual("top project per day", perDay[0]?.name, "alpha");
assertClose("alpha per-day cost", perDay[0]?.perDay ?? -1, 10);
assertClose("zero days clamps to one", costPerProjectPerDay(projects, 0)[0]?.perDay ?? -1, 30);

console.log("usage-insights.test.ts passed");
