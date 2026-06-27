"use client";

import type { AIStat, HourlyStat, DailyStat, SliceTotal } from "@/lib/api";
import { colorForLanguage, fallbackPalette } from "@/lib/language-colors";
import { useMemo } from "react";
import { Area, AreaChart, Bar, BarChart, CartesianGrid, Cell, Pie, PieChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";

const chartPanelClass = "ops-chart-panel rounded border border-line bg-panel/95 p-4 shadow-[0_1px_0_rgba(255,255,255,0.04)]";
const chartTick = { fontSize: 12 };
const smallChartTick = { fontSize: 11 };
const tooltipStyle = { background: "#161618", border: "1px solid #26262b", borderRadius: 8 };
const shortDateTick = (value: string) => value.slice(5);

export function ActivityBars({ days, title = "Last 7 Days" }: { days: DailyStat[]; title?: string }) {
  const data = useMemo(() => days.map((day) => ({ ...day, hours: Number((day.total_seconds / 3600).toFixed(2)) })), [days]);
  return (
    <div className={`${chartPanelClass} h-72`}>
      <div className="mb-4 text-sm font-medium text-zinc-300">{title}</div>
      <ResponsiveContainer width="100%" height="85%">
        <BarChart data={data}>
          <CartesianGrid stroke="#26262b" vertical={false} />
          <XAxis dataKey="date" stroke="#888" tick={chartTick} tickFormatter={shortDateTick} />
          <YAxis stroke="#888" tick={chartTick} />
          <Tooltip contentStyle={tooltipStyle} />
          <Bar dataKey="hours" fill="#00b4d8" radius={[4, 4, 0, 0]} />
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

export function ProjectStackedArea({ days }: { days: DailyStat[] }) {
  const keys = useMemo(() => topProjectKeys(days), [days]);
  const data = useMemo(() => days.map((day) => {
    const row: Record<string, string | number> = { date: day.date, label: day.date.slice(5), total: Number((day.total_seconds / 3600).toFixed(2)) };
    const projectsByName = new Map((day.projects ?? []).map((entry) => [entry.name, entry]));
    for (const key of keys) {
      const project = projectsByName.get(key);
      row[key] = project ? Number((project.total_seconds / 3600).toFixed(2)) : 0;
    }
    return row;
  }), [days, keys]);
  return (
    <div className={`${chartPanelClass} h-72`}>
      <div className="mb-4 flex items-center justify-between gap-3">
        <div className="text-sm font-medium text-zinc-300">Project activity</div>
        <div className="text-xs text-zinc-500">Stacked daily hours</div>
      </div>
      <ResponsiveContainer width="100%" height="82%">
        <AreaChart data={data}>
          <CartesianGrid stroke="#26262b" vertical={false} />
          <XAxis dataKey="label" stroke="#888" tick={chartTick} />
          <YAxis stroke="#888" tick={chartTick} />
          <Tooltip contentStyle={tooltipStyle} />
          {keys.map((key, index) => (
            <Area key={key} dataKey={key} stackId="projects" stroke={fallbackPalette[index % fallbackPalette.length]} fill={fallbackPalette[index % fallbackPalette.length]} fillOpacity={0.72} />
          ))}
        </AreaChart>
      </ResponsiveContainer>
      {keys.length === 0 ? <div className="-mt-40 text-center text-sm text-zinc-500">No project activity in this range.</div> : null}
    </div>
  );
}

export function SliceDonut({ title, rows, colors = {} }: { title: string; rows: SliceTotal[]; colors?: Record<string, string> }) {
  const data = rows.length ? rows : [{ name: "No data", total_seconds: 1, text: "0 secs" }];
  return (
    <div className={chartPanelClass}>
      <div className="mb-3 text-sm font-medium text-zinc-300">{title}</div>
      <div className="grid gap-4 sm:grid-cols-[160px_1fr]">
        <div className="h-40">
          <ResponsiveContainer width="100%" height="100%">
            <PieChart>
              <Pie data={data} dataKey="total_seconds" nameKey="name" innerRadius={48} outerRadius={70} paddingAngle={3}>
                {data.map((row, index) => (
                  <Cell key={index} fill={colorForLanguage(row.name, colors, index)} />
                ))}
              </Pie>
              <Tooltip contentStyle={tooltipStyle} />
            </PieChart>
          </ResponsiveContainer>
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
  const data = useMemo(() => hours.map((hour) => {
    const row: Record<string, string | number> = { label: hour.label, total: Number((hour.total_seconds / 3600).toFixed(2)) };
    const itemsByName = new Map((hour[mode] ?? []).map((entry) => [entry.name, entry]));
    for (const key of keys) {
      const item = itemsByName.get(key);
      row[key] = item ? Number((item.total_seconds / 3600).toFixed(2)) : 0;
    }
    return row;
  }), [hours, keys, mode]);
  return (
    <div className={`${chartPanelClass} h-72`}>
      <div className="mb-4 flex items-center justify-between gap-3">
        <div className="text-sm font-medium text-zinc-300">{mode === "projects" ? "Project timeline" : "Language timeline"}</div>
        <div className="text-xs text-zinc-500">24-hour UTC</div>
      </div>
      <ResponsiveContainer width="100%" height="82%">
        <BarChart data={data}>
          <CartesianGrid stroke="#26262b" vertical={false} />
          <XAxis dataKey="label" stroke="#888" tick={smallChartTick} interval={2} />
          <YAxis stroke="#888" tick={chartTick} />
          <Tooltip contentStyle={tooltipStyle} />
          {keys.map((key, index) => (
            <Bar key={key} dataKey={key} stackId={mode} fill={colorForLanguage(key, colors, index)} radius={index === keys.length - 1 ? [4, 4, 0, 0] : [0, 0, 0, 0]} />
          ))}
        </BarChart>
      </ResponsiveContainer>
      {keys.length === 0 ? <div className="-mt-40 text-center text-sm text-zinc-500">No hourly activity in this range.</div> : null}
    </div>
  );
}

export function AIHumanByDay({ days, title = "AI vs Human by Day" }: { days: AIStat[]; title?: string }) {
  const data = useMemo(() => days.map((day) => ({
    name: day.name.slice(5),
    ai: day.ai_line_changes,
    human: day.human_line_changes,
    tokens: day.ai_input_tokens + day.ai_output_tokens,
    sessions: day.session_count
  })), [days]);
  const hasRows = data.some((day) => day.ai > 0 || day.human > 0 || day.tokens > 0);
  return (
    <div className={`${chartPanelClass} h-72`}>
      <div className="mb-4 flex items-center justify-between gap-3">
        <div className="text-sm font-medium text-zinc-300">{title}</div>
        <div className="flex gap-3 text-xs text-zinc-500">
          <span className="inline-flex items-center gap-1.5"><span className="h-2.5 w-2.5 rounded-sm bg-accent" /> AI</span>
          <span className="inline-flex items-center gap-1.5"><span className="h-2.5 w-2.5 rounded-sm bg-lime-400" /> Human</span>
        </div>
      </div>
      <ResponsiveContainer width="100%" height="82%">
        <BarChart data={data}>
          <CartesianGrid stroke="#26262b" vertical={false} />
          <XAxis dataKey="name" stroke="#888" tick={chartTick} />
          <YAxis stroke="#888" tick={chartTick} />
          <Tooltip
            contentStyle={tooltipStyle}
            formatter={(value, name) => [Number(value).toLocaleString(), name === "ai" ? "AI changes" : "Human changes"]}
          />
          <Bar dataKey="ai" stackId="lines" fill={fallbackPalette[0]} radius={[4, 4, 0, 0]} />
          <Bar dataKey="human" stackId="lines" fill={fallbackPalette[1]} radius={[4, 4, 0, 0]} />
        </BarChart>
      </ResponsiveContainer>
      {!hasRows ? <div className="-mt-40 text-center text-sm text-zinc-500">No AI or human line-change data in this range.</div> : null}
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

function topTimelineKeys(hours: HourlyStat[], mode: "projects" | "languages") {
  const totals = new Map<string, number>();
  for (const hour of hours) {
    for (const row of hour[mode] ?? []) {
      totals.set(row.name, (totals.get(row.name) ?? 0) + row.total_seconds);
    }
  }
  return [...totals.entries()]
    .sort((a, b) => {
      if (a[1] === b[1]) {
        return a[0].localeCompare(b[0]);
      }
      return b[1] - a[1];
    })
    .slice(0, 5)
    .map(([name]) => name);
}

function topProjectKeys(days: DailyStat[]) {
  const totals = new Map<string, number>();
  for (const day of days) {
    for (const project of day.projects ?? []) {
      totals.set(project.name, (totals.get(project.name) ?? 0) + project.total_seconds);
    }
  }
  return [...totals.entries()]
    .sort((a, b) => {
      if (a[1] === b[1]) {
        return a[0].localeCompare(b[0]);
      }
      return b[1] - a[1];
    })
    .slice(0, 5)
    .map(([name]) => name);
}

function barPercent(value: number, total: number) {
  if (total <= 0 || value <= 0) {
    return 0;
  }
  return Math.max(3, Math.round((value / total) * 100));
}

function weekdayRows(days: DailyStat[]) {
  const labels = [
    { name: "Sunday", short: "Sun" },
    { name: "Monday", short: "Mon" },
    { name: "Tuesday", short: "Tue" },
    { name: "Wednesday", short: "Wed" },
    { name: "Thursday", short: "Thu" },
    { name: "Friday", short: "Fri" },
    { name: "Saturday", short: "Sat" }
  ];
  const totals = labels.map((label) => ({ ...label, totalSeconds: 0, activeDays: 0, averageSeconds: 0 }));
  for (const day of days) {
    const date = new Date(`${day.date}T00:00:00Z`);
    const index = date.getUTCDay();
    totals[index].totalSeconds += day.total_seconds;
    if (day.total_seconds > 0) {
      totals[index].activeDays += 1;
    }
  }
  return totals.map((row) => ({
    ...row,
    averageSeconds: row.activeDays > 0 ? Math.round(row.totalSeconds / row.activeDays) : 0
  }));
}

function formatShortDuration(seconds: number) {
  if (seconds <= 0) {
    return "0s";
  }
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) {
    return `${minutes}m`;
  }
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return remainingMinutes > 0 ? `${hours}h ${remainingMinutes}m` : `${hours}h`;
}
