"use client";

import { useQuery } from "@tanstack/react-query";
import { listEditors } from "@/lib/api";

export function EditorsCard() {
  const editors = useQuery({ queryKey: ["editors"], queryFn: listEditors, retry: false, staleTime: 3600000 });

  return (
    <section className="mt-5 rounded border border-line bg-panel p-5">
      <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
        <div>
          <h2 className="font-medium">Supported editors</h2>
          <p className="mt-1 text-sm text-zinc-400">Known editor clients exposed by the local metadata endpoint.</p>
        </div>
        <code className="rounded border border-line bg-ink px-3 py-2 text-xs text-zinc-500">GET /api/v1/editors</code>
      </div>
      <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {(editors.data?.data ?? []).slice(0, 8).map((editor) => (
          <div key={editor.key} className="rounded border border-line bg-ink p-3">
            <div className="font-medium text-zinc-100">{editor.name}</div>
            <div className="mt-1 text-xs text-zinc-500">{editor.key}</div>
            {editor.version ? <div className="mt-2 rounded border border-line px-2 py-1 text-xs text-zinc-400">{editor.version}</div> : null}
          </div>
        ))}
        {editors.data?.data.length === 0 ? <div className="rounded border border-line bg-ink p-3 text-sm text-zinc-500">No editor metadata returned yet.</div> : null}
      </div>
    </section>
  );
}
