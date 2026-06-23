"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, Plus } from "lucide-react";
import { useState } from "react";
import { createDataDump, dataDumpDownloadURL, listDataDumps } from "@/lib/api";
import { dataDumpExpiryText, dataDumpIsDownloadable, hasPendingDumps } from "@/lib/data-dumps";

export function DataExportCard() {
  const client = useQueryClient();
  const [settingsDumpType, setSettingsDumpType] = useState<"heartbeats" | "daily">("heartbeats");
  const settingsDumps = useQuery({
    queryKey: ["settings-data-dumps"],
    queryFn: listDataDumps,
    retry: false,
    refetchInterval: (query) => (hasPendingDumps(query.state.data) ? 2000 : false)
  });
  const createSettingsDump = useMutation({
    mutationFn: () => createDataDump(settingsDumpType),
    onSuccess: () => client.invalidateQueries({ queryKey: ["settings-data-dumps"] })
  });

  return (
    <section className="mt-5 rounded border border-line bg-panel p-5">
      <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-end">
        <div>
          <h2 className="font-medium">Data export</h2>
          <p className="mt-1 text-sm text-zinc-400">Generate downloadable heartbeat archives or daily summary exports for backup and portability.</p>
        </div>
        <div className="flex flex-col gap-2 sm:flex-row">
          <select className="rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={settingsDumpType} onChange={(event) => setSettingsDumpType(event.target.value as "heartbeats" | "daily")}>
            <option value="heartbeats">Heartbeats</option>
            <option value="daily">Daily summaries</option>
          </select>
          <button className="inline-flex items-center justify-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60" onClick={() => createSettingsDump.mutate()} disabled={createSettingsDump.isPending}>
            <Plus size={16} /> Generate export
          </button>
        </div>
      </div>
      <div className="mt-4 divide-y divide-line rounded border border-line">
        {(settingsDumps.data?.data ?? []).slice(0, 8).map((dump) => {
          const isReady = dataDumpIsDownloadable(dump);
          const expiryText = dataDumpExpiryText(dump);
          return (
            <a
              key={dump.id}
              className={`flex flex-col justify-between gap-2 px-3 py-3 text-sm sm:flex-row sm:items-center ${isReady ? "hover:bg-white/5" : "cursor-not-allowed opacity-60"}`}
              href={isReady ? dataDumpDownloadURL(dump.download_url) : "#"}
              aria-disabled={!isReady}
              onClick={(event) => {
                if (!isReady) {
                  event.preventDefault();
                }
              }}
            >
              <span>
                <span className="font-medium text-zinc-100">{dump.type}</span>
                <span className="ml-2 text-zinc-500">{dump.status}</span>
                {expiryText ? <span className="ml-2 text-zinc-600">{expiryText}</span> : null}
                {!isReady && !expiryText ? <span className="ml-2 text-zinc-600">{dump.percent_complete}%</span> : null}
              </span>
              {isReady ? <span className="inline-flex items-center gap-2 text-zinc-300"><Download size={15} /> Download</span> : null}
            </a>
          );
        })}
        {settingsDumps.data?.data.length === 0 ? <div className="p-3 text-sm text-zinc-500">No exports generated yet.</div> : null}
      </div>
      {createSettingsDump.error ? <p className="mt-3 text-sm text-red-300">{createSettingsDump.error.message}</p> : null}
    </section>
  );
}
