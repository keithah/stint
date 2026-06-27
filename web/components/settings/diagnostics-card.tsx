"use client";

import { useQuery } from "@tanstack/react-query";
import { useSyncExternalStore } from "react";
import { serverMeta, wakatimeAPIURL } from "@/lib/api";
import { Diagnostic, noopSubscribe, serverWakaTimeAPIURL } from "@/components/settings/shared";

export function DiagnosticsCard() {
  const meta = useQuery({ queryKey: ["server-meta"], queryFn: serverMeta, staleTime: 60000 });
  const apiURL = useSyncExternalStore(noopSubscribe, wakatimeAPIURL, serverWakaTimeAPIURL);

  return (
    <section className="mt-5 rounded border border-line bg-panel p-5">
      <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
        <div>
          <h2 className="font-medium">Server diagnostics</h2>
          <p className="mt-1 text-sm text-zinc-400">Confirm the public API origin and runtime details reported to connected clients.</p>
        </div>
        <code className="rounded border border-line bg-ink px-3 py-2 text-xs text-zinc-500">GET /api/v1/meta</code>
      </div>
      <div className="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-5">
        <Diagnostic label="API URL" value={meta.data?.data.api_url ?? apiURL} />
        <Diagnostic label="Base URL" value={meta.data?.data.base_url ?? "Loading"} />
        <Diagnostic label="Hostname" value={meta.data?.data.hostname ?? "Loading"} />
        <Diagnostic label="Client IP" value={meta.data?.data.ip ?? "Loading"} />
        <Diagnostic label="Version" value={meta.data?.data.version ?? "Loading"} />
      </div>
      {meta.error ? <p className="mt-3 text-sm text-red-300">{meta.error.message}</p> : null}
    </section>
  );
}
