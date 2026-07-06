"use client";

import { useQuery } from "@tanstack/react-query";
import dynamic from "next/dynamic";
import Link from "next/link";
import { Activity, ArrowRight, Bot, Check, Coins, Copy, GitPullRequestArrow, KeyRound, Monitor, RefreshCw, Sparkles, X } from "lucide-react";
import type { ReactNode } from "react";
import { useMemo, useState, useSyncExternalStore } from "react";
import { StatCard } from "@/components/stat-card";
import { noopSubscribe, serverWakaTimeAPIURL } from "@/components/settings/shared";
import { AuthGate, EmptyState, HeroHeader, SecondaryButton, SecondaryLink, SegmentedToggle, Skeleton, pillWrapperClass } from "@/components/ui";
import { allTimeSinceToday, listProgramLanguages, me, statsForRange, statusBarToday, type AIStat, type Stats, type StatsRange, wakatimeAPIURL } from "@/lib/api";
import { languageColorMap } from "@/lib/language-colors";
import { rangeOptions } from "@/lib/ranges";
import { compactNumber, formatCents } from "@/lib/number-format";
import { ONBOARDING_STORAGE_KEY, shouldShowOnboarding } from "@/lib/onboarding-state";

const AIPanel = dynamic(() => import("@/components/ai-panel").then((module) => module.AIPanel), { ssr: false, loading: ChartSkeleton });
const ActivityBars = dynamic(() => import("@/components/dashboard-charts").then((module) => module.ActivityBars), { ssr: false, loading: ChartSkeleton });
const AIHumanByDay = dynamic(() => import("@/components/dashboard-charts").then((module) => module.AIHumanByDay), { ssr: false, loading: ChartSkeleton });
const HourlyTimeline = dynamic(() => import("@/components/dashboard-charts").then((module) => module.HourlyTimeline), { ssr: false, loading: ChartSkeleton });
const ProjectStackedArea = dynamic(() => import("@/components/dashboard-charts").then((module) => module.ProjectStackedArea), { ssr: false, loading: ChartSkeleton });
const SliceBars = dynamic(() => import("@/components/dashboard-charts").then((module) => module.SliceBars), { ssr: false, loading: ChartSkeleton });
const SliceDonut = dynamic(() => import("@/components/dashboard-charts").then((module) => module.SliceDonut), { ssr: false, loading: ChartSkeleton });
const WeekdayHeatmap = dynamic(() => import("@/components/dashboard-charts").then((module) => module.WeekdayHeatmap), { ssr: false, loading: ChartSkeleton });

function ChartSkeleton() {
  return <Skeleton className="h-[260px]" />;
}

export default function DashboardPage() {
  return (
    <DashboardContent />
  );
}

function DashboardContent() {
	const [range, setRange] = useState<StatsRange>("last_7_days");
	const [onboardingDismissed, setOnboardingDismissed] = useStoredBoolean(ONBOARDING_STORAGE_KEY);
	const user = useQuery({ queryKey: ["me"], queryFn: me, });
	const stats = useQuery({ queryKey: ["stats", range], queryFn: () => statsForRange(range), staleTime: 120000, refetchOnWindowFocus: true });
	const aiTrend = useQuery({ queryKey: ["stats", "last_30_days"], queryFn: () => statsForRange("last_30_days"), staleTime: 120000, refetchOnWindowFocus: true });
	const status = useQuery({ queryKey: ["status-bar-today"], queryFn: statusBarToday, staleTime: 120000, refetchOnWindowFocus: true });
	const allTime = useQuery({ queryKey: ["all-time"], queryFn: allTimeSinceToday, staleTime: 120000, refetchOnWindowFocus: true });
	const programLanguages = useQuery({ queryKey: ["program-languages"], queryFn: listProgramLanguages, staleTime: 3600000 });
	const apiURL = useSyncExternalStore(noopSubscribe, wakatimeAPIURL, serverWakaTimeAPIURL);
	const data = stats.data?.data;
	const languageColors = useMemo(() => languageColorMap(programLanguages.data?.data ?? []), [programLanguages.data?.data]);
	const activeRange = rangeOptions.find((item) => item.value === range) ?? rangeOptions[0];
	const onboardingConfig = useMemo(
		() => `[settings]\napi_url = ${apiURL}\napi_key = stint_00000000-0000-4000-8000-000000000000\nhide_file_names = false\ntimeout = ${user.data?.data.timeout_minutes ?? 15}`,
		[apiURL, user.data?.data.timeout_minutes]
	);
	const showOnboarding = Boolean(user.data?.data) && stats.isSuccess && shouldShowOnboarding(data?.total_seconds, onboardingDismissed);

	if (user.isError) {
    return <AuthGate message="Create a session before viewing activity." />;
  }

	return (
		<div className="mx-auto max-w-7xl px-5 py-6 lg:px-8">
			{showOnboarding ? (
				<OnboardingModal
					configBlock={onboardingConfig}
					onDismiss={() => setOnboardingDismissed(true)}
				/>
			) : null}
			<HeroHeader
				className="ops-dashboard-header"
				caption={`Coding activity · ${activeRange.label}`}
				value={data?.human_readable_total ?? "0 secs"}
				freshness={freshnessLabel(data)}
				freshnessActive={Boolean(data) && !data?.is_up_to_date}
				subline={
					<>
						{status.data?.data.grand_total_text ?? "0 secs"} today · averaging {data?.human_readable_daily_average ?? "0 secs"} a day · @{user.data?.data.github_username ?? "local"}
					</>
				}
				controls={
					<>
						<SegmentedToggle options={rangeOptions} value={range} onChange={setRange} variant="pill" className={pillWrapperClass} />
						<div className="flex gap-2">
							<SecondaryButton onClick={() => { stats.refetch(); status.refetch(); }}>
								<RefreshCw size={15} /> Refresh
							</SecondaryButton>
							<SecondaryLink href="/settings">
								Setup <ArrowRight size={15} />
							</SecondaryLink>
						</div>
					</>
				}
			/>

      {stats.isLoading || status.isLoading || allTime.isLoading ? (
        <section className="grid gap-4 md:grid-cols-5" aria-busy="true" aria-label="Loading activity totals">
          {Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-[88px]" />)}
        </section>
      ) : (
        <section className="grid gap-4 md:grid-cols-5">
          <StatCard label={activeRange.label} value={data?.human_readable_total ?? "0 secs"} detail={user.data?.data.github_username ?? "Waiting for session"} />
          <StatCard label="Today" value={status.data?.data.grand_total_text ?? "0 secs"} detail={todayDetail(status.data?.data.project, status.data?.data.language)} />
          <StatCard label="Daily average" value={data?.human_readable_daily_average ?? "0 secs"} detail={`${data?.days?.length ?? 0} calendar days`} />
          <StatCard label="Best day" value={data?.best_day?.text ?? "0 secs"} detail={data?.best_day?.date ?? "No activity yet"} />
          <StatCard label="All time" value={allTime.data?.data.text ?? "0 secs"} detail="Since first heartbeat" />
        </section>
      )}

      <section className="mt-6 rounded border border-line bg-panel p-5">
        <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-center">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded bg-white/5 text-accent">
              <Monitor size={20} />
            </div>
            <div>
              <h2 className="font-medium">Today status bar</h2>
              <p className="mt-1 text-sm text-zinc-400">
                {status.data?.data.grand_total_text ?? "0 secs"} today
                {status.data?.data.project ? ` in ${status.data.data.project}` : ""}
                {status.data?.data.language ? ` using ${status.data.data.language}` : ""}
              </p>
            </div>
          </div>
          <Link className="inline-flex items-center justify-center gap-2 rounded border border-line px-4 py-2 text-sm text-zinc-300 hover:bg-white/5" href="/insights">
            Inspect breakdowns <ArrowRight size={16} />
          </Link>
        </div>
      </section>

      <div className="mt-6">
        <AIPanel metrics={data?.ai} />
      </div>

      <section className="mt-6 grid gap-5 xl:grid-cols-[1.2fr_0.8fr]">
        <AIHumanByDay days={aiTrend.data?.data.ai?.days ?? []} title="AI vs Human 30-Day Trend" />
        <WeekdayHeatmap days={data?.days ?? []} />
      </section>

      <ProjectAIGrid rows={data?.project_ai ?? []} />

      <section className="mt-6 grid gap-5 xl:grid-cols-[1.4fr_1fr]">
        <ProjectStackedArea days={data?.days ?? []} />
        <div className="grid gap-5">
          <SliceDonut title="Projects" rows={data?.projects ?? []} />
          <SliceDonut title="Languages" rows={data?.languages ?? []} colors={languageColors} />
          <SliceBars title="Categories" rows={data?.categories ?? []} />
        </div>
      </section>

      <section className="mt-6">
        <ActivityBars days={data?.days ?? []} title={`${activeRange.label} Activity`} />
      </section>

      <section className="mt-6 grid gap-5 xl:grid-cols-2">
        <HourlyTimeline hours={data?.hourly ?? []} mode="projects" />
        <HourlyTimeline hours={data?.hourly ?? []} mode="languages" colors={languageColors} />
      </section>

      <section className="mt-6 grid gap-5 lg:grid-cols-3">
        <SliceDonut title="Editors" rows={data?.editors ?? []} />
        <SliceDonut title="Machines" rows={data?.machines ?? []} />
        <SliceDonut title="Operating Systems" rows={data?.operating_systems ?? []} />
      </section>

      <section className="mt-6 rounded border border-line bg-panel p-5">
        <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
          <div>
            <h2 className="font-medium">Editor setup</h2>
            <p className="mt-1 text-sm text-zinc-400">Create a Stint key, connect your editor, and start collecting live activity.</p>
          </div>
          <Link className="inline-flex items-center justify-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink" href="/settings">
            Open settings <ArrowRight size={16} />
          </Link>
        </div>
      </section>
    </div>
  );
}

function OnboardingModal({ configBlock, onDismiss }: { configBlock: string; onDismiss: () => void }) {
	const [copied, setCopied] = useState(false);
	return (
		<div className="fixed inset-0 z-50 grid place-items-center bg-black/70 px-4 py-6 backdrop-blur-sm">
			<section className="w-full max-w-2xl rounded border border-line bg-panel shadow-glow">
				<div className="flex items-start justify-between gap-4 border-b border-line p-5">
					<div>
						<div className="mb-3 inline-flex h-10 w-10 items-center justify-center rounded bg-accent text-ink">
							<KeyRound size={20} />
						</div>
						<h2 className="text-2xl font-semibold tracking-tight">Connect your editor</h2>
						<p className="mt-2 text-sm leading-6 text-zinc-400">Create a Stint API key, save the editor config, then open a project. Activity appears as soon as your editor checks in.</p>
					</div>
					<button className="rounded border border-line p-2 text-zinc-400 hover:bg-white/5 hover:text-zinc-100" onClick={onDismiss} aria-label="Dismiss setup">
						<X size={18} />
					</button>
				</div>
				<div className="grid gap-5 p-5 lg:grid-cols-[0.85fr_1.15fr]">
					<div className="space-y-3">
						<a className="flex items-center justify-between gap-3 rounded border border-line bg-ink px-4 py-3 text-sm text-zinc-200 hover:border-accent/60" href="/integrations">
							<span>Choose an editor client</span>
							<ArrowRight size={16} />
						</a>
						<Link className="flex items-center justify-between gap-3 rounded border border-line bg-ink px-4 py-3 text-sm text-zinc-200 hover:border-accent/60" href="/settings">
							<span>Create API key</span>
							<ArrowRight size={16} />
						</Link>
						<div className="rounded border border-line bg-ink px-4 py-3 text-sm text-zinc-300">
							<div className="font-medium text-zinc-100">Open your editor</div>
							<div className="mt-1 text-xs leading-5 text-zinc-500">Start coding after saving the config. Activity appears within 2 minutes.</div>
						</div>
						<button className="flex w-full items-center justify-between gap-3 rounded border border-line bg-ink px-4 py-3 text-sm text-zinc-200 hover:border-accent/60" onClick={onDismiss}>
							<span>Activity is already sending</span>
							<Check size={16} />
						</button>
					</div>
					<div className="min-w-0">
						<div className="mb-3 flex items-center justify-between gap-3">
							<div className="text-sm font-medium text-zinc-200">Editor config</div>
							<SecondaryButton
								onClick={async () => {
									await navigator.clipboard.writeText(configBlock);
									setCopied(true);
								}}
							>
								{copied ? <Check size={15} /> : <Copy size={15} />} {copied ? "Copied" : "Copy"}
							</SecondaryButton>
						</div>
						<pre className="overflow-x-auto rounded border border-line bg-ink p-4 text-sm leading-6 text-zinc-200">{configBlock}</pre>
					</div>
				</div>
			</section>
		</div>
	);
}

function todayDetail(project?: string, language?: string) {
  if (project && language) {
    return `${project} · ${language}`;
  }
  return project || language || "Current day";
}

function useStoredBoolean(key: string) {
	const eventName = `stint-storage:${key}`;
	const value = useSyncExternalStore(
		(onStoreChange) => {
			if (typeof window === "undefined") return noopSubscribe();
			const onStorage = (event: StorageEvent) => {
				if (event.key === key) onStoreChange();
			};
			window.addEventListener("storage", onStorage);
			window.addEventListener(eventName, onStoreChange);
			return () => {
				window.removeEventListener("storage", onStorage);
				window.removeEventListener(eventName, onStoreChange);
			};
		},
		() => {
			if (typeof window !== "undefined") {
				return window.localStorage.getItem(key) === "true";
			}
			return false;
		},
		() => false
	);
	return [
		value,
		(nextValue: boolean) => {
			if (typeof window !== "undefined") {
				window.localStorage.setItem(key, String(nextValue));
				window.dispatchEvent(new Event(eventName));
			}
		}
	] as const;
}

function freshnessLabel(stats?: Stats) {
  if (!stats) {
    return "loading cache";
  }
  return stats.is_up_to_date ? "cache fresh" : "cache refreshing";
}

function ProjectAIGrid({ rows }: { rows: AIStat[] }) {
  const visibleRows = rows.slice(0, 6);
  return (
    <section className="mt-6">
      <div className="mb-3 flex flex-col justify-between gap-2 sm:flex-row sm:items-end">
        <div>
          <h2 className="text-lg font-medium text-zinc-100">Project AI mix</h2>
          <p className="mt-1 text-sm text-zinc-400">Per-project changes, prompts, tokens, sessions, spend, and active time.</p>
        </div>
        <Link className="inline-flex items-center gap-2 text-sm text-accent hover:text-cyan-200" href="/projects">
          All projects <ArrowRight size={15} />
        </Link>
      </div>
      {visibleRows.length > 0 ? (
        <div className="grid gap-4 lg:grid-cols-2 2xl:grid-cols-3">
          {visibleRows.map((row) => (
            <Link
              key={row.name}
              className="group rounded border border-line bg-panel p-4 transition hover:border-accent/60 hover:bg-white/[0.04]"
              href={`/projects/${encodeURIComponent(row.name)}`}
            >
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="truncate text-base font-medium text-zinc-100">{row.name}</div>
                  <div className="mt-1 text-xs uppercase tracking-[0.16em] text-zinc-500">{formatSeconds(row.ai_seconds)} active</div>
                </div>
                <span className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded border border-accent/30 bg-accent/10 text-accent">
                  <Bot size={18} />
                </span>
              </div>
              <div className="mt-4 grid grid-cols-2 gap-3">
                <ProjectAIMetric icon={<Sparkles size={15} />} label="AI changes" value={compactNumber(row.ai_line_changes)} />
                <ProjectAIMetric icon={<GitPullRequestArrow size={15} />} label="Human changes" value={compactNumber(row.human_line_changes)} />
                <ProjectAIMetric icon={<Bot size={15} />} label="Prompt chars" value={compactNumber(row.ai_prompt_length)} />
                <ProjectAIMetric icon={<Activity size={15} />} label="Sessions" value={compactNumber(row.session_count)} />
                <ProjectAIMetric icon={<Monitor size={15} />} label="Tokens" value={compactNumber(row.ai_input_tokens + row.ai_output_tokens)} />
                <ProjectAIMetric icon={<Coins size={15} />} label="Spend" value={formatCents(row.estimated_cost_cents)} />
              </div>
            </Link>
          ))}
        </div>
      ) : (
        <EmptyState icon={<Bot size={20} />} title="No project activity yet" hint="Send heartbeats with project data to populate the project grid." />
      )}
    </section>
  );
}

function ProjectAIMetric({ icon, label, value }: { icon: ReactNode; label: string; value: string }) {
  return (
    <div className="min-w-0 rounded border border-white/5 bg-black/20 p-3">
      <div className="mb-2 flex items-center gap-2 text-xs text-zinc-500">
        <span className="text-zinc-400">{icon}</span>
        <span className="truncate">{label}</span>
      </div>
      <div className="truncate text-sm font-medium text-zinc-200">{value}</div>
    </div>
  );
}

function formatSeconds(value: number) {
  if (value <= 0) {
    return "0 secs";
  }
  const minutes = Math.floor(value / 60);
  if (minutes < 1) {
    return `${value} secs`;
  }
  const hours = Math.floor(minutes / 60);
  if (hours < 1) {
    return `${minutes} mins`;
  }
  const remainingMinutes = minutes % 60;
  return remainingMinutes > 0 ? `${hours} hrs ${remainingMinutes} mins` : `${hours} hrs`;
}
