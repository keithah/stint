// AI usage-cost client surface: canonical usage events, priced summaries, 5-hour
// blocks, and custom pricing overrides. Kept separate from the general api.ts
// client (it shares the request() helper) so the AI-cost domain has one cohesive
// module, mirroring the server-side internal/usagestats split.
import { request, type StatsRange } from "./api";

export type UsageCostMode = "auto" | "calculate" | "display";

export type CustomPricing = {
  model: string;
  input_per_million_usd: number;
  output_per_million_usd: number;
  cache_write_per_million_usd: number;
  cache_read_per_million_usd: number;
  created_at?: string;
  modified_at?: string;
};

export type UsageTotal = {
  cost_usd: number;
  marginal_usd: number;
  event_count: number;
  input_tokens: number;
  output_tokens: number;
  cache_create_tokens: number;
  cache_read_tokens: number;
  reasoning_tokens: number;
};

export type UsageSlice = {
  name: string;
  cost_usd: number;
  marginal_usd: number;
  tokens: number;
  event_count: number;
};

export type UsageDay = {
  date: string;
  cost_usd: number;
  marginal_usd: number;
  tokens: number;
};

export type UsageSummary = {
  range: string;
  cost_mode: UsageCostMode;
  total: UsageTotal;
  by_agent: UsageSlice[];
  by_model: UsageSlice[];
  by_project: UsageSlice[];
  by_day: UsageDay[];
  unpriced_models: string[];
};

export type UsageBlock = {
  start: string;
  end: string;
  is_active: boolean;
  cost_usd: number;
  tokens: number;
  event_count: number;
};

export type UsageCurrentBlock = {
  start: string;
  end: string;
  is_active: boolean;
  elapsed_minutes: number;
  cost_usd: number;
  tokens: number;
  burn_rate_cost_per_hour: number;
  burn_rate_tokens_per_min: number;
  projected_block_cost_usd: number;
  projected_day_cost_usd: number;
  projected_month_cost_usd: number;
};

export type UsageBlocks = {
  cost_mode: UsageCostMode;
  blocks: UsageBlock[];
  current: UsageCurrentBlock | null;
};

export type UsageBillingType = "api" | "subscription";

// Matches the raw canonical usage.Event JSON returned by listUsageEvents
// (internal/usage/event.go). These are stored events, not priced summaries:
// cost is provider-reported (optional) and cache-create keeps 5m/1h granularity.
export type UsageEvent = {
  event_id: string;
  agent: string;
  session_id: string;
  project?: string;
  model: string;
  input_tokens: number;
  output_tokens: number;
  cache_create_5m_tokens: number;
  cache_create_1h_tokens: number;
  cache_read_tokens: number;
  reasoning_tokens?: number;
  cost_usd_provided?: number;
  billing_type?: UsageBillingType;
  timestamp: string;
};

export async function usageSummary(range: StatsRange, costMode: UsageCostMode = "auto", agent?: string) {
  const query = new URLSearchParams({ range, cost_mode: costMode });
  if (agent) {
    query.set("agent", agent);
  }
  return request<{ data: UsageSummary }>(`/api/v1/users/current/usage_events/summary?${query.toString()}`);
}

export async function usageBlocks(range: StatsRange, costMode: UsageCostMode = "auto") {
  const query = new URLSearchParams({ range, cost_mode: costMode });
  return request<{ data: UsageBlocks }>(`/api/v1/users/current/usage_events/blocks?${query.toString()}`);
}

export async function usageExport(start: string, end: string) {
  const query = new URLSearchParams({ start, end });
  return request<{ data: UsageEvent[] }>(`/api/v1/users/current/usage_events?${query.toString()}`);
}

export async function listCustomPricing() {
  return request<{ data: CustomPricing[] }>("/api/v1/users/current/custom_pricing");
}

export async function upsertCustomPricing(pricing: CustomPricing) {
  return request<{ data: CustomPricing[] }>("/api/v1/users/current/custom_pricing", {
    method: "PUT",
    body: JSON.stringify(pricing)
  });
}

export async function deleteCustomPricing(model: string) {
  return request<void>(`/api/v1/users/current/custom_pricing/${encodeURIComponent(model)}`, { method: "DELETE" });
}

// BillingPref is a per-agent billing-mode override: declare an agent as flat-rate
// subscription (marginal cost $0) or metered api (marginal = equivalent-API cost),
// overriding the billing_type the collecting adapter stamped on stored events.
export type BillingPref = {
  agent: string;
  billing_type: "api" | "subscription";
};

export async function listBillingPrefs() {
  return request<{ data: BillingPref[] }>("/api/v1/users/current/billing_prefs");
}

export async function upsertBillingPref(pref: BillingPref) {
  return request<{ data: BillingPref[] }>("/api/v1/users/current/billing_prefs", {
    method: "PUT",
    body: JSON.stringify(pref)
  });
}

export async function deleteBillingPref(agent: string) {
  return request<void>(`/api/v1/users/current/billing_prefs/${encodeURIComponent(agent)}`, { method: "DELETE" });
}
