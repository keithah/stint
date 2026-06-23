"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { AlertTriangle, ArrowRight, Coins, Database, Flame, RefreshCw, Sparkles, Zap } from "lucide-react";
import { useMemo, useState } from "react";
import { SliceDonut } from "@/components/dashboard-charts";
import { Providers } from "@/components/providers";
import { Shell } from "@/components/shell";
import { StatCard } from "@/components/stat-card";
import { me, type SliceTotal, type StatsRange } from "@/lib/api";
import { usageBlocks, usageSummary, type UsageCostMode, type UsageCurrentBlock, type UsageSlice, type UsageSummary } from "@/lib/usage-api";
import { activityHeatmapClass } from "@/lib/activity-heatmap";
import { compactNumber, formatUSD } from "@/lib/number-format";
import {
  cacheEfficiency,
  cacheSavingsEstimate,
  costPerProjectPerDay,
  latestDay,
  modelCostExtremes,
  reasoningShare,
  todayVsAverage
} from "@/lib/usage-insights";

const rangeOptions: Array<{ value: StatsRange; label: string }> = [
  { value: "last_7_days", label: "7 days" },
  { value: "last_30_days", label: "30 days" },
  { value: "last_6_months", label: "6 months" },
  { value: "last_year", label: "Year" },
  { value: "all_time", label: "All time" }
];

const costModeOptions: Array<{ value: UsageCostMode; label: string }> = [
  { value: "auto", label: "Auto" },
  { value: "calculate", label: "Calculate" },
  { value: "display", label: "Display" }
];

export default function AICostsPage() {
  return (
    <Providers>
      <Shell>
        <AICostsContent />
      </Shell>
    </Providers>
  );
}

function AICostsContent() {
  const [range, setRange] = useState<StatsRange>("last_30_days");
  const [costMode, setCostMode] = useState<UsageCostMode>("auto");

  const user = useQuery({ queryKey: ["me"], queryFn: me, retry: false });
  // Near-real-time "today" feed off the shortest range, polled frequently.
  const live = useQuery({
    queryKey: ["usage-summary", "last_7_days", costMode, "live"],
    queryFn: () => usageSummary("last_7_days", costMode),
    retry: false,
    refetchInterval: 45000
  });
  const summary = useQuery({
    queryKey: ["usage-summary", range, costMode],
    queryFn: () => usageSummary(range, costMode),
    retry: false,
    refetchInterval: 120000
  });
  const blocks = useQuery({
    queryKey: ["usage-blocks", costMode],
    queryFn: () => usageBlocks("last_30_days", costMode),
    retry: false,
    refetchInterval: 60000
  });

  const data = summary.data?.data;
  const currentBlock = blocks.data?.data.current ?? null;
  const activeRange = rangeOptions.find((item) => item.value === range) ?? rangeOptions[0];
  const liveToday = latestDay(live.data?.data.by_day ?? []);

  if (user.isError) {
    return (
      <div className="grid min-h-screen place-items-center px-6">
        <div className="max-w-md rounded border border-line bg-panel p-6">
          <h1 className="text-xl font-semibold">Login required</h1>
          <p className="mt-2 text-sm text-zinc-400">Create a session before viewing AI costs.</p>
          <Link className="mt-5 inline-flex items-center gap-2 rounded bg-accent px-4 py-2 font-medium text-ink" href="/login">
            Login <ArrowRight size={16} />
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-7xl px-5 py-6 lg:px-8">
      <LiveHeader
        activeRange={activeRange}
        data={data}
        range={range}
        setRange={setRange}
        costMode={costMode}
        setCostMode={setCostMode}
        todayCost={liveToday?.cost_usd ?? 0}
        todayTokens={liveToday?.tokens ?? 0}
        todayDate={liveToday?.date}
        onRefresh={() => {
          summary.refetch();
          live.refetch();
        }}
      />

      {summary.isError ? (
        <div className="mt-5 rounded border border-ember/40 bg-ember/10 p-5 text-sm text-ember">
          Could not load usage: {(summary.error as Error)?.message ?? "unknown error"}
        </div>
      ) : null}

      {summary.isLoading ? (
        <div className="mt-5 rounded border border-dashed border-line bg-panel/70 p-8 text-center text-sm text-zinc-500">
          Loading usage…
        </div>
      ) : null}

      {data && isEmptySummary(data) && !summary.isLoading ? (
        <div className="mt-5 rounded border border-dashed border-line bg-panel/70 p-8 text-center">
          <div className="text-base font-medium text-zinc-200">No AI usage yet</div>
          <p className="mt-2 text-sm text-zinc-500">
            Run <code className="rounded bg-ink px-1.5 py-0.5 text-zinc-300">stint-collect</code> to start recording agent usage events.
          </p>
        </div>
      ) : null}

      {data && !isEmptySummary(data) ? <SummaryBody data={data} activeRange={activeRange} currentBlock={currentBlock} /> : null}
    </div>
  );
}

function SummaryBody({ data, activeRange, currentBlock }: { data: UsageSummary; activeRange: { value: StatsRange; label: string }; currentBlock: UsageCurrentBlock | null }) {
  const total = data.total;
  const eff = useMemo(() => cacheEfficiency(total), [total]);
  const savings = useMemo(() => cacheSavingsEstimate(total), [total]);
  const extremes = useMemo(() => modelCostExtremes(data.by_model), [data.by_model]);
  const reasoning = useMemo(() => reasoningShare(total), [total]);
  const burn = useMemo(() => todayVsAverage(data.by_day), [data.by_day]);
  const perProject = useMemo(() => costPerProjectPerDay(data.by_project, data.by_day.length), [data.by_project, data.by_day.length]);
  const hasSubscription = Math.abs(total.cost_usd - total.marginal_usd) > 0.005;

  const tokenTypeRows: SliceTotal[] = useMemo(
    () => [
      { name: "Input (fresh)", total_seconds: total.input_tokens, text: compactNumber(total.input_tokens) },
      { name: "Output", total_seconds: total.output_tokens, text: compactNumber(total.output_tokens) },
      { name: "Cache create", total_seconds: total.cache_create_tokens, text: compactNumber(total.cache_create_tokens) },
      { name: "Cache read", total_seconds: total.cache_read_tokens, text: compactNumber(total.cache_read_tokens) },
      { name: "Reasoning", total_seconds: total.reasoning_tokens, text: compactNumber(total.reasoning_tokens) }
    ].filter((row) => row.total_seconds > 0),
    [total]
  );

  return (
    <>
      {data.unpriced_models.length > 0 ? (
        <div className="mt-5 flex flex-col gap-3 rounded border border-ember/40 bg-ember/10 p-4 sm:flex-row sm:items-center sm:justify-between">
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

      <section className="mt-5 grid gap-4 md:grid-cols-4">
        <StatCard label={`${activeRange.label} cost`} value={formatUSD(total.cost_usd)} detail={hasSubscription ? `${formatUSD(total.marginal_usd)} marginal (subscription)` : `${total.event_count.toLocaleString()} events`} />
        <StatCard label="Tokens" value={compactNumber(totalTokens(total))} detail={`${total.event_count.toLocaleString()} events`} />
        <StatCard label="Cache hit ratio" value={`${(eff.cacheHitRatio * 100).toFixed(1)}%`} detail={eff.hasData ? `${compactNumber(eff.cacheReadTokens)} cached vs ${compactNumber(eff.freshInputTokens)} fresh` : "No input data"} />
        <StatCard label="Reasoning share" value={`${(reasoning * 100).toFixed(1)}%`} detail={`${compactNumber(total.reasoning_tokens)} reasoning tokens`} />
      </section>

      <section className="mt-5 grid gap-5 lg:grid-cols-3">
        <UsageDonut title="By agent" rows={data.by_agent} />
        <UsageDonut title="By model" rows={data.by_model} />
        <UsageDonut title="By project" rows={data.by_project} />
      </section>

      <section className="mt-5 grid gap-5 xl:grid-cols-[1fr_1fr]">
        <SliceDonut title="Token type mix" rows={tokenTypeRows} />
        <CostHeatmap data={data} />
      </section>

      <section className="mt-5 grid gap-5 lg:grid-cols-2 xl:grid-cols-3">
        <InsightCard icon={<Database size={16} />} title="Cache efficiency">
          <Metric label="Cache hit ratio" value={`${(eff.cacheHitRatio * 100).toFixed(1)}%`} />
          <Metric label="Estimated savings" value={`~${(savings.savingsRatio * 100).toFixed(1)}% of input cost`} />
          <p className="mt-2 text-xs leading-5 text-zinc-500">
            {eff.hasData
              ? `Cache reads (${compactNumber(eff.cacheReadTokens)} tok) are billed far below fresh input (${compactNumber(eff.freshInputTokens)} tok). A higher ratio means cheaper context.`
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

        <InsightCard icon={<Flame size={16} />} title="Current 5-hour block">
          {currentBlock && currentBlock.is_active ? (
            <>
              <Metric label="Spent this block" value={formatUSD(currentBlock.cost_usd)} />
              <Metric label="Burn rate" value={`${formatUSD(currentBlock.burn_rate_cost_per_hour)}/hr · ${Math.round(currentBlock.burn_rate_tokens_per_min).toLocaleString()} tok/min`} />
              <Metric label="Projected (block end)" value={formatUSD(currentBlock.projected_block_cost_usd)} />
              <Metric label="Projected today" value={formatUSD(currentBlock.projected_day_cost_usd)} />
              <p className="mt-2 text-xs leading-5 text-zinc-600">
                Block ends {new Date(currentBlock.end).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })} · {currentBlock.elapsed_minutes} min elapsed
              </p>
            </>
          ) : (
            <p className="text-xs text-zinc-500">No active 5-hour block — start coding to begin one.</p>
          )}
        </InsightCard>
      </section>

      <section className="mt-5 rounded border border-line bg-panel/95 p-5">
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
        <div className="text-xs text-zinc-500">Color by cost</div>
      </div>
      {days.length > 0 ? (
        <div className="flex flex-wrap gap-1.5">
          {days.map((day) => (
            <div
              key={day.date}
              // Reuse the activity-heatmap colour ramp, scaled to cost in cents.
              className={`h-5 w-5 rounded-sm border ${activityHeatmapClass({ total_seconds: Math.round(day.cost_usd * 100) }, Math.round(maxCost * 100))}`}
              title={`${day.date}: ${formatUSD(day.cost_usd)} · ${compactNumber(day.tokens)} tokens`}
            />
          ))}
        </div>
      ) : (
        <p className="text-sm text-zinc-500">No daily usage in this range.</p>
      )}
    </div>
  );
}

function UsageDonut({ title, rows }: { title: string; rows: UsageSlice[] }) {
  // Map cost-bearing slices onto the shared SliceDonut shape (cost in cents so
  // integer rounding stays meaningful for small dollar amounts).
  const sliceRows: SliceTotal[] = rows
    .map((row) => ({ name: row.name, total_seconds: Math.round(row.cost_usd * 100), text: formatUSD(row.cost_usd) }))
    .filter((row) => row.total_seconds > 0);
  return <SliceDonut title={title} rows={sliceRows} />;
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

function LiveHeader({
  activeRange,
  data,
  range,
  setRange,
  costMode,
  setCostMode,
  todayCost,
  todayTokens,
  todayDate,
  onRefresh
}: {
  activeRange: { value: StatsRange; label: string };
  data?: UsageSummary;
  range: StatsRange;
  setRange: (range: StatsRange) => void;
  costMode: UsageCostMode;
  setCostMode: (mode: UsageCostMode) => void;
  todayCost: number;
  todayTokens: number;
  todayDate?: string;
  onRefresh: () => void;
}) {
  return (
    <header className="ops-dashboard-header mb-6 rounded border border-line bg-panel/95 shadow-[0_1px_0_rgba(255,255,255,0.04)]">
      <div className="grid gap-0 lg:grid-cols-[1fr_auto]">
        <div className="border-b border-line p-5 lg:border-b-0 lg:border-r">
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <span className="inline-flex items-center gap-2 rounded border border-accent/30 bg-accent/10 px-2.5 py-1 text-xs uppercase tracking-[0.16em] text-accent">
              <Coins size={14} /> AI cost ops
            </span>
            <span className="rounded border border-line bg-ink px-2.5 py-1 text-xs text-zinc-500">cost mode: {costMode}</span>
            {data ? <span className="rounded border border-line bg-ink px-2.5 py-1 text-xs text-zinc-500">{data.total.event_count.toLocaleString()} events</span> : null}
          </div>
          <div className="grid gap-4 md:grid-cols-[1fr_auto_auto] md:items-end">
            <div>
              <h1 className="text-3xl font-semibold tracking-tight text-zinc-50">Unified AI cost</h1>
              <p className="mt-2 max-w-2xl text-sm leading-6 text-zinc-400">
                Cross-agent spend, tokens, cache efficiency, and burn rate.
              </p>
            </div>
            <HeaderReadout label={todayDate ? `Today (${todayDate.slice(5)})` : "Today"} value={formatUSD(todayCost)} />
            <HeaderReadout label="Today tokens" value={compactNumber(todayTokens)} />
          </div>
        </div>
        <div className="flex flex-col justify-between gap-4 p-5 lg:min-w-80">
          <div className="grid grid-cols-2 gap-2">
            {rangeOptions.map((option) => (
              <button
                key={option.value}
                className={`rounded border px-3 py-2 text-sm transition ${range === option.value ? "border-accent bg-accent text-ink" : "border-line bg-ink text-zinc-300 hover:border-zinc-500"}`}
                onClick={() => setRange(option.value)}
              >
                {option.label}
              </button>
            ))}
          </div>
          <div className="grid grid-cols-3 gap-2">
            {costModeOptions.map((option) => (
              <button
                key={option.value}
                className={`rounded border px-3 py-2 text-xs transition ${costMode === option.value ? "border-accent bg-accent text-ink" : "border-line bg-ink text-zinc-300 hover:border-zinc-500"}`}
                onClick={() => setCostMode(option.value)}
                title={`Cost mode: ${option.label}`}
              >
                {option.label}
              </button>
            ))}
          </div>
          <button
            className="inline-flex items-center justify-center gap-2 rounded border border-line px-3 py-2 text-sm text-zinc-300 hover:bg-white/5"
            onClick={onRefresh}
          >
            <RefreshCw size={15} /> Refresh
          </button>
        </div>
      </div>
      <div className="sr-only">{activeRange.label}</div>
    </header>
  );
}

function HeaderReadout({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-36 rounded border border-line bg-ink px-3 py-2">
      <div className="text-xs uppercase tracking-[0.14em] text-zinc-500">{label}</div>
      <div className="mt-1 truncate text-lg font-semibold text-zinc-100">{value}</div>
    </div>
  );
}

function totalTokens(total: UsageSummary["total"]) {
  return total.input_tokens + total.output_tokens + total.cache_create_tokens + total.cache_read_tokens + total.reasoning_tokens;
}

function isEmptySummary(data: UsageSummary) {
  return data.total.event_count === 0 && totalTokens(data.total) === 0 && data.total.cost_usd === 0;
}
