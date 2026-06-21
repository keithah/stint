"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BarChart3, CalendarDays, Download, FileDown, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { Providers } from "@/components/providers";
import { Shell } from "@/components/shell";
import { createDataDump, createExternalDuration, dataDumpDownloadURL, deleteExternalDurationsBulk, deleteHeartbeats, durationsForDay, heartbeatsForDay, listDataDumps, listExternalDurations, summaries, type DurationSlice } from "@/lib/api";
import { dataDumpExpiryText, dataDumpIsDownloadable, hasPendingDumps } from "@/lib/data-dumps";

const today = new Date().toISOString().slice(0, 10);
const weekAgo = new Date(Date.now() - 6 * 24 * 60 * 60 * 1000).toISOString().slice(0, 10);
const durationSlices: Array<{ value: DurationSlice; label: string }> = [
  { value: "project", label: "Project" },
  { value: "language", label: "Language" },
  { value: "editor", label: "Editor" },
  { value: "operating_system", label: "OS" },
  { value: "machine", label: "Machine" },
  { value: "category", label: "Category" },
  { value: "branch", label: "Branch" },
  { value: "dependencies", label: "Dependency" }
];

export default function ReportsPage() {
  return (
    <Providers>
      <Shell>
        <ReportsContent />
      </Shell>
    </Providers>
  );
}

function ReportsContent() {
  const client = useQueryClient();
  const [dumpType, setDumpType] = useState<"heartbeats" | "daily">("heartbeats");
  const [startDate, setStartDate] = useState(weekAgo);
  const [endDate, setEndDate] = useState(today);
  const [externalEntity, setExternalEntity] = useState("Planning");
  const [externalProject, setExternalProject] = useState("manual");
  const [externalLanguage, setExternalLanguage] = useState("Markdown");
  const [externalMinutes, setExternalMinutes] = useState(30);
  const [selectedExternalDurationIDs, setSelectedExternalDurationIDs] = useState<string[]>([]);
  const [heartbeatDate, setHeartbeatDate] = useState(today);
  const [selectedHeartbeatIDs, setSelectedHeartbeatIDs] = useState<string[]>([]);
  const [durationDate, setDurationDate] = useState(today);
  const [durationSlice, setDurationSlice] = useState<DurationSlice>("project");
  const dumps = useQuery({
    queryKey: ["data-dumps"],
    queryFn: listDataDumps,
    retry: false,
    refetchInterval: (query) => (hasPendingDumps(query.state.data) ? 2000 : false)
  });
  const external = useQuery({ queryKey: ["external-durations"], queryFn: listExternalDurations, retry: false });
  const report = useQuery({ queryKey: ["summaries", startDate, endDate], queryFn: () => summaries(startDate, endDate), retry: false });
  const heartbeats = useQuery({ queryKey: ["heartbeats", heartbeatDate], queryFn: () => heartbeatsForDay(heartbeatDate), retry: false });
  const durations = useQuery({ queryKey: ["durations", durationDate, durationSlice], queryFn: () => durationsForDay(durationDate, durationSlice), retry: false });
  const createDump = useMutation({
    mutationFn: () => createDataDump(dumpType),
    onSuccess: () => client.invalidateQueries({ queryKey: ["data-dumps"] })
  });
  const canCreateExternalDuration = externalEntity.trim().length > 0 && Number.isFinite(externalMinutes) && externalMinutes > 0;
  const createExternal = useMutation({
    mutationFn: () => {
      const now = Math.floor(Date.now() / 1000);
      return createExternalDuration({
        external_id: `manual-${now}`,
        provider: "manual",
        entity: externalEntity.trim(),
        type: "app",
        category: "planning",
        start_time: now - Math.max(1, externalMinutes) * 60,
        end_time: now,
        project: externalProject.trim() || undefined,
        language: externalLanguage.trim() || undefined
      });
    },
    onSuccess: () => client.invalidateQueries({ queryKey: ["external-durations"] })
  });
  const deleteSelectedHeartbeats = useMutation({
    mutationFn: () => deleteHeartbeats(heartbeatDate, selectedHeartbeatIDs),
    onSuccess: () => {
      setSelectedHeartbeatIDs([]);
      client.invalidateQueries({ queryKey: ["heartbeats", heartbeatDate] });
    }
  });
  const deleteSelectedExternalDurations = useMutation({
    mutationFn: () => deleteExternalDurationsBulk(selectedExternalDurationIDs),
    onSuccess: () => {
      setSelectedExternalDurationIDs([]);
      client.invalidateQueries({ queryKey: ["external-durations"] });
    }
  });
  const reportRows = report.data?.data ?? [];
  const totalSeconds = reportRows.reduce((sum, day) => sum + day.grand_total.total_seconds, 0);
  const heartbeatRows = heartbeats.data?.data ?? [];
  const durationRows = durations.data?.data ?? [];
  const durationTotalSeconds = durationRows.reduce((sum, row) => sum + row.duration, 0);
  const externalRows = external.data?.data ?? [];

  return (
    <div className="mx-auto max-w-6xl px-5 py-6 lg:px-8">
      <header className="mb-8 border-b border-line pb-6">
        <div className="mb-3 inline-flex items-center gap-2 rounded border border-accent/30 bg-accent/10 px-3 py-1 text-xs uppercase tracking-[0.18em] text-accent">
          <FileDown size={14} /> Exports and external time
        </div>
        <h1 className="text-4xl font-semibold tracking-tight">Reports</h1>
        <p className="mt-2 text-sm text-zinc-400">Generate data dumps and track manually supplied external durations.</p>
      </header>

      <section className="mb-5 rounded border border-line bg-panel p-5">
        <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-end">
          <div>
            <div className="mb-3 inline-flex items-center gap-2 text-xs uppercase tracking-[0.16em] text-zinc-500">
              <CalendarDays size={14} /> Custom range
            </div>
            <h2 className="font-medium">Date range report</h2>
            <p className="mt-1 text-sm text-zinc-400">
              {formatDuration(totalSeconds)} across {reportRows.length} days
            </p>
          </div>
          <div className="grid gap-2 sm:grid-cols-[150px_150px_auto_auto]">
            <input className="rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" type="date" value={startDate} onChange={(event) => setStartDate(event.target.value)} />
            <input className="rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" type="date" value={endDate} onChange={(event) => setEndDate(event.target.value)} />
            <button className="inline-flex items-center justify-center gap-2 rounded border border-line px-4 py-2 text-sm text-zinc-300 hover:bg-white/5" onClick={() => downloadReportJSON(reportRows, startDate, endDate)} disabled={!reportRows.length}>
              <Download size={16} /> JSON
            </button>
            <button className="inline-flex items-center justify-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60" onClick={() => downloadReportCSV(reportRows, startDate, endDate)} disabled={!reportRows.length}>
              <Download size={16} /> CSV
            </button>
          </div>
        </div>
        <div className="mt-4 overflow-hidden rounded border border-line">
          <div className="grid grid-cols-[140px_1fr_1fr] gap-3 border-b border-line bg-ink px-3 py-2 text-xs uppercase tracking-[0.16em] text-zinc-500">
            <span>Date</span>
            <span>Total</span>
            <span>Top project</span>
          </div>
          {reportRows.slice(0, 14).map((day) => (
            <div key={day.range.date} className="grid grid-cols-[140px_1fr_1fr] gap-3 border-b border-line px-3 py-3 text-sm last:border-b-0">
              <span className="text-zinc-300">{day.range.date}</span>
              <span className="text-zinc-400">{day.grand_total.text}</span>
              <span className="truncate text-zinc-500">{day.projects?.[0]?.name ?? "No project"}</span>
            </div>
          ))}
          {reportRows.length === 0 ? <div className="p-3 text-sm text-zinc-500">No report rows for this range.</div> : null}
        </div>
      </section>

      <section className="mb-5 rounded border border-line bg-panel p-5">
        <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-end">
          <div>
            <div className="mb-3 inline-flex items-center gap-2 text-xs uppercase tracking-[0.16em] text-zinc-500">
              <BarChart3 size={14} /> Merged activity
            </div>
            <h2 className="font-medium">Duration breakdown</h2>
            <p className="mt-1 text-sm text-zinc-400">
              {formatDuration(durationTotalSeconds)} from merged heartbeat and external-duration rows.
            </p>
          </div>
          <div className="grid gap-2 sm:grid-cols-[150px_180px]">
            <input className="rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" type="date" value={durationDate} onChange={(event) => setDurationDate(event.target.value)} />
            <select className="rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={durationSlice} onChange={(event) => setDurationSlice(event.target.value as DurationSlice)}>
              {durationSlices.map((slice) => (
                <option key={slice.value} value={slice.value}>{slice.label}</option>
              ))}
            </select>
          </div>
        </div>
        <div className="mt-4 overflow-hidden rounded border border-line">
          <div className="grid grid-cols-[120px_1fr_1fr] gap-3 border-b border-line bg-ink px-3 py-2 text-xs uppercase tracking-[0.16em] text-zinc-500">
            <span>Start</span>
            <span>Name</span>
            <span>Duration</span>
          </div>
          {durationRows.slice(0, 24).map((row, index) => (
            <div key={`${row.name}-${row.time}-${index}`} className="grid grid-cols-[120px_1fr_1fr] gap-3 border-b border-line px-3 py-3 text-sm last:border-b-0">
              <span className="text-zinc-400">{formatHeartbeatTime(row.time)}</span>
              <span className="truncate text-zinc-200">{row.name || "Unknown"}</span>
              <span className="text-zinc-500">{formatDuration(row.duration)}</span>
            </div>
          ))}
          {durations.isLoading ? <div className="p-3 text-sm text-zinc-500">Loading durations...</div> : null}
          {!durations.isLoading && durationRows.length === 0 ? <div className="p-3 text-sm text-zinc-500">No durations for this day.</div> : null}
        </div>
      </section>

      <section className="mb-5 rounded border border-line bg-panel p-5">
        <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-end">
          <div>
            <div className="mb-3 inline-flex items-center gap-2 text-xs uppercase tracking-[0.16em] text-zinc-500">
              <CalendarDays size={14} /> Raw events
            </div>
            <h2 className="font-medium">Raw heartbeats</h2>
            <p className="mt-1 text-sm text-zinc-400">
              Inspect stored heartbeats for a single day and delete accidental events before they affect reports.
            </p>
          </div>
          <div className="grid gap-2 sm:grid-cols-[150px_auto]">
            <input
              className="rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent"
              type="date"
              value={heartbeatDate}
              onChange={(event) => {
                setHeartbeatDate(event.target.value);
                setSelectedHeartbeatIDs([]);
              }}
            />
            <button
              className="inline-flex items-center justify-center gap-2 rounded border border-red-400/40 px-4 py-2 text-sm text-red-200 hover:bg-red-500/10 disabled:opacity-50"
              onClick={() => deleteSelectedHeartbeats.mutate()}
              disabled={!selectedHeartbeatIDs.length || deleteSelectedHeartbeats.isPending}
            >
              <Trash2 size={16} /> Delete selected
            </button>
          </div>
        </div>
        <div className="mt-4 overflow-hidden rounded border border-line">
          <div className="grid grid-cols-[36px_120px_1.2fr_1fr_1fr_1fr] gap-3 border-b border-line bg-ink px-3 py-2 text-xs uppercase tracking-[0.16em] text-zinc-500">
            <span />
            <span>Time</span>
            <span>Entity</span>
            <span>Project</span>
            <span>Language</span>
            <span>Editor</span>
          </div>
          {heartbeatRows.slice(0, 50).map((heartbeat) => (
            <label key={heartbeat.id} className="grid cursor-pointer grid-cols-[36px_120px_1.2fr_1fr_1fr_1fr] gap-3 border-b border-line px-3 py-3 text-sm last:border-b-0 hover:bg-white/5">
              <input
                className="mt-1 h-4 w-4 accent-accent"
                type="checkbox"
                checked={selectedHeartbeatIDs.includes(heartbeat.id)}
                onChange={(event) => {
                  setSelectedHeartbeatIDs((current) =>
                    event.target.checked ? [...current, heartbeat.id] : current.filter((id) => id !== heartbeat.id)
                  );
                }}
              />
              <span className="text-zinc-400">{formatHeartbeatTime(heartbeat.time)}</span>
              <span className="truncate text-zinc-200" title={heartbeat.entity}>{heartbeat.entity}</span>
              <span className="truncate text-zinc-500">{heartbeat.project ?? "No project"}</span>
              <span className="truncate text-zinc-500">{heartbeat.language ?? "Unknown"}</span>
              <span className="truncate text-zinc-500">{heartbeat.editor ?? "Unknown"}</span>
            </label>
          ))}
          {heartbeats.isLoading ? <div className="p-3 text-sm text-zinc-500">Loading heartbeats...</div> : null}
          {!heartbeats.isLoading && heartbeatRows.length === 0 ? <div className="p-3 text-sm text-zinc-500">No heartbeats stored for this day.</div> : null}
        </div>
        {deleteSelectedHeartbeats.error ? <p className="mt-3 text-sm text-red-300">{deleteSelectedHeartbeats.error.message}</p> : null}
      </section>

      <section className="grid gap-5 lg:grid-cols-2">
        <div className="rounded border border-line bg-panel p-5">
          <h2 className="font-medium">Data dumps</h2>
          <div className="mt-4 flex gap-2">
            <select className="rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={dumpType} onChange={(event) => setDumpType(event.target.value as "heartbeats" | "daily")}>
              <option value="heartbeats">Heartbeats</option>
              <option value="daily">Daily summaries</option>
            </select>
            <button className="inline-flex items-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60" onClick={() => createDump.mutate()} disabled={createDump.isPending}>
              <Plus size={16} /> Generate
            </button>
          </div>
          <div className="mt-4 divide-y divide-line rounded border border-line">
            {(dumps.data?.data ?? []).map((dump) => {
              const isReady = dataDumpIsDownloadable(dump);
              const expiryText = dataDumpExpiryText(dump);
              return (
                <a
                  key={dump.id}
                  className={`block px-3 py-3 text-sm ${isReady ? "hover:bg-white/5" : "cursor-not-allowed opacity-60"}`}
                  href={isReady ? dataDumpDownloadURL(dump.download_url) : "#"}
                  aria-disabled={!isReady}
                  onClick={(event) => {
                    if (!isReady) {
                      event.preventDefault();
                    }
                  }}
                >
                  <span className="font-medium text-zinc-100">{dump.type}</span>
                  <span className="ml-2 text-zinc-500">{dump.status}</span>
                  {expiryText ? <span className="ml-2 text-zinc-600">{expiryText}</span> : null}
                  {!isReady && !expiryText ? <span className="ml-2 text-zinc-600">{dump.percent_complete}%</span> : null}
                </a>
              );
            })}
            {dumps.data?.data.length === 0 ? <div className="p-3 text-sm text-zinc-500">No dumps generated yet.</div> : null}
          </div>
        </div>

        <div className="rounded border border-line bg-panel p-5">
          <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
            <div>
              <h2 className="font-medium">External durations</h2>
              <p className="mt-1 text-sm text-zinc-500">Manual and imported time entries that merge into summaries.</p>
            </div>
            <button
              className="inline-flex items-center justify-center gap-2 rounded border border-red-400/40 px-3 py-2 text-sm text-red-200 hover:bg-red-500/10 disabled:opacity-50"
              onClick={() => deleteSelectedExternalDurations.mutate()}
              disabled={!selectedExternalDurationIDs.length || deleteSelectedExternalDurations.isPending}
            >
              <Trash2 size={16} /> Delete selected durations
            </button>
          </div>
          <div className="mt-4 grid gap-3 sm:grid-cols-2">
            <input className="rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={externalEntity} onChange={(event) => setExternalEntity(event.target.value)} />
            <input className="rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={externalProject} onChange={(event) => setExternalProject(event.target.value)} />
            <input className="rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={externalLanguage} onChange={(event) => setExternalLanguage(event.target.value)} />
            <input className="rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" type="number" min={1} step={5} value={externalMinutes} onChange={(event) => setExternalMinutes(Math.max(1, Number(event.target.value) || 1))} />
          </div>
          <button className="mt-3 inline-flex items-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60" onClick={() => createExternal.mutate()} disabled={createExternal.isPending || !canCreateExternalDuration}>
            <Plus size={16} /> Add duration
          </button>
          <div className="mt-4 divide-y divide-line rounded border border-line">
            {externalRows.slice(0, 8).map((duration) => (
              <label key={duration.id} className="grid cursor-pointer grid-cols-[28px_1fr] gap-3 px-3 py-3 text-sm hover:bg-white/5">
                <input
                  className="mt-1 h-4 w-4 accent-accent"
                  type="checkbox"
                  checked={selectedExternalDurationIDs.includes(duration.id)}
                  onChange={(event) => {
                    setSelectedExternalDurationIDs((current) =>
                      event.target.checked ? [...current, duration.id] : current.filter((id) => id !== duration.id)
                    );
                  }}
                />
                <span className="min-w-0">
                  <span className="block truncate font-medium text-zinc-100" title={duration.entity}>{duration.entity}</span>
                  <span className="mt-1 block truncate text-zinc-500">
                    {duration.project ?? "Unknown"} · {Math.max(0, Math.round((duration.end_time - duration.start_time) / 60))} mins
                  </span>
                </span>
              </label>
            ))}
            {externalRows.length === 0 ? <div className="p-3 text-sm text-zinc-500">No external durations yet.</div> : null}
          </div>
          {deleteSelectedExternalDurations.error ? <p className="mt-3 text-sm text-red-300">{deleteSelectedExternalDurations.error.message}</p> : null}
        </div>
      </section>
    </div>
  );
}

function formatDuration(seconds: number) {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours > 0) {
    return `${hours} hrs ${minutes} mins`;
  }
  if (minutes > 0) {
    return `${minutes} mins`;
  }
  return `${seconds} secs`;
}

function formatHeartbeatTime(seconds: number) {
  return new Date(seconds * 1000).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function downloadReportJSON(rows: Awaited<ReturnType<typeof summaries>>["data"], start: string, end: string) {
  downloadText(`stint-report-${start}-${end}.json`, JSON.stringify(rows, null, 2), "application/json");
}

function downloadReportCSV(rows: Awaited<ReturnType<typeof summaries>>["data"], start: string, end: string) {
  const header = "date,total_seconds,total_text,top_project,top_language";
  const lines = rows.map((day) =>
    [day.range.date, day.grand_total.total_seconds, day.grand_total.text, day.projects?.[0]?.name ?? "", day.languages?.[0]?.name ?? ""].map(csvCell).join(",")
  );
  downloadText(`stint-report-${start}-${end}.csv`, [header, ...lines].join("\n"), "text/csv");
}

function csvCell(value: string | number) {
  const text = String(value);
  if (!/[",\n]/.test(text)) {
    return text;
  }
  return `"${text.replace(/"/g, '""')}"`;
}

function downloadText(filename: string, body: string, type: string) {
  const blob = new Blob([body], { type });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(url);
}
