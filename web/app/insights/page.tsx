"use client";

import { useQuery } from "@tanstack/react-query";
import { BarChart3, CalendarDays } from "lucide-react";
import { useState } from "react";
import { AppShell } from "@/components/app-shell";
import { PageHeader, SegmentedToggle, pillWrapperClass } from "@/components/ui";
import { activityHeatmapClass, activityHeatmapClassForLevel, activityHeatmapTitle } from "@/lib/activity-heatmap";
import {
  insight,
  statsForRange,
  type AIStat,
  type DailyAverageTrendStat,
  type DailyStat,
  type HourlyStat,
  type SliceTotal,
  type Stats,
  type StatsRange,
  type WeekdayStat
} from "@/lib/api";
import { minimumVisiblePercent } from "@/lib/chart-percent";

const ranges: Array<{ value: StatsRange; label: string }> = [
  { value: "last_7_days", label: "7 days" },
  { value: "last_30_days", label: "30 days" },
  { value: "last_6_months", label: "6 months" },
  { value: "last_year", label: "Year" },
  { value: "all_time", label: "All time" }
];

const breakdowns = [
  "stats",
  "projects",
  "languages",
  "editors",
  "machines",
  "operating_systems",
  "categories",
  "dependencies",
  "days",
  "hours",
  "weekdays",
  "best_day",
  "daily_average",
  "daily_average_trend",
  "ai_agents",
  "ai_days"
] as const;

type DailyAverageInsightValue = {
  seconds: number;
  text: string;
};

export default function InsightsPage() {
  return (
    <AppShell>
      <InsightsContent />
    </AppShell>
  );
}

function InsightsContent() {
  const [range, setRange] = useState<StatsRange>("last_30_days");
  const [breakdown, setBreakdown] = useState<(typeof breakdowns)[number]>("stats");
  const stats = useQuery({ queryKey: ["stats", range], queryFn: () => statsForRange(range), retry: false });
  const rows = useQuery({ queryKey: ["insight", breakdown, range], queryFn: () => insight(breakdown, range), retry: false });

  const sliceRows = Array.isArray(rows.data?.data) ? (rows.data?.data as Array<SliceTotal | AIStat | DailyStat | HourlyStat | WeekdayStat | DailyAverageTrendStat>) : [];
  const statsOverview = breakdown === "stats" && rows.data?.data && !Array.isArray(rows.data.data) ? (rows.data.data as Stats) : undefined;
  const dayRows = breakdown === "days" ? (sliceRows as DailyStat[]) : [];
  const hourRows = breakdown === "hours" ? (sliceRows as HourlyStat[]) : [];
  const weekdayRows = breakdown === "weekdays" ? (sliceRows as WeekdayStat[]) : [];
  const bestDay = breakdown === "best_day" && rows.data?.data && !Array.isArray(rows.data.data) ? (rows.data.data as DailyStat) : undefined;
  const dailyAverage = breakdown === "daily_average" && rows.data?.data && !Array.isArray(rows.data.data) ? (rows.data.data as DailyAverageInsightValue) : undefined;
  const averageTrendRows = breakdown === "daily_average_trend" ? (sliceRows as DailyAverageTrendStat[]) : [];

  return (
    <div className="mx-auto max-w-6xl px-5 py-6 lg:px-8">
      <PageHeader
        icon={<BarChart3 size={14} />}
        caption="Breakdown explorer"
        title="Insights"
        sub="Range-aware totals by project, language, editor, machine, OS, and category."
        actions={<SegmentedToggle options={ranges} value={range} onChange={setRange} variant="pill" className={pillWrapperClass} />}
      />

      <section className="grid gap-5 lg:grid-cols-[220px_1fr]">
        <div className="rounded border border-line bg-panel p-3">
          {breakdowns.map((item) => (
            <button
              key={item}
              className={`mb-2 block w-full rounded px-3 py-2 text-left text-sm last:mb-0 ${breakdown === item ? "bg-white/10 text-zinc-50" : "text-zinc-400 hover:bg-white/5"}`}
              onClick={() => setBreakdown(item)}
            >
              {labelFor(item)}
            </button>
          ))}
        </div>

        <div className="rounded border border-line bg-panel">
          <div className="flex items-center justify-between border-b border-line px-4 py-3">
            <div>
              <h2 className="font-medium">{labelFor(breakdown)}</h2>
              <p className="mt-1 text-sm text-zinc-500">{stats.data?.data.human_readable_total ?? "0 secs"} total in selected range</p>
            </div>
          </div>
          {breakdown === "stats" ? <StatsOverview stats={statsOverview ?? stats.data?.data} /> : null}
          {breakdown === "days" ? <DailyRows rows={dayRows} fallbackTotal={stats.data?.data.total_seconds ?? 0} /> : null}
          {breakdown === "hours" ? <HourlyRows rows={hourRows} fallbackTotal={stats.data?.data.total_seconds ?? 0} /> : null}
          {breakdown === "weekdays" ? <WeekdayPattern rows={weekdayRows} /> : null}
          {breakdown === "best_day" ? <BestDayInsight day={bestDay} /> : null}
          {breakdown === "daily_average" ? <DailyAverageInsight average={dailyAverage} /> : null}
          {breakdown === "daily_average_trend" ? <DailyAverageTrend rows={averageTrendRows} /> : null}
          {!["stats", "days", "hours", "weekdays", "best_day", "daily_average", "daily_average_trend"].includes(breakdown) ? (
            <InsightRows rows={sliceRows as Array<SliceTotal | AIStat>} fallbackTotal={stats.data?.data.total_seconds ?? 0} />
          ) : null}
        </div>
      </section>

      <section className="mt-6 rounded border border-line bg-panel p-5">
        <ActivityHeatmap days={stats.data?.data.days ?? []} />
        <div className="mb-4 mt-6 flex items-center gap-2 text-sm font-medium text-zinc-300">
          <CalendarDays size={16} /> Daily buckets
        </div>
        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
          {(stats.data?.data.days ?? []).slice(-16).map((day: DailyStat) => (
            <div key={day.date} className="rounded border border-line bg-ink px-3 py-2">
              <div className="text-xs text-zinc-500">{day.date}</div>
              <div className="mt-1 text-sm text-zinc-200">{day.text}</div>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}

function ActivityHeatmap({ days }: { days: DailyStat[] }) {
  const visibleDays = days.slice(-90);
  const maxSeconds = Math.max(1, ...visibleDays.map((day) => day.total_seconds));
  const activeDays = visibleDays.filter((day) => day.total_seconds > 0).length;
  return (
    <div>
      <div className="mb-4 flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
        <div>
          <div className="flex items-center gap-2 text-sm font-medium text-zinc-300">
            <CalendarDays size={16} /> Coding heatmap
          </div>
          <p className="mt-1 text-xs text-zinc-500">{activeDays} active days across the visible range.</p>
        </div>
        <div className="flex items-center gap-2 text-xs text-zinc-500">
          <span>Less</span>
          {[0, 1, 2, 3, 4].map((level) => (
            <span key={level} className={`h-3 w-3 rounded-sm border ${activityHeatmapClassForLevel(level)}`} />
          ))}
          <span>More</span>
        </div>
      </div>
      <div className="grid grid-cols-[repeat(auto-fill,minmax(14px,1fr))] gap-1.5">
        {visibleDays.map((day) => (
          <div key={day.date} className={`aspect-square rounded border ${activityHeatmapClass(day, maxSeconds)}`} title={activityHeatmapTitle(day)} aria-label={activityHeatmapTitle(day)} />
        ))}
        {visibleDays.length === 0 ? <div className="col-span-full text-sm text-zinc-500">No daily activity has been computed for this range yet.</div> : null}
      </div>
    </div>
  );
}

function StatsOverview({ stats }: { stats?: Stats }) {
  const tiles = [
    { label: "Total", value: stats?.human_readable_total ?? "0 secs" },
    { label: "Daily average", value: stats?.human_readable_daily_average ?? "0 secs" },
    { label: "Best day", value: stats?.best_day?.text ?? "0 secs", detail: stats?.best_day?.date ?? "No activity" },
    { label: "Calculated", value: `${stats?.percent_calculated ?? 0}%`, detail: stats?.is_up_to_date ? "Fresh" : "Refreshing" }
  ];
  return (
    <div className="space-y-5 p-4">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {tiles.map((tile) => (
          <div key={tile.label} className="rounded border border-line bg-ink p-3">
            <div className="text-xs text-zinc-500">{tile.label}</div>
            <div className="mt-1 text-lg font-medium text-zinc-100">{tile.value}</div>
            {tile.detail ? <div className="mt-1 text-xs text-zinc-500">{tile.detail}</div> : null}
          </div>
        ))}
      </div>
      <div className="grid gap-4 lg:grid-cols-3">
        <TopList title="Projects" rows={stats?.projects ?? []} total={stats?.total_seconds ?? 0} />
        <TopList title="Languages" rows={stats?.languages ?? []} total={stats?.total_seconds ?? 0} />
        <TopList title="Editors" rows={stats?.editors ?? []} total={stats?.total_seconds ?? 0} />
      </div>
    </div>
  );
}

function TopList({ title, rows, total }: { title: string; rows: SliceTotal[]; total: number }) {
  return (
    <div className="rounded border border-line bg-ink p-3">
      <h3 className="text-sm font-medium text-zinc-200">{title}</h3>
      <div className="mt-3 space-y-3">
        {rows.slice(0, 5).map((row) => (
          <div key={row.name}>
            <div className="flex items-center justify-between gap-3 text-sm">
              <span className="truncate text-zinc-300">{row.name}</span>
              <span className="shrink-0 text-zinc-500">{row.text}</span>
            </div>
            <div className="mt-1.5 h-1.5 overflow-hidden rounded bg-white/5">
              <div className="h-full rounded bg-accent" style={{ width: `${percent(row.total_seconds, total)}%` }} />
            </div>
          </div>
        ))}
        {rows.length === 0 ? <div className="text-sm text-zinc-500">No activity yet.</div> : null}
      </div>
    </div>
  );
}

function InsightRows({ rows, fallbackTotal }: { rows: Array<SliceTotal | AIStat>; fallbackTotal: number }) {
  return (
    <div className="divide-y divide-line">
      {rows.map((row, index) => (
        <div key={row.name} className="grid grid-cols-[40px_1fr_120px] items-center gap-4 px-4 py-3">
          <span className="text-sm text-zinc-500">{index + 1}</span>
          <div className="min-w-0">
            <div className="truncate text-sm font-medium text-zinc-100">{row.name}</div>
            <div className="mt-2 h-1.5 overflow-hidden rounded bg-white/5">
              <div className="h-full rounded bg-accent" style={{ width: `${percent(rowValue(row), totalForRows(rows, fallbackTotal))}%` }} />
            </div>
          </div>
          <span className="text-right text-sm text-zinc-400">{rowLabel(row)}</span>
        </div>
      ))}
      {rows.length === 0 ? <div className="p-5 text-sm text-zinc-500">No data for this breakdown yet.</div> : null}
    </div>
  );
}

function DailyRows({ rows, fallbackTotal }: { rows: DailyStat[]; fallbackTotal: number }) {
  return (
    <div className="divide-y divide-line">
      {rows.map((row) => (
        <div key={row.date} className="grid gap-3 px-4 py-3 sm:grid-cols-[120px_1fr_120px] sm:items-center">
          <div className="text-sm font-medium text-zinc-200">{row.date}</div>
          <div className="min-w-0">
            <div className="h-1.5 overflow-hidden rounded bg-white/5">
              <div className="h-full rounded bg-accent" style={{ width: `${percent(row.total_seconds, fallbackTotal)}%` }} />
            </div>
            <div className="mt-2 truncate text-xs text-zinc-500">{topProjectNames(row.projects ?? [])}</div>
          </div>
          <div className="text-right text-sm text-zinc-400">{row.text}</div>
        </div>
      ))}
      {rows.length === 0 ? <div className="p-5 text-sm text-zinc-500">No daily activity in this range yet.</div> : null}
    </div>
  );
}

function HourlyRows({ rows, fallbackTotal }: { rows: HourlyStat[]; fallbackTotal: number }) {
  return (
    <div className="divide-y divide-line">
      {rows.map((row) => (
        <div key={row.hour} className="grid gap-3 px-4 py-3 sm:grid-cols-[88px_1fr_120px] sm:items-center">
          <div className="text-sm font-medium text-zinc-200">{row.label}</div>
          <div className="min-w-0">
            <div className="h-1.5 overflow-hidden rounded bg-white/5">
              <div className="h-full rounded bg-accent" style={{ width: `${percent(row.total_seconds, fallbackTotal)}%` }} />
            </div>
            <div className="mt-2 truncate text-xs text-zinc-500">{topProjectNames(row.projects ?? [])}</div>
          </div>
          <div className="text-right text-sm text-zinc-400">{row.text}</div>
        </div>
      ))}
      {rows.length === 0 ? <div className="p-5 text-sm text-zinc-500">No hourly activity in this range yet.</div> : null}
    </div>
  );
}

function BestDayInsight({ day }: { day?: DailyStat }) {
  return (
    <div className="p-4">
      <div className="rounded border border-line bg-ink p-4">
        <div className="text-xs text-zinc-500">Best day</div>
        <div className="mt-2 text-2xl font-semibold text-zinc-100">{day?.text ?? "0 secs"}</div>
        <div className="mt-1 text-sm text-zinc-500">{day?.date ?? "No activity in this range"}</div>
        <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {(day?.projects ?? []).slice(0, 3).map((project) => (
            <div key={project.name} className="rounded border border-line bg-panel px-3 py-2">
              <div className="truncate text-sm text-zinc-200">{project.name}</div>
              <div className="mt-1 text-xs text-zinc-500">{project.text}</div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function DailyAverageInsight({ average }: { average?: DailyAverageInsightValue }) {
  return (
    <div className="p-4">
      <div className="rounded border border-line bg-ink p-4">
        <div className="text-xs text-zinc-500">Daily average</div>
        <div className="mt-2 text-2xl font-semibold text-zinc-100">{average?.text ?? "0 secs"}</div>
        <div className="mt-1 text-sm text-zinc-500">{(average?.seconds ?? 0).toLocaleString()} seconds per day</div>
      </div>
    </div>
  );
}

function DailyAverageTrend({ rows }: { rows: DailyAverageTrendStat[] }) {
  const maxSeconds = Math.max(1, ...rows.map((row) => Math.max(row.total_seconds, row.average_seconds)));
  const lastRow = rows[rows.length - 1];
  return (
    <div className="p-4">
      <div className="grid h-72 items-end gap-1 border-b border-line pb-3" style={{ gridTemplateColumns: `repeat(${Math.max(rows.length, 1)}, minmax(10px, 1fr))` }}>
        {rows.map((row) => (
          <div key={row.date} className="flex h-full flex-col justify-end gap-1" title={`${row.date}: ${row.text}, ${row.average_text} average`}>
            <div className="rounded-t bg-accent/80" style={{ height: `${Math.max(3, Math.round((row.average_seconds / maxSeconds) * 100))}%` }} />
            <div className="rounded-t bg-white/20" style={{ height: `${Math.max(3, Math.round((row.total_seconds / maxSeconds) * 100))}%` }} />
          </div>
        ))}
      </div>
      <div className="mt-4 grid gap-3 sm:grid-cols-3">
        <TrendStat label="Current average" value={lastRow?.average_text ?? "0 secs"} />
        <TrendStat label="Latest day" value={lastRow?.text ?? "0 secs"} />
        <TrendStat label="Days included" value={`${lastRow?.day_count ?? 0}`} />
      </div>
      <div className="mt-4 flex flex-wrap gap-3 text-xs text-zinc-500">
        <span className="inline-flex items-center gap-2"><span className="h-2.5 w-2.5 rounded-sm bg-accent/80" /> Cumulative average</span>
        <span className="inline-flex items-center gap-2"><span className="h-2.5 w-2.5 rounded-sm bg-white/20" /> Daily total</span>
      </div>
    </div>
  );
}

function TrendStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-line bg-ink p-3">
      <div className="text-xs text-zinc-500">{label}</div>
      <div className="mt-1 text-sm font-medium text-zinc-100">{value}</div>
    </div>
  );
}

function WeekdayPattern({ rows }: { rows: WeekdayStat[] }) {
  const maxSeconds = Math.max(1, ...rows.map((row) => row.total_seconds));
  return (
    <div className="grid gap-3 p-4 md:grid-cols-7">
      {rows.map((row) => {
        const intensity = row.total_seconds / maxSeconds;
        return (
          <div key={row.name} className="rounded border border-line bg-ink p-3">
            <div className="flex items-center justify-between gap-2">
              <div className="text-sm font-medium text-zinc-200">{row.name.slice(0, 3)}</div>
              <div className="text-xs text-zinc-500">{row.active_days} active</div>
            </div>
            <div className="mt-3 h-20 rounded border border-white/5 bg-white/[0.03] p-1">
              <div
                className="h-full rounded bg-accent transition"
                style={{
                  opacity: row.total_seconds > 0 ? 0.25 + intensity * 0.75 : 0.08,
                  transform: `scaleY(${Math.max(0.08, intensity)})`,
                  transformOrigin: "bottom"
                }}
              />
            </div>
            <div className="mt-3 text-sm text-zinc-100">{row.text}</div>
            <div className="mt-1 text-xs text-zinc-500">{row.average_text} avg active day</div>
          </div>
        );
      })}
    </div>
  );
}

function labelFor(value: string) {
  const labels: Record<string, string> = {
    ai_agents: "AI agents",
    ai_days: "AI days",
    daily_average: "Daily average",
    daily_average_trend: "Daily average trend",
    operating_systems: "Operating systems"
  };
  return labels[value] ?? value.replace(/_/g, " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function percent(value: number, total: number) {
  if (!total) {
    return 0;
  }
  return minimumVisiblePercent((value / total) * 100);
}

function rowValue(row: SliceTotal | AIStat) {
  if ("total_seconds" in row) {
    return row.total_seconds;
  }
  return row.ai_line_changes;
}

function rowLabel(row: SliceTotal | AIStat) {
  if ("text" in row) {
    return row.text;
  }
  return `${row.ai_line_changes.toLocaleString()} AI lines`;
}

function totalForRows(rows: Array<SliceTotal | AIStat>, fallback: number) {
  if (rows.some((row) => !("total_seconds" in row))) {
    return rows.reduce((sum, row) => sum + rowValue(row), 0);
  }
  return fallback;
}

function topProjectNames(projects: SliceTotal[]) {
  if (projects.length === 0) {
    return "No project breakdown";
  }
  return projects
    .slice(0, 3)
    .map((project) => project.name)
    .join(", ");
}
