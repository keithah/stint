"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { ArrowRight, Boxes, Clock3 } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { PageHeader, SecondaryLink } from "@/components/ui";
import { listProjects, statsForRange } from "@/lib/api";

export default function ProjectsPage() {
  return (
    <AppShell>
      <ProjectsContent />
    </AppShell>
  );
}

function ProjectsContent() {
  const projects = useQuery({ queryKey: ["projects"], queryFn: listProjects, retry: false });
  const stats = useQuery({ queryKey: ["stats", "last_30_days"], queryFn: () => statsForRange("last_30_days"), retry: false });
  const totals = new Map((stats.data?.data.projects ?? []).map((row) => [row.name, row]));

  return (
    <div className="mx-auto max-w-6xl px-5 py-6 lg:px-8">
      <PageHeader
        icon={<Boxes size={14} />}
        caption="Project activity"
        title="Projects"
        sub="Recently seen projects with last heartbeat timestamps and 30-day totals."
      />

      <section className="overflow-hidden rounded border border-line bg-panel">
        <div className="grid grid-cols-[1.4fr_1fr_1fr_auto] gap-4 border-b border-line px-4 py-3 text-xs uppercase tracking-[0.16em] text-zinc-500">
          <span>Name</span>
          <span>30-day time</span>
          <span>Last seen</span>
          <span />
        </div>
        {(projects.data?.data ?? []).map((project) => {
          const total = totals.get(project.name);
          return (
            <div key={project.id} className="grid grid-cols-[1.4fr_1fr_1fr_auto] items-center gap-4 border-b border-line px-4 py-4 last:border-b-0">
              <div className="min-w-0">
                <div className="truncate font-medium text-zinc-100">{project.name}</div>
                <div className="mt-1 text-sm text-zinc-500">Created {formatDate(project.created_at)}</div>
              </div>
              <div className="flex items-center gap-2 text-sm text-zinc-300">
                <Clock3 size={15} className="text-accent" /> {total?.text ?? "0 secs"}
              </div>
              <div className="text-sm text-zinc-400">{project.last_heartbeat_at ? formatDate(project.last_heartbeat_at) : "No heartbeat"}</div>
              <SecondaryLink href={`/projects/${encodeURIComponent(project.name)}`}>
                Inspect <ArrowRight size={15} />
              </SecondaryLink>
            </div>
          );
        })}
        {projects.data?.data.length === 0 ? <div className="p-5 text-sm text-zinc-500">No projects yet. Send a heartbeat with a project name.</div> : null}
      </section>
    </div>
  );
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" }).format(new Date(value));
}
