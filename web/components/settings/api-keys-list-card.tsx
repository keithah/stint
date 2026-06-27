"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { listKeys, revokeKey } from "@/lib/api";

export function ApiKeysListCard() {
  const client = useQueryClient();
  const keys = useQuery({ queryKey: ["api-keys"], queryFn: listKeys, });
  const revoke = useMutation({
    mutationFn: revokeKey,
    onSuccess: () => client.invalidateQueries({ queryKey: ["api-keys"] })
  });

  return (
    <section className="mt-5 rounded border border-line bg-panel p-5">
      <h2 className="font-medium">API keys</h2>
      <div className="mt-4 divide-y divide-line rounded border border-line">
        {(keys.data?.data ?? []).map((key) => (
          <div key={key.id} className="flex flex-col justify-between gap-3 p-4 sm:flex-row sm:items-center">
            <div>
              <div className="font-medium text-zinc-100">{key.name}</div>
              <div className="mt-1 text-sm text-zinc-500">Fingerprint {key.fingerprint}</div>
              <div className="mt-2 flex flex-wrap gap-1">
                {key.scopes.map((scope) => (
                  <span key={scope} className="rounded border border-line bg-ink px-2 py-1 text-xs text-zinc-400">{scope}</span>
                ))}
              </div>
            </div>
            <button
              className="inline-flex items-center justify-center gap-2 rounded border border-red-900/70 px-3 py-2 text-sm text-red-300 hover:bg-red-950/40"
              onClick={() => revoke.mutate(key.id)}
            >
              <Trash2 size={15} /> Revoke
            </button>
          </div>
        ))}
        {keys.data?.data?.length === 0 ? <div className="p-4 text-sm text-zinc-500">No API keys yet.</div> : null}
      </div>
    </section>
  );
}
