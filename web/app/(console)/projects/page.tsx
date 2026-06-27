"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { ArrowRight, Boxes, Clock3 } from "lucide-react";
import { useMemo, useState } from "react";
import { PageHeader, SecondaryButton, SecondaryLink } from "@/components/ui";
import { listProjects, statsForRange } from "@/lib/api";

const projectsPageSize = 50;

export default function ProjectsPage() {
  return (
    <ProjectsContent />
  );
}

function ProjectsContent() {
  const [page, setPage] = useState(1);
	  const projects = useQuery({ queryKey: ["projects"], queryFn: listProjects, });
	  const stats = useQuery({ queryKey: ["stats", "last_30_days"], queryFn: () => statsForRange("last_30_days"), });
	  const totals = useMemo(() => new Map((stats.data?.data.projects ?? []).map((row) => [row.name, row])), [stats.data?.data.projects]);
  const allProjects = projects.data?.data ?? [];
  const totalPages = Math.max(1, Math.ceil(allProjects.length / projectsPageSize));
  const currentPage = Math.min(page, totalPages);
  const visibleProjects = useMemo(() => allProjects.slice((currentPage - 1) * projectsPageSize, currentPage * projectsPageSize), [allProjects, currentPage]);

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
        {visibleProjects.map((project) => {
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
        {allProjects.length > projectsPageSize ? (
          <div className="flex flex-col justify-between gap-3 border-t border-line px-4 py-3 text-sm text-zinc-400 sm:flex-row sm:items-center">
            <span>
              Showing {(currentPage - 1) * projectsPageSize + 1}-{Math.min(currentPage * projectsPageSize, allProjects.length)} of {allProjects.length}
            </span>
            <div className="flex gap-2">
              <SecondaryButton onClick={() => setPage(Math.max(1, currentPage - 1))} disabled={currentPage === 1}>
                Previous
              </SecondaryButton>
              <SecondaryButton onClick={() => setPage(Math.min(totalPages, currentPage + 1))} disabled={currentPage === totalPages}>
                Next
              </SecondaryButton>
            </div>
          </div>
        ) : null}
        {projects.data?.data.length === 0 ? <div className="p-5 text-sm text-zinc-500">No projects yet. Send a heartbeat with a project name.</div> : null}
      </section>
    </div>
  );
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" }).format(new Date(value));
}
