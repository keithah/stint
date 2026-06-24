"use client";

import { useQuery } from "@tanstack/react-query";
import { useParams } from "next/navigation";
import { Boxes, ChevronLeft, ChevronRight, ExternalLink, GitCommitHorizontal } from "lucide-react";
import { useMemo, useState } from "react";
import { AIPanel } from "@/components/ai-panel";
import { ActivityBars, SliceDonut } from "@/components/dashboard-charts";
import { AppShell } from "@/components/app-shell";
import { StatCard } from "@/components/stat-card";
import { listProgramLanguages, listProjectCommits, projectDetail, type StatsRange } from "@/lib/api";
import { languageColorMap } from "@/lib/language-colors";
import { rangeOptions } from "@/lib/ranges";
import { PageHeader, SegmentedToggle, pillWrapperClass } from "@/components/ui";

export default function ProjectDetailPage() {
  return (
    <AppShell>
      <ProjectDetailContent />
    </AppShell>
  );
}

function ProjectDetailContent() {
  const params = useParams<{ name: string }>();
  const name = decodeURIComponent(params.name);
  const [range, setRange] = useState<StatsRange>("last_30_days");
  const [commitBranch, setCommitBranch] = useState("");
  const [commitPage, setCommitPage] = useState(1);
  const detail = useQuery({ queryKey: ["project", name, range], queryFn: () => projectDetail(name, range), retry: false });
  const commits = useQuery({ queryKey: ["project-commits", name, commitBranch, commitPage], queryFn: () => listProjectCommits(name, { branch: commitBranch || undefined, page: commitPage }), retry: false });
  const programLanguages = useQuery({ queryKey: ["program-languages"], queryFn: listProgramLanguages, retry: false, staleTime: 3600000 });
  const data = detail.data?.data;
  const languageColors = useMemo(() => languageColorMap(programLanguages.data?.data ?? []), [programLanguages.data?.data]);
  const activeRange = rangeOptions.find((item) => item.value === range) ?? rangeOptions[1];

  return (
    <div className="mx-auto max-w-6xl px-5 py-6 lg:px-8">
      <PageHeader
        icon={<Boxes size={14} />}
        caption="Project detail"
        title={name}
        sub={`${activeRange.label} of project-specific activity.`}
        actions={<SegmentedToggle options={rangeOptions} value={range} onChange={setRange} variant="pill" className={pillWrapperClass} />}
      />

      <section className="grid gap-4 md:grid-cols-3">
        <StatCard label={`${activeRange.label} total`} value={data?.stats.human_readable_total ?? "0 secs"} detail="Project-filtered" />
        <StatCard label="Daily average" value={data?.stats.human_readable_daily_average ?? "0 secs"} detail={`${data?.stats.days.length ?? 0} days`} />
        <StatCard label="Last seen" value={data?.project.last_heartbeat_at ? formatDate(data.project.last_heartbeat_at) : "Never"} detail="Most recent heartbeat" />
      </section>

      <section className="mt-6 grid gap-5 xl:grid-cols-[1.4fr_1fr]">
        <ActivityBars days={data?.stats.days ?? []} title={`${activeRange.label} Activity`} />
        <div className="grid gap-5">
          <SliceDonut title="Languages" rows={data?.stats.languages ?? []} colors={languageColors} />
          <SliceDonut title="Editors" rows={data?.stats.editors ?? []} />
        </div>
      </section>

      <section className="mt-6">
        <AIPanel metrics={data?.stats.ai} />
      </section>

      <section className="mt-6 grid gap-5 lg:grid-cols-2">
        <SliceDonut title="Branches" rows={data?.stats.branches ?? []} />
        <SliceDonut title="Dependencies" rows={data?.stats.dependencies ?? []} />
      </section>

      <section className="mt-6 overflow-hidden rounded border border-line bg-panel/80">
        <div className="flex flex-col justify-between gap-4 border-b border-line px-5 py-4 lg:flex-row lg:items-center">
          <div>
            <div className="flex items-center gap-2 text-sm font-medium text-zinc-200">
              <GitCommitHorizontal size={16} className="text-accent" /> Recent commits
            </div>
            <p className="mt-1 text-xs text-zinc-500">
              {commits.data?.total ?? 0} tracked commits
              {commits.data?.total_pages ? ` · page ${commits.data.page} of ${commits.data.total_pages}` : ""}
            </p>
          </div>
          <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto] lg:min-w-[380px]">
            <label className="sr-only" htmlFor="commit-branch">Commit branch</label>
            <input
              id="commit-branch"
              className="min-w-0 rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent"
              placeholder="Filter branch"
              value={commitBranch}
              onChange={(event) => {
                setCommitBranch(event.target.value);
                setCommitPage(1);
              }}
            />
            <div className="grid grid-cols-2 gap-2">
              <button
                className="inline-flex items-center justify-center gap-2 rounded border border-line px-3 py-2 text-sm text-zinc-300 hover:bg-white/5 disabled:opacity-40"
                onClick={() => setCommitPage(commits.data?.prev_page ?? 1)}
                disabled={!commits.data?.prev_page}
                aria-label="Previous commit page"
              >
                <ChevronLeft size={16} /> Prev
              </button>
              <button
                className="inline-flex items-center justify-center gap-2 rounded border border-line px-3 py-2 text-sm text-zinc-300 hover:bg-white/5 disabled:opacity-40"
                onClick={() => setCommitPage(commits.data?.next_page ?? commitPage)}
                disabled={!commits.data?.next_page}
                aria-label="Next commit page"
              >
                Next <ChevronRight size={16} />
              </button>
            </div>
          </div>
        </div>
        <div className="divide-y divide-line">
          {(commits.data?.commits ?? []).map((commit) => (
            <div key={commit.hash} className="grid gap-3 px-5 py-4 md:grid-cols-[140px_1fr_120px] md:items-center">
              <a
                className="inline-flex min-w-0 items-center gap-2 text-sm text-accent hover:text-accent/80"
                href={commit.html_url || commit.url || undefined}
                target={commit.html_url || commit.url ? "_blank" : undefined}
                rel={commit.html_url || commit.url ? "noreferrer" : undefined}
                onClick={(event) => {
                  if (!commit.html_url && !commit.url) {
                    event.preventDefault();
                  }
                }}
              >
                <code className="truncate">{commit.truncated_hash}</code>
                {commit.html_url || commit.url ? <ExternalLink size={13} /> : null}
              </a>
              <div className="min-w-0">
                <div className="truncate text-sm font-medium text-zinc-200">{commit.branch || "Unknown branch"}</div>
                <div className="mt-1 truncate text-xs text-zinc-500">{commit.ref || commit.hash}</div>
              </div>
              <div className="text-sm font-medium text-zinc-300 md:text-right">{commit.human_readable_total}</div>
            </div>
          ))}
          {commits.data && commits.data.commits.length === 0 ? <div className="px-5 py-5 text-sm text-zinc-500">No commit metadata has been recorded for this project.</div> : null}
        </div>
      </section>
    </div>
  );
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" }).format(new Date(value));
}

