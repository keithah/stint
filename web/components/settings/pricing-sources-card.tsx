"use client";

import { useQuery } from "@tanstack/react-query";
import { ExternalLink, Search } from "lucide-react";
import { useMemo, useState } from "react";
import { listPricingModels, listPricingSources, type PricingModel } from "@/lib/usage-api";

const MAX_ROWS = 60;

function freshness(source: { status: string; fetched_at?: string }): string {
  if (source.status === "bundled" || !source.fetched_at) return "Using bundled snapshot — not yet refreshed";
  const when = new Date(source.fetched_at);
  if (Number.isNaN(when.getTime())) return "Unknown";
  const days = Math.floor((Date.now() - when.getTime()) / 86_400_000);
  const rel = days <= 0 ? "today" : days === 1 ? "1 day ago" : `${days} days ago`;
  return `${source.status === "error" ? "Last attempt failed · last good " : "Updated "}${rel}`;
}

function fmtRate(perMillion: number): string {
  if (perMillion === 0) return "$0";
  if (perMillion < 0.01) return `$${perMillion.toFixed(4)}`;
  return `$${perMillion.toFixed(2)}`;
}

export function PricingSourcesCard() {
  const sources = useQuery({ queryKey: ["pricing-sources"], queryFn: listPricingSources, });
  const models = useQuery({ queryKey: ["pricing-models"], queryFn: listPricingModels, });
  const [search, setSearch] = useState("");

  const filtered = useMemo(() => {
    const all = models.data?.data ?? [];
    const q = search.trim().toLowerCase();
    const matched = q ? all.filter((m: PricingModel) => m.model.toLowerCase().includes(q)) : all;
    return { rows: matched.slice(0, MAX_ROWS), total: matched.length };
  }, [models.data, search]);

  return (
    <section className="mt-5 rounded border border-line bg-panel p-5">
      <div>
        <h2 className="font-medium">AI price sources</h2>
        <p className="mt-1 text-sm text-zinc-400">
          Costs use list prices from these public sources, refreshed automatically every week. Cache reads are priced at
          their discounted rate, not as fresh input. Override any model below in “Custom AI pricing”.
        </p>
      </div>

      <div className="mt-4 grid gap-3 sm:grid-cols-2">
        {(sources.data?.data ?? []).map((s) => (
          <div key={s.source} className="rounded border border-line bg-ink p-4">
            <div className="flex items-center justify-between gap-2">
              <span className="font-medium text-zinc-100">{s.label}</span>
              <a
                className="inline-flex items-center gap-1 text-xs text-accent hover:underline"
                href={s.site_url}
                target="_blank"
                rel="noreferrer"
              >
                source <ExternalLink size={12} />
              </a>
            </div>
            <div className="mt-1.5 text-xs text-zinc-500">{freshness(s)}</div>
            <div className="mt-1 text-xs text-zinc-600">
              {s.model_count.toLocaleString()} models priced{s.error ? ` · ${s.error}` : ""}
            </div>
          </div>
        ))}
        {sources.isError ? <p className="text-sm text-red-300">Could not load price sources.</p> : null}
      </div>

      <div className="mt-5">
        <label className="relative block">
          <Search size={15} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-zinc-500" />
          <input
            className="w-full rounded border border-line bg-ink py-2 pl-9 pr-3 text-sm outline-none focus:border-accent"
            placeholder="Search models to see the rate we use…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </label>

        <div className="mt-3 overflow-x-auto rounded border border-line">
          <table className="w-full min-w-[640px] text-sm">
            <thead className="text-xs uppercase tracking-wide text-zinc-500">
              <tr className="border-b border-line">
                <th className="px-3 py-2 text-left font-medium">Model</th>
                <th className="px-3 py-2 text-right font-medium">Input /1M</th>
                <th className="px-3 py-2 text-right font-medium">Output /1M</th>
                <th className="px-3 py-2 text-right font-medium">Cache read /1M</th>
                <th className="px-3 py-2 text-left font-medium">Source</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line">
              {filtered.rows.map((m) => (
                <tr key={`${m.source}:${m.model}`}>
                  <td className="px-3 py-2 font-medium text-zinc-100">
                    {m.model}
                    {m.overridden ? <span className="ml-2 rounded bg-accent/20 px-1.5 py-0.5 text-[10px] text-accent">overridden</span> : null}
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums text-zinc-300">{fmtRate(m.input_per_million_usd)}</td>
                  <td className="px-3 py-2 text-right tabular-nums text-zinc-300">{fmtRate(m.output_per_million_usd)}</td>
                  <td className="px-3 py-2 text-right tabular-nums text-zinc-400">{fmtRate(m.cache_read_per_million_usd)}</td>
                  <td className="px-3 py-2 text-zinc-500">{m.source}</td>
                </tr>
              ))}
              {models.isLoading ? (
                <tr><td className="px-3 py-3 text-zinc-500" colSpan={5}>Loading prices…</td></tr>
              ) : filtered.rows.length === 0 ? (
                <tr><td className="px-3 py-3 text-zinc-500" colSpan={5}>No models match “{search}”.</td></tr>
              ) : null}
            </tbody>
          </table>
        </div>
        {filtered.total > filtered.rows.length ? (
          <p className="mt-2 text-xs text-zinc-500">Showing {filtered.rows.length} of {filtered.total.toLocaleString()} matches — refine your search to see more.</p>
        ) : null}
      </div>
    </section>
  );
}
