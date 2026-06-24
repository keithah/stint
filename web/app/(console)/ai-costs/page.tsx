"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { AlertTriangle, ArrowRight, Coins, Database, Flame, RefreshCw, Sparkles, Wallet, X, Zap } from "lucide-react";
import { useMemo, useState } from "react";
import { StatCard } from "@/components/stat-card";
import { TokenTypeBar } from "@/components/token-type-bar";
import { UsageBlockPanel } from "@/components/usage-block-panel";
import { UsageBreakdown } from "@/components/usage-breakdown";
import { AuthGate, EmptyState, HeroHeader, SecondaryButton, SegmentedToggle, Skeleton, pillWrapperClass } from "@/components/ui";
import { me, type StatsRange } from "@/lib/api";
import { usageBlocks, usageSummary, type UsageCostMode, type UsageCurrentBlock, type UsageSummary } from "@/lib/usage-api";
import { activityHeatmapClass, activityHeatmapClassForLevel } from "@/lib/activity-heatmap";
import { compactNumber, formatUSD } from "@/lib/number-format";
import { rangeOptions, costModeOptions } from "@/lib/ranges";
import { hasSubscriptionSavings, subscriptionCovered } from "@/lib/usage-billing";
import {
  cacheEfficiency,
  cachingDollarSavings,
  costPerProjectPerDay,
  latestDay,
  modelCostExtremes,
  reasoningShare,
  todayVsAverage
} from "@/lib/usage-insights";

export default function AICostsPage() {
  return (
    <AICostsContent />
  );
}

function AICostsContent() {
  const [range, setRange] = useState<StatsRange>("last_30_days");
  const [costMode, setCostMode] = useState<UsageCostMode>("auto");
  const [agent, setAgent] = useState<string | null>(null);

  const user = useQuery({ queryKey: ["me"], queryFn: me, retry: false });
  const summary = useQuery({
    queryKey: ["usage-summary", range, costMode, agent],
    queryFn: () => usageSummary(range, costMode, agent ?? undefined),
    retry: false,
    refetchInterval: 60000
  });
  const blocks = useQuery({
    queryKey: ["usage-blocks", costMode, agent],
    queryFn: () => usageBlocks("last_30_days", costMode, agent ?? undefined),
    retry: false,
    refetchInterval: 60000
  });

  const data = summary.data?.data;
  const currentBlock = blocks.data?.data.current ?? null;
  const activeRange = rangeOptions.find((item) => item.value === range) ?? rangeOptions[0];
  // Today's cost/tokens come from the summary's by_day series (which includes
  // today for the rolling ranges) — no separate poller needed.
  const liveToday = latestDay(data?.by_day ?? []);

  if (user.isError) {
    return <AuthGate message="Create a session before viewing AI costs." />;
  }

  return (
    <div className="mx-auto max-w-7xl px-5 py-6 lg:px-8">
      <HeroHeader
        srLabel={activeRange.label}
        caption={liveToday?.date ? `AI spend · today · ${liveToday.date.slice(5)}` : "AI spend · today"}
        value={formatUSD(liveToday?.cost_usd ?? 0)}
        accentValue
        freshness={summary.isFetching ? "Updating…" : "Live"}
        freshnessActive={summary.isFetching}
        subline={
          <>
            {compactNumber(liveToday?.tokens ?? 0)} tokens today{agent ? <> · <span className="text-accent">{agent}</span></> : <> across all agents</>}
            {data ? <> · {data.total.event_count.toLocaleString()} events in {activeRange.label.toLowerCase()}</> : null}
          </>
        }
        controls={
          <>
            <div className="flex flex-col gap-2 sm:items-end">
              <SegmentedToggle options={rangeOptions} value={range} onChange={setRange} variant="pill" className={pillWrapperClass} />
              <SegmentedToggle
                options={costModeOptions}
                value={costMode}
                onChange={setCostMode}
                variant="pill"
                size="xs"
                className={pillWrapperClass}
                optionTitle={(option) => `Cost mode: ${option.label}`}
              />
            </div>
            <div className="flex items-center gap-2">
              {agent ? (
                <button
                  type="button"
                  onClick={() => setAgent(null)}
                  className="inline-flex items-center gap-1.5 rounded-full border border-accent/40 bg-accent/10 px-2.5 py-1 text-xs font-medium text-accent transition hover:bg-accent/20"
                  title="Clear agent filter"
                >
                  Filtered: {agent} <X size={13} />
                </button>
              ) : null}
              <SecondaryButton onClick={() => { summary.refetch(); blocks.refetch(); }}>
                <RefreshCw size={15} className={summary.isFetching ? "animate-spin" : ""} /> Refresh
              </SecondaryButton>
            </div>
          </>
        }
      />

      {summary.isError ? (
        <div className="mt-6 rounded border border-ember/40 bg-ember/10 p-5 text-sm text-ember">
          Could not load usage: {(summary.error as Error)?.message ?? "unknown error"}
        </div>
      ) : null}

      {summary.isLoading ? (
        <div className="mt-6 space-y-5" aria-busy="true" aria-label="Loading usage">
          <div className="grid gap-4 md:grid-cols-4">
            {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-24" />)}
          </div>
          <Skeleton className="h-44" />
        </div>
      ) : null}

      {data && isEmptySummary(data) && !summary.isLoading ? (
        <div className="mt-6">
          <EmptyState
            icon={<Wallet size={20} />}
            title="No AI usage yet"
            hint={agent ? (
              <>No usage recorded for <span className="text-zinc-300">{agent}</span> in this range.</>
            ) : (
              <>Run <code className="rounded bg-ink px-1.5 py-0.5 text-zinc-300">stint-collect</code> to start recording agent usage events.</>
            )}
          />
        </div>
      ) : null}

      {data && !isEmptySummary(data) ? (
        <SummaryBody data={data} activeRange={activeRange} currentBlock={currentBlock} agent={agent} onSelectAgent={setAgent} />
      ) : null}
    </div>
  );
}

function SummaryBody({
  data,
  activeRange,
  currentBlock,
  agent,
  onSelectAgent
}: {
  data: UsageSummary;
  activeRange: { value: StatsRange; label: string };
  currentBlock: UsageCurrentBlock | null;
  agent: string | null;
  onSelectAgent: (name: string | null) => void;
}) {
  const total = data.total;
  const eff = useMemo(() => cacheEfficiency(total), [total]);
  const savings = useMemo(() => cachingDollarSavings(total), [total]);
  const extremes = useMemo(() => modelCostExtremes(data.by_model), [data.by_model]);
  const reasoning = useMemo(() => reasoningShare(total), [total]);
  const burn = useMemo(() => todayVsAverage(data.by_day), [data.by_day]);
  const perProject = useMemo(() => costPerProjectPerDay(data.by_project, data.by_day.length), [data.by_project, data.by_day.length]);
  const subscription = hasSubscriptionSavings(total);

  // Toggle the agent filter: clicking the already-selected agent clears it.
  const toggleAgent = (name: string) => onSelectAgent(agent === name ? null : name);

  return (
    <>
      {data.unpriced_models.length > 0 ? (
        <div className="mt-6 flex flex-col gap-3 rounded border border-ember/40 bg-ember/10 p-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-start gap-3 text-sm text-ember">
            <AlertTriangle size={18} className="mt-0.5 shrink-0" />
            <div>
              <div className="font-medium">{data.unpriced_models.length} {data.unpriced_models.length === 1 ? "model" : "models"} unpriced — add custom pricing</div>
              <div className="mt-1 text-xs text-ember/80">{data.unpriced_models.join(", ")}</div>
            </div>
          </div>
          <Link className="inline-flex shrink-0 items-center justify-center gap-2 rounded border border-ember/50 px-3 py-2 text-sm text-ember hover:bg-ember/10" href="/settings">
            Set pricing <ArrowRight size={15} />
          </Link>
        </div>
      ) : null}

      {subscription ? (
        <section className="mt-6 rounded border border-moss/30 bg-moss/[0.06] p-5">
          <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex items-start gap-3">
              <span className="mt-0.5 text-moss"><Wallet size={18} /></span>
              <div>
                <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                  <span className="text-2xl font-semibold tracking-tight text-zinc-50">{formatUSD(total.cost_usd)}</span>
                  <span className="text-sm text-zinc-400">equivalent API value</span>
                  <span className="text-zinc-600">·</span>
                  <span className="text-2xl font-semibold tracking-tight text-moss">{formatUSD(total.marginal_usd)}</span>
                  <span className="text-sm text-zinc-400">out-of-pocket</span>
                </div>
                <p className="mt-1.5 max-w-2xl text-xs leading-5 text-zinc-500">
                  Subscription-billed agents cover <span className="text-moss">{formatUSD(subscriptionCovered(total))}</span> of metered-API-equivalent
                  usage at no marginal cost. Out-of-pocket is what you actually pay for this range.
                </p>
              </div>
            </div>
          </div>
        </section>
      ) : null}

      <section className="mt-6 grid gap-4 md:grid-cols-4">
        <StatCard
          label={`${activeRange.label} cost`}
          value={formatUSD(total.cost_usd)}
          detail={subscription ? `${formatUSD(total.marginal_usd)} out-of-pocket` : `${total.event_count.toLocaleString()} events`}
        />
        <StatCard label="Tokens" value={compactNumber(totalTokens(total))} detail={`${total.event_count.toLocaleString()} events`} />
        <StatCard label="Cache hit ratio" value={`${(eff.cacheHitRatio * 100).toFixed(1)}%`} detail={eff.hasData ? `${compactNumber(eff.cacheReadTokens)} cached vs ${compactNumber(eff.freshInputTokens)} fresh` : "No input data"} />
        <StatCard label="Reasoning share" value={`${(reasoning * 100).toFixed(1)}%`} detail={`${compactNumber(total.reasoning_tokens)} reasoning tokens`} />
      </section>

      <section className="mt-6 grid gap-5 lg:grid-cols-3">
        <UsageBreakdown title="By agent" rows={data.by_agent} showBilling onSelect={toggleAgent} selected={agent} />
        <UsageBreakdown title="By model" rows={data.by_model} />
        <UsageBreakdown title="By project" rows={data.by_project} />
      </section>

      <section className="mt-6 grid gap-5 lg:grid-cols-[1fr_1fr] xl:grid-cols-[1.4fr_1fr]">
        <TokenTypeBar total={total} />
        <UsageBlockPanel block={currentBlock} />
      </section>

      <section className="mt-6">
        <CostHeatmap data={data} />
      </section>

      <section className="mt-6 grid gap-5 lg:grid-cols-3">
        <InsightCard icon={<Database size={16} />} title="Cache efficiency">
          <Metric label="Cache hit ratio" value={`${(eff.cacheHitRatio * 100).toFixed(1)}%`} />
          <Metric label="Saved by caching" value={savings.hasData ? `${formatUSD(savings.savedUSD)} (${(savings.savingsRatio * 100).toFixed(0)}%)` : "—"} />
          <Metric label="Without caching" value={savings.hasData ? formatUSD(savings.uncachedUSD) : "—"} />
          <p className="mt-2 text-xs leading-5 text-zinc-500">
            {savings.hasData
              ? `Cache reads (${compactNumber(eff.cacheReadTokens)} tok) bill far below fresh input. Uncached, this range would cost ${formatUSD(savings.uncachedUSD)} — trackers that ignore caching report that inflated number.`
              : "No input/cache tokens recorded yet."}
          </p>
        </InsightCard>

        <InsightCard icon={<Coins size={16} />} title="Model spread">
          {extremes.mostExpensive ? (
            <>
              <Metric label="Most expensive" value={`${extremes.mostExpensive.name} · ${formatUSD(extremes.mostExpensive.cost_usd)}`} />
              <Metric label="Cheapest priced" value={extremes.cheapest ? `${extremes.cheapest.name} · ${formatUSD(extremes.cheapest.cost_usd)}` : "—"} />
              <Metric label="Reasoning tokens" value={`${compactNumber(total.reasoning_tokens)} (${(reasoning * 100).toFixed(1)}%)`} />
            </>
          ) : (
            <p className="text-xs text-zinc-500">No priced models in this range.</p>
          )}
        </InsightCard>

        <InsightCard icon={<Flame size={16} />} title="Burn rate & anomalies">
          {burn && burn.priorDayCount > 0 && burn.averageCost > 0 ? (
            <>
              <Metric label="Today" value={formatUSD(burn.todayCost)} />
              <Metric label={`${burn.priorDayCount}-day average`} value={formatUSD(burn.averageCost)} />
              <div className={`mt-2 inline-flex items-center gap-2 rounded px-2.5 py-1 text-xs ${burn.isAnomaly ? "bg-ember/15 text-ember" : "bg-white/5 text-zinc-400"}`}>
                {burn.isAnomaly ? <Flame size={13} /> : <Zap size={13} />}
                Today is {burn.multiple.toFixed(1)}× your {burn.priorDayCount}-day average
              </div>
            </>
          ) : (
            <p className="text-xs text-zinc-500">Not enough days to compute a burn-rate baseline yet.</p>
          )}
        </InsightCard>
      </section>

      <section className="mt-6 rounded border border-line bg-panel/95 p-5">
        <div className="mb-4 flex items-center gap-2 text-sm font-medium text-zinc-300">
          <Sparkles size={16} /> Cost per project per day
        </div>
        {perProject.length > 0 ? (
          <div className="space-y-3">
            {perProject.slice(0, 8).map((row) => (
              <div key={row.name} className="flex items-center justify-between gap-3 text-sm">
                <span className="truncate text-zinc-300">{row.name}</span>
                <span className="shrink-0 text-zinc-500">{formatUSD(row.perDay)}/day</span>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-zinc-500">No project usage recorded yet.</p>
        )}
      </section>
    </>
  );
}

function CostHeatmap({ data }: { data: UsageSummary }) {
  const days = data.by_day;
  const maxCost = Math.max(1, ...days.map((day) => day.cost_usd));
  return (
    <div className="ops-chart-panel rounded border border-line bg-panel/95 p-4 shadow-[0_1px_0_rgba(255,255,255,0.04)]">
      <div className="mb-4 flex items-center justify-between gap-3">
        <div className="text-sm font-medium text-zinc-300">Cross-agent day heatmap</div>
        <div className="flex items-center gap-2 text-xs text-zinc-500">
          <span>Less</span>
          {[0, 1, 2, 3, 4].map((level) => (
            <span key={level} className={`h-3 w-3 rounded-sm border ${activityHeatmapClassForLevel(level)}`} />
          ))}
          <span>More</span>
        </div>
      </div>
      {days.length > 0 ? (
        <div className="flex flex-wrap gap-1.5">
          {days.map((day) => {
            const cellTitle = `${day.date}: ${formatUSD(day.cost_usd)} · ${compactNumber(day.tokens)} tokens`;
            return (
              <div
                key={day.date}
                // Reuse the activity-heatmap colour ramp, scaled to cost in cents.
                className={`h-5 w-5 rounded-sm border ${activityHeatmapClass({ total_seconds: Math.round(day.cost_usd * 100) }, Math.round(maxCost * 100))}`}
                title={cellTitle}
                aria-label={cellTitle}
              />
            );
          })}
        </div>
      ) : (
        <p className="text-sm text-zinc-500">No daily usage in this range.</p>
      )}
    </div>
  );
}

function InsightCard({ icon, title, children }: { icon: React.ReactNode; title: string; children: React.ReactNode }) {
  return (
    <div className="rounded border border-line bg-panel/95 p-4 shadow-[0_1px_0_rgba(255,255,255,0.04)]">
      <div className="mb-3 flex items-center gap-2 text-sm font-medium text-zinc-300">
        <span className="text-accent">{icon}</span>
        {title}
      </div>
      <div className="space-y-2">{children}</div>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 text-sm">
      <span className="text-zinc-500">{label}</span>
      <span className="truncate text-right font-medium text-zinc-200">{value}</span>
    </div>
  );
}

function totalTokens(total: UsageSummary["total"]) {
  return total.input_tokens + total.output_tokens + total.cache_create_tokens + total.cache_read_tokens + total.reasoning_tokens;
}

function isEmptySummary(data: UsageSummary) {
  return data.total.event_count === 0 && totalTokens(data.total) === 0 && data.total.cost_usd === 0;
}
