"use client";

import { useQuery } from "@tanstack/react-query";
import { Activity, BarChart3 } from "lucide-react";
import dynamic from "next/dynamic";
import { useMemo } from "react";
import { StatCard } from "@/components/stat-card";
import { listProgramLanguages, type Stats } from "@/lib/api";
import { languageColorMap } from "@/lib/language-colors";

const AIPanel = dynamic(() => import("@/components/ai-panel").then((module) => module.AIPanel), { ssr: false });
const ActivityBars = dynamic(() => import("@/components/dashboard-charts").then((module) => module.ActivityBars), { ssr: false });
const SliceDonut = dynamic(() => import("@/components/dashboard-charts").then((module) => module.SliceDonut), { ssr: false });

type ShareStatsResponse = { data: Stats; user: { id: string; username: string; name?: string } };

export function PublicSharePage({
  queryKey,
  queryFn,
  enabled
}: {
  queryKey: readonly unknown[];
  queryFn: () => Promise<ShareStatsResponse>;
  enabled: boolean;
}) {
  const stats = useQuery({ queryKey, queryFn, enabled });
  const programLanguages = useQuery({ queryKey: ["program-languages"], queryFn: listProgramLanguages, staleTime: 3600000 });
  const data = stats.data?.data;
  const user = stats.data?.user;
  const languageColors = useMemo(() => languageColorMap(programLanguages.data?.data ?? []), [programLanguages.data?.data]);

  if (stats.isError) {
    return (
      <main className="grid min-h-screen place-items-center px-6">
        <section className="max-w-md rounded border border-line bg-panel p-6">
          <h1 className="text-xl font-semibold">Share link unavailable</h1>
          <p className="mt-2 text-sm text-zinc-400">{stats.error.message}</p>
        </section>
      </main>
    );
  }

  return (
    <main className="min-h-screen">
      <div className="mx-auto max-w-6xl px-5 py-8 lg:px-8">
        <header className="mb-8 border-b border-line pb-6">
          <div className="mb-3 inline-flex items-center gap-2 rounded border border-accent/30 bg-accent/10 px-3 py-1 text-xs uppercase tracking-[0.18em] text-accent">
            <Activity size={14} /> Public stats
          </div>
          <h1 className="text-4xl font-semibold tracking-tight">{user?.name || user?.username || "Shared dashboard"}</h1>
          <p className="mt-2 text-sm text-zinc-400">Read-only coding activity for the last 7 days.</p>
        </header>

        <section className="grid gap-4 md:grid-cols-3">
          <StatCard label="Total" value={data?.human_readable_total ?? "0 secs"} detail="Last 7 days" />
          <StatCard label="Daily average" value={data?.human_readable_daily_average ?? "0 secs"} detail={`${data?.days.length ?? 0} calendar days`} />
          <StatCard label="Best day" value={data?.best_day.text ?? "0 secs"} detail={data?.best_day.date ?? "No activity yet"} />
        </section>

        <section className="mt-5 grid gap-5 xl:grid-cols-[1.4fr_1fr]">
          <ActivityBars days={data?.days ?? []} />
          <div className="grid gap-5">
            <SliceDonut title="Projects" rows={data?.projects ?? []} />
            <SliceDonut title="Languages" rows={data?.languages ?? []} colors={languageColors} />
          </div>
        </section>

        <section className="mt-5">
          <AIPanel metrics={data?.ai} />
        </section>

        <section className="mt-5 grid gap-5 lg:grid-cols-3">
          <SliceDonut title="Editors" rows={data?.editors ?? []} />
          <SliceDonut title="Machines" rows={data?.machines ?? []} />
          <SliceDonut title="Categories" rows={data?.categories ?? []} />
        </section>

        <footer className="mt-8 flex items-center gap-2 border-t border-line pt-5 text-sm text-zinc-500">
          <BarChart3 size={16} /> Powered by Stint
        </footer>
      </div>
    </main>
  );
}
