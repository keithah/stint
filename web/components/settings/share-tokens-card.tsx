"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Trash2 } from "lucide-react";
import { useState, useSyncExternalStore } from "react";
import { createShareToken, deleteShareToken, listShareTokens, me, wakatimeAPIURL } from "@/lib/api";
import { noopSubscribe, serverWakaTimeAPIURL, shareStatsJSONPURL } from "@/components/settings/shared";

export function ShareTokensCard() {
  const client = useQueryClient();
  const user = useQuery({ queryKey: ["me"], queryFn: me, });
  const shareTokens = useQuery({ queryKey: ["share-tokens"], queryFn: listShareTokens, });
  const [latestShareToken, setLatestShareToken] = useState("");
  const [shareName, setShareName] = useState("Public dashboard");
  const apiURL = useSyncExternalStore(noopSubscribe, wakatimeAPIURL, serverWakaTimeAPIURL);
  const publicOrigin = typeof window === "undefined" ? "" : window.location.origin;
  const canCreateShareToken = shareName.trim().length > 0;
  const createShare = useMutation({
    mutationFn: () => createShareToken(shareName.trim()),
    onSuccess: (result) => {
      setLatestShareToken(result.data.token ?? "");
      client.invalidateQueries({ queryKey: ["share-tokens"] });
    }
  });
  const removeShare = useMutation({
    mutationFn: deleteShareToken,
    onSuccess: () => client.invalidateQueries({ queryKey: ["share-tokens"] })
  });

  return (
    <section className="mt-5 rounded border border-line bg-panel p-5">
      <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-end">
        <div>
          <h2 className="font-medium">Share tokens</h2>
          <p className="mt-1 text-sm text-zinc-400">Create read-only public stats links and JSONP-compatible embed endpoints.</p>
        </div>
        <div className="flex w-full flex-col gap-2 sm:w-auto sm:min-w-96 sm:flex-row">
          <input className="min-w-0 flex-1 rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={shareName} onChange={(event) => setShareName(event.target.value)} />
          <button className="inline-flex items-center justify-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60" onClick={() => createShare.mutate()} disabled={createShare.isPending || !canCreateShareToken}>
            <Plus size={16} /> Create
          </button>
        </div>
      </div>
      {latestShareToken && user.data?.data ? (
        <div className="mt-4 rounded border border-accent/40 bg-accent/10 p-3">
          <div className="text-xs uppercase tracking-[0.16em] text-accent">Share token shown once</div>
          <code className="mt-2 block break-all text-sm text-zinc-100">{latestShareToken}</code>
          <code className="mt-2 block break-all text-xs text-zinc-400">{`${publicOrigin}/share/${latestShareToken}`}</code>
          <div className="mt-3 text-xs uppercase tracking-[0.16em] text-accent">JSONP stats endpoint</div>
          <code className="mt-2 block break-all text-xs text-zinc-400">{shareStatsJSONPURL(apiURL, latestShareToken)}</code>
        </div>
      ) : null}
      <div className="mt-4 divide-y divide-line rounded border border-line">
        {(shareTokens.data?.data ?? []).map((token) => (
          <div key={token.id} className="flex flex-col justify-between gap-3 p-4 sm:flex-row sm:items-center">
            <div>
              <div className="font-medium text-zinc-100">{token.name}</div>
              <div className="mt-1 text-sm text-zinc-500">Fingerprint {token.fingerprint}</div>
            </div>
            <button className="inline-flex items-center justify-center gap-2 rounded border border-red-900/70 px-3 py-2 text-sm text-red-300 hover:bg-red-950/40" onClick={() => removeShare.mutate(token.id)}>
              <Trash2 size={15} /> Delete
            </button>
          </div>
        ))}
        {shareTokens.data?.data.length === 0 ? <div className="p-4 text-sm text-zinc-500">No share tokens yet.</div> : null}
      </div>
    </section>
  );
}
