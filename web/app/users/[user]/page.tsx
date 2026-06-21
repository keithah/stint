"use client";

import { useQuery } from "@tanstack/react-query";
import { Activity, BarChart3, CalendarDays, Github, UserRound } from "lucide-react";
import { useParams } from "next/navigation";
import { useMemo, useState } from "react";
import { AIPanel } from "@/components/ai-panel";
import { ActivityBars, SliceDonut } from "@/components/dashboard-charts";
import { Providers } from "@/components/providers";
import { StatCard } from "@/components/stat-card";
import { listProgramLanguages, publicUserProfile, publicUserStats, publicUserSummaries, type StatsRange } from "@/lib/api";
import { languageColorMap } from "@/lib/language-colors";

const today = new Date().toISOString().slice(0, 10);
const weekAgo = new Date(Date.now() - 6 * 24 * 60 * 60 * 1000).toISOString().slice(0, 10);
const ranges: Array<{ value: StatsRange; label: string }> = [
  { value: "last_7_days", label: "7 days" },
  { value: "last_30_days", label: "30 days" },
  { value: "last_6_months", label: "6 months" },
  { value: "last_year", label: "Year" },
  { value: "all_time", label: "All time" }
];

export default function PublicUserPage() {
  return (
    <Providers>
      <PublicUserContent />
    </Providers>
  );
}

function PublicUserContent() {
  const params = useParams<{ user: string }>();
  const username = params.user;
  const [range, setRange] = useState<StatsRange>("last_7_days");
  const [startDate, setStartDate] = useState(weekAgo);
  const [endDate, setEndDate] = useState(today);
  const profile = useQuery({ queryKey: ["public-user", username], queryFn: () => publicUserProfile(username), retry: false, enabled: Boolean(username) });
  const stats = useQuery({ queryKey: ["public-user-stats", username, range], queryFn: () => publicUserStats(username, range), retry: false, enabled: Boolean(username) });
  const summaries = useQuery({ queryKey: ["public-user-summaries", username, startDate, endDate], queryFn: () => publicUserSummaries(username, startDate, endDate), retry: false, enabled: Boolean(username) && Boolean(profile.data?.data.permissions.summaries ?? stats.data?.user.permissions.summaries) });
  const programLanguages = useQuery({ queryKey: ["program-languages"], queryFn: listProgramLanguages, retry: false, staleTime: 3600000 });
  const data = stats.data?.data;
  const publicUser = profile.data?.data ?? stats.data?.user;
  const permissions = publicUser?.permissions ?? {
    total_time: true,
    projects: true,
    project_visibility: "public_repos",
    languages: true,
    editors: false,
    machines: false,
    operating_systems: false,
    categories: false,
    ai: false,
    summaries: true,
    github: false
  };
  const summaryRows = summaries.data?.data ?? [];
  const summaryTotalSeconds = summaryRows.reduce((sum, day) => sum + day.grand_total.total_seconds, 0);
  const languageColors = useMemo(() => languageColorMap(programLanguages.data?.data ?? []), [programLanguages.data?.data]);

  if (profile.isError || stats.isError) {
    const error = profile.error ?? stats.error;
    return (
      <main className="grid min-h-screen place-items-center px-6">
        <section className="max-w-md rounded border border-line bg-panel p-6">
          <h1 className="text-xl font-semibold">Public profile unavailable</h1>
          <p className="mt-2 text-sm text-zinc-400">{error?.message ?? "This user has not enabled a public profile."}</p>
        </section>
      </main>
    );
  }

  return (
    <main className="min-h-screen">
      <div className="mx-auto max-w-6xl px-5 py-8 lg:px-8">
        <header className="mb-8 border-b border-line pb-6">
          <div className="mb-3 inline-flex items-center gap-2 rounded border border-accent/30 bg-accent/10 px-3 py-1 text-xs uppercase tracking-[0.18em] text-accent">
            <UserRound size={14} /> Public profile
          </div>
          <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-end">
            <div>
              <h1 className="text-4xl font-semibold tracking-tight">{publicUser?.name || publicUser?.username || username}</h1>
              <p className="mt-2 text-sm text-zinc-400">@{publicUser?.username || username} · Opt-in public coding activity.</p>
              {publicUser?.github_url ? (
                <a className="mt-3 inline-flex items-center gap-2 text-sm text-accent hover:text-cyan-200" href={publicUser.github_url} target="_blank" rel="noreferrer">
                  <Github size={15} /> GitHub @{publicUser.github_username}
                </a>
              ) : null}
            </div>
            <div className="flex flex-wrap gap-2">
              {ranges.map((item) => (
                <button
                  key={item.value}
                  className={`rounded border px-3 py-2 text-sm ${range === item.value ? "border-accent bg-accent text-ink" : "border-line text-zinc-300 hover:bg-white/5"}`}
                  onClick={() => setRange(item.value)}
                >
                  {item.label}
                </button>
              ))}
            </div>
          </div>
        </header>

        {permissions.total_time ? (
          <section className="grid gap-4 md:grid-cols-3">
            <StatCard label="Total" value={data?.human_readable_total ?? "0 secs"} detail={rangeLabel(range)} />
            <StatCard label="Daily average" value={data?.human_readable_daily_average ?? "0 secs"} detail={`${data?.days.length ?? 0} calendar days`} />
            <StatCard label="Best day" value={data?.best_day.text ?? "0 secs"} detail={data?.best_day.date ?? "No activity yet"} />
          </section>
        ) : null}

        {(permissions.total_time || permissions.projects || permissions.languages) ? (
          <section className="mt-5 grid gap-5 xl:grid-cols-[1.4fr_1fr]">
            {permissions.total_time ? <ActivityBars days={data?.days ?? []} title="Public activity" /> : null}
            <div className="grid gap-5">
              {permissions.projects ? <SliceDonut title="Projects" rows={data?.projects ?? []} /> : null}
              {permissions.languages ? <SliceDonut title="Languages" rows={data?.languages ?? []} colors={languageColors} /> : null}
            </div>
          </section>
        ) : null}

        {permissions.ai ? (
          <section className="mt-5">
            <AIPanel metrics={data?.ai} />
          </section>
        ) : null}

        {(permissions.editors || permissions.machines || permissions.categories) ? (
          <section className="mt-5 grid gap-5 lg:grid-cols-3">
            {permissions.editors ? <SliceDonut title="Editors" rows={data?.editors ?? []} /> : null}
            {permissions.machines ? <SliceDonut title="Machines" rows={data?.machines ?? []} /> : null}
            {permissions.categories ? <SliceDonut title="Categories" rows={data?.categories ?? []} /> : null}
          </section>
        ) : null}

        {permissions.summaries ? (
          <section className="mt-5 rounded border border-line bg-panel p-5">
          <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-end">
            <div>
              <div className="mb-3 inline-flex items-center gap-2 text-xs uppercase tracking-[0.16em] text-zinc-500">
                <CalendarDays size={14} /> Summaries
              </div>
              <h2 className="font-medium">Public date range</h2>
              <p className="mt-1 text-sm text-zinc-400">
                {formatDuration(summaryTotalSeconds)} across {summaryRows.length} days
              </p>
            </div>
            <div className="grid gap-2 sm:grid-cols-[150px_150px]">
              <input className="rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" type="date" value={startDate} onChange={(event) => setStartDate(event.target.value)} />
              <input className="rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" type="date" value={endDate} onChange={(event) => setEndDate(event.target.value)} />
            </div>
          </div>
          <div className="mt-4 overflow-hidden rounded border border-line">
            <div className="grid grid-cols-[140px_1fr_1fr] gap-3 border-b border-line bg-ink px-3 py-2 text-xs uppercase tracking-[0.16em] text-zinc-500">
              <span>Date</span>
              <span>Total</span>
              <span>Top language</span>
            </div>
            {summaryRows.slice(0, 14).map((day) => (
              <div key={day.range.date} className="grid grid-cols-[140px_1fr_1fr] gap-3 border-b border-line px-3 py-3 text-sm last:border-b-0">
                <span className="text-zinc-300">{day.range.date}</span>
                <span className="text-zinc-400">{day.grand_total.text}</span>
                <span className="truncate text-zinc-500">{day.languages?.[0]?.name ?? "No language"}</span>
              </div>
            ))}
            {!summaries.isLoading && summaryRows.length === 0 ? <div className="p-3 text-sm text-zinc-500">No public summaries for this range.</div> : null}
          </div>
          </section>
        ) : null}

        <footer className="mt-8 flex items-center gap-2 border-t border-line pt-5 text-sm text-zinc-500">
          <BarChart3 size={16} />
          <span>Public data is controlled by the user&apos;s profile visibility setting.</span>
          <span className="ml-auto inline-flex items-center gap-2"><Activity size={16} /> Powered by Stint</span>
        </footer>
      </div>
    </main>
  );
}

function rangeLabel(range: StatsRange) {
  const match = ranges.find((item) => item.value === range);
  return match?.label ?? range;
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
