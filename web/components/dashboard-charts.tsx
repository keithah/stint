"use client";

import type { AIStat, DailyStat, HourlyStat, SliceTotal } from "@/lib/api";
import { colorForLanguage, fallbackPalette } from "@/lib/language-colors";
import { useMemo } from "react";

const chartPanelClass = "ops-chart-panel rounded border border-line bg-panel/95 p-4 shadow-[0_1px_0_rgba(255,255,255,0.04)]";
const chartGridColor = "#26262b";
const chartTextColor = "#888";
const chartHeight = 170;
const chartWidth = 640;
const chartPadding = { top: 12, right: 16, bottom: 28, left: 36 };

export function ActivityBars({ days, title = "Last 7 Days" }: { days: DailyStat[]; title?: string }) {
  const rows = useMemo(() => days.map((day) => ({ label: day.date.slice(5), value: day.total_seconds / 3600 })), [days]);
  return (
    <div className={`${chartPanelClass} h-72`}>
      <div className="mb-4 text-sm font-medium text-zinc-300">{title}</div>
      <BarSvg rows={rows} colors={["#00b4d8"]} emptyText="No activity in this range." />
    </div>
  );
}

export function ProjectStackedArea({ days }: { days: DailyStat[] }) {
  const keys = useMemo(() => topProjectKeys(days), [days]);
  const rows = useMemo(() => days.map((day) => {
    const projects = sliceTotalByName(day.projects);
    return {
      label: day.date.slice(5),
      segments: keys.map((key, index) => {
        const project = projects.get(key);
        return { key, value: project ? project.total_seconds / 3600 : 0, color: fallbackPalette[index % fallbackPalette.length] };
      })
    };
  }), [days, keys]);
  return (
    <div className={`${chartPanelClass} h-72`}>
      <div className="mb-4 flex items-center justify-between gap-3">
        <div className="text-sm font-medium text-zinc-300">Project activity</div>
        <div className="text-xs text-zinc-500">Stacked daily hours</div>
      </div>
      <StackedBarSvg rows={rows} emptyText="No project activity in this range." />
    </div>
  );
}

export function SliceDonut({ title, rows, colors = {} }: { title: string; rows: SliceTotal[]; colors?: Record<string, string> }) {
  const total = rows.reduce((sum, row) => sum + row.total_seconds, 0);
  const gradient = donutGradient(rows, colors);
  return (
    <div className={chartPanelClass}>
      <div className="mb-3 text-sm font-medium text-zinc-300">{title}</div>
      <div className="grid gap-4 sm:grid-cols-[160px_1fr]">
        <div className="grid h-40 place-items-center">
          <div className="grid h-32 w-32 place-items-center rounded-full" style={{ background: total > 0 ? gradient : "#26262b" }}>
            <div className="grid h-20 w-20 place-items-center rounded-full bg-panel text-xs font-medium text-zinc-500">{rows.length}</div>
          </div>
        </div>
        <div className="space-y-3 self-center">
          {rows.slice(0, 5).map((row, index) => (
            <div key={row.name} className="flex items-center justify-between gap-3 text-sm">
              <span className="flex min-w-0 items-center gap-2 text-zinc-300">
                <span className="h-2.5 w-2.5 rounded-sm" style={{ background: colorForLanguage(row.name, colors, index) }} />
                <span className="truncate">{row.name}</span>
              </span>
              <span className="shrink-0 text-zinc-500">{row.text}</span>
            </div>
          ))}
          {rows.length === 0 ? <div className="text-sm text-zinc-500">Send a heartbeat to populate this panel.</div> : null}
        </div>
      </div>
    </div>
  );
}

export function SliceBars({ title, rows, colors = {} }: { title: string; rows: SliceTotal[]; colors?: Record<string, string> }) {
  const total = rows.reduce((sum, row) => sum + row.total_seconds, 0);
  return (
    <div className={chartPanelClass}>
      <div className="mb-4 text-sm font-medium text-zinc-300">{title}</div>
      <div className="space-y-3">
        {rows.slice(0, 7).map((row, index) => (
          <div key={row.name}>
            <div className="mb-1.5 flex items-center justify-between gap-3 text-sm">
              <span className="flex min-w-0 items-center gap-2 text-zinc-300">
                <span className="h-2.5 w-2.5 rounded-sm" style={{ background: colorForLanguage(row.name, colors, index) }} />
                <span className="truncate">{row.name}</span>
              </span>
              <span className="shrink-0 text-zinc-500">{row.text}</span>
            </div>
            <div className="h-2 overflow-hidden rounded bg-white/5">
              <div className="h-full rounded" style={{ width: `${barPercent(row.total_seconds, total)}%`, background: colorForLanguage(row.name, colors, index) }} />
            </div>
          </div>
        ))}
        {rows.length === 0 ? <div className="text-sm text-zinc-500">No category data yet.</div> : null}
      </div>
    </div>
  );
}

export function HourlyTimeline({ hours, mode, colors = {} }: { hours: HourlyStat[]; mode: "projects" | "languages"; colors?: Record<string, string> }) {
  const keys = useMemo(() => topTimelineKeys(hours, mode), [hours, mode]);
  const rows = useMemo(() => hours.map((hour) => {
    const values = sliceTotalByName(hour[mode]);
    return {
      label: hour.label,
      segments: keys.map((key, index) => {
        const item = values.get(key);
        return { key, value: item ? item.total_seconds / 3600 : 0, color: colorForLanguage(key, colors, index) };
      })
    };
  }), [hours, keys, mode, colors]);
  return (
    <div className={`${chartPanelClass} h-72`}>
      <div className="mb-4 flex items-center justify-between gap-3">
        <div className="text-sm font-medium text-zinc-300">{mode === "projects" ? "Project timeline" : "Language timeline"}</div>
        <div className="text-xs text-zinc-500">24-hour UTC</div>
      </div>
      <StackedBarSvg rows={rows} labelEvery={3} emptyText="No hourly activity in this range." />
    </div>
  );
}

export function AIHumanByDay({ days, title = "AI vs Human by Day" }: { days: AIStat[]; title?: string }) {
  const rows = useMemo(() => days.map((day) => ({
    label: day.name.slice(5),
    segments: [
      { key: "ai", value: day.ai_line_changes, color: fallbackPalette[0] },
      { key: "human", value: day.human_line_changes, color: fallbackPalette[1] }
    ]
  })), [days]);
  const hasRows = rows.some((row) => row.segments.some((segment) => segment.value > 0));
  return (
    <div className={`${chartPanelClass} h-72`}>
      <div className="mb-4 flex items-center justify-between gap-3">
        <div className="text-sm font-medium text-zinc-300">{title}</div>
        <div className="flex gap-3 text-xs text-zinc-500">
          <span className="inline-flex items-center gap-1.5"><span className="h-2.5 w-2.5 rounded-sm bg-accent" /> AI</span>
          <span className="inline-flex items-center gap-1.5"><span className="h-2.5 w-2.5 rounded-sm bg-lime-400" /> Human</span>
        </div>
      </div>
      <StackedBarSvg rows={rows} emptyText={hasRows ? "" : "No AI or human line-change data in this range."} />
    </div>
  );
}

export function WeekdayHeatmap({ days }: { days: DailyStat[] }) {
  const rows = useMemo(() => weekdayRows(days), [days]);
  const maxSeconds = Math.max(1, ...rows.map((row) => row.totalSeconds));
  return (
    <div className={chartPanelClass}>
      <div className="mb-4 flex items-center justify-between gap-3">
        <div className="text-sm font-medium text-zinc-300">Weekday Pattern</div>
        <div className="text-xs text-zinc-500">Active day average</div>
      </div>
      <div className="grid grid-cols-7 gap-2">
        {rows.map((row) => {
          const intensity = row.totalSeconds / maxSeconds;
          return (
            <div key={row.name} className="min-w-0">
              <div className="mb-2 truncate text-center text-xs text-zinc-500">{row.short}</div>
              <div
                className="h-24 rounded border border-white/5 bg-accent transition"
                title={`${row.name}: ${formatShortDuration(row.averageSeconds)} average across ${row.activeDays} active days`}
                style={{ opacity: row.totalSeconds > 0 ? 0.2 + intensity * 0.8 : 0.08 }}
              />
              <div className="mt-2 text-center text-xs text-zinc-400">{formatShortDuration(row.averageSeconds)}</div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

type BarRow = { label: string; value: number };
type StackedRow = { label: string; segments: Array<{ key: string; value: number; color: string }> };

function BarSvg({ rows, colors, emptyText }: { rows: BarRow[]; colors: string[]; emptyText: string }) {
  const max = Math.max(1, ...rows.map((row) => row.value));
  const plot = chartPlot();
  const barWidth = rows.length ? Math.max(4, plot.width / rows.length - 6) : 0;
  return (
    <div className="relative h-[210px]">
      <svg className="h-full w-full" viewBox={`0 0 ${chartWidth} ${chartHeight}`} role="img">
        <Grid max={max} />
        {rows.map((row, index) => {
          const height = (row.value / max) * plot.height;
          const x = plot.x + index * (plot.width / Math.max(1, rows.length)) + 3;
          const y = plot.y + plot.height - height;
          return <rect key={`${row.label}-${index}`} x={x} y={y} width={barWidth} height={height} rx="3" fill={colors[index % colors.length]}><title>{`${row.label}: ${row.value.toFixed(2)}h`}</title></rect>;
        })}
        <AxisLabels labels={rows.map((row) => row.label)} />
      </svg>
      {rows.length === 0 ? <EmptyChart text={emptyText} /> : null}
    </div>
  );
}

function StackedBarSvg({ rows, labelEvery = 1, emptyText }: { rows: StackedRow[]; labelEvery?: number; emptyText: string }) {
  const max = Math.max(1, ...rows.map((row) => row.segments.reduce((sum, segment) => sum + segment.value, 0)));
  const plot = chartPlot();
  const barWidth = rows.length ? Math.max(3, plot.width / rows.length - 5) : 0;
  const hasRows = rows.some((row) => row.segments.some((segment) => segment.value > 0));
  return (
    <div className="relative h-[210px]">
      <svg className="h-full w-full" viewBox={`0 0 ${chartWidth} ${chartHeight}`} role="img">
        <Grid max={max} />
        {rows.map((row, index) => {
          let y = plot.y + plot.height;
          const x = plot.x + index * (plot.width / Math.max(1, rows.length)) + 2;
          return row.segments.map((segment) => {
            const height = (segment.value / max) * plot.height;
            y -= height;
            return <rect key={`${row.label}-${segment.key}`} x={x} y={y} width={barWidth} height={height} fill={segment.color}><title>{`${row.label} ${segment.key}: ${segment.value.toFixed(2)}`}</title></rect>;
          });
        })}
        <AxisLabels labels={rows.map((row, index) => (index % labelEvery === 0 ? row.label : ""))} />
      </svg>
      {!hasRows ? <EmptyChart text={emptyText} /> : null}
    </div>
  );
}

function Grid({ max }: { max: number }) {
  const plot = chartPlot();
  const ticks = [0, 0.5, 1];
  return (
    <>
      {ticks.map((tick) => {
        const y = plot.y + plot.height - tick * plot.height;
        return (
          <g key={tick}>
            <line x1={plot.x} x2={plot.x + plot.width} y1={y} y2={y} stroke={chartGridColor} strokeWidth="1" />
            <text x={plot.x - 8} y={y + 4} textAnchor="end" fill={chartTextColor} fontSize="10">{formatAxis(max * tick)}</text>
          </g>
        );
      })}
    </>
  );
}

function AxisLabels({ labels }: { labels: string[] }) {
  const plot = chartPlot();
  return (
    <>
      {labels.map((label, index) => {
        if (!label) return null;
        const x = plot.x + index * (plot.width / Math.max(1, labels.length)) + plot.width / Math.max(1, labels.length) / 2;
        return <text key={`${label}-${index}`} x={x} y={chartHeight - 8} textAnchor="middle" fill={chartTextColor} fontSize="10">{label}</text>;
      })}
    </>
  );
}

function EmptyChart({ text }: { text: string }) {
  return text ? <div className="absolute inset-0 grid place-items-center text-sm text-zinc-500">{text}</div> : null;
}

function chartPlot() {
  return {
    x: chartPadding.left,
    y: chartPadding.top,
    width: chartWidth - chartPadding.left - chartPadding.right,
    height: chartHeight - chartPadding.top - chartPadding.bottom
  };
}

function donutGradient(rows: SliceTotal[], colors: Record<string, string>) {
  const total = rows.reduce((sum, row) => sum + row.total_seconds, 0);
  if (total <= 0) {
    return "#26262b";
  }
  let cursor = 0;
  const stops = rows.map((row, index) => {
    const start = cursor;
    cursor += (row.total_seconds / total) * 100;
    const color = colorForLanguage(row.name, colors, index);
    return `${color} ${start.toFixed(2)}% ${cursor.toFixed(2)}%`;
  });
  return `conic-gradient(${stops.join(", ")})`;
}

function topTimelineKeys(hours: HourlyStat[], mode: "projects" | "languages") {
  const totals = new Map<string, number>();
  for (const hour of hours) {
    for (const row of hour[mode] ?? []) {
      totals.set(row.name, (totals.get(row.name) ?? 0) + row.total_seconds);
    }
  }
  return topNames(totals, 5);
}

function topProjectKeys(days: DailyStat[]) {
  const totals = new Map<string, number>();
  for (const day of days) {
    for (const project of day.projects ?? []) {
      totals.set(project.name, (totals.get(project.name) ?? 0) + project.total_seconds);
    }
  }
  return topNames(totals, 5);
}

function sliceTotalByName(rows: SliceTotal[] | undefined) {
  const indexed = new Map<string, SliceTotal>();
  for (const row of rows ?? []) {
    indexed.set(row.name, row);
  }
  return indexed;
}

function topNames(totals: Map<string, number>, limit: number) {
  const top: Array<[string, number]> = [];
  for (const entry of totals.entries()) {
    const insertAt = top.findIndex((candidate) => compareTotals(entry, candidate) < 0);
    if (insertAt === -1) {
      if (top.length < limit) top.push(entry);
      continue;
    }
    top.splice(insertAt, 0, entry);
    if (top.length > limit) top.pop();
  }
  return top.map(([name]) => name);
}

function compareTotals(a: [string, number], b: [string, number]) {
  if (a[1] === b[1]) return a[0].localeCompare(b[0]);
  return b[1] - a[1];
}

function barPercent(value: number, total: number) {
  if (total <= 0) return 0;
  return Math.max(3, Math.min(100, (value / total) * 100));
}

function weekdayRows(days: DailyStat[]) {
  const byDay = new Map<number, { totalSeconds: number; activeDays: number }>();
  for (const day of days) {
    const date = new Date(`${day.date}T00:00:00Z`);
    const index = Number.isNaN(date.getTime()) ? 0 : date.getUTCDay();
    const current = byDay.get(index) ?? { totalSeconds: 0, activeDays: 0 };
    current.totalSeconds += day.total_seconds;
    if (day.total_seconds > 0) current.activeDays++;
    byDay.set(index, current);
  }
  return ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"].map((name, index) => {
    const current = byDay.get(index) ?? { totalSeconds: 0, activeDays: 0 };
    return {
      name,
      short: name.slice(0, 3),
      totalSeconds: current.totalSeconds,
      activeDays: current.activeDays,
      averageSeconds: current.activeDays > 0 ? Math.round(current.totalSeconds / current.activeDays) : 0
    };
  });
}

function formatAxis(value: number) {
  if (value >= 1000) return `${Math.round(value / 1000)}k`;
  if (value >= 10) return Math.round(value).toString();
  return value.toFixed(value > 0 && value < 1 ? 1 : 0);
}

function formatShortDuration(seconds: number) {
  if (seconds <= 0) return "0m";
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  return rest ? `${hours}h ${rest}m` : `${hours}h`;
}
