"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { createOAuthApp, deleteOAuthApp, listOAuthApps } from "@/lib/api";
import { isHTTPURL } from "@/components/settings/shared";

export function OAuthAppsCard() {
  const client = useQueryClient();
  const oauthApps = useQuery({ queryKey: ["oauth-apps"], queryFn: listOAuthApps, });
  const [latestOAuthSecret, setLatestOAuthSecret] = useState("");
  const [oauthName, setOAuthName] = useState("Local OAuth client");
  const [oauthRedirect, setOAuthRedirect] = useState("http://localhost:3000/oauth/callback");
  const [oauthScopes, setOAuthScopes] = useState("read_stats read_summaries write_heartbeats");
  const oauthRedirectURIs = oauthRedirect.split("\n").map((value) => value.trim()).filter(Boolean);
  const canCreateOAuthApp = oauthName.trim().length > 0 && oauthRedirectURIs.length > 0 && oauthRedirectURIs.every(isHTTPURL);
  const createApp = useMutation({
    mutationFn: () =>
      createOAuthApp({
        name: oauthName.trim(),
        redirect_uris: oauthRedirectURIs,
        scopes: oauthScopes.split(/[,\s]+/).map((value) => value.trim()).filter(Boolean)
      }),
    onSuccess: (result) => {
      setLatestOAuthSecret(result.data.client_secret ?? "");
      client.invalidateQueries({ queryKey: ["oauth-apps"] });
    }
  });
  const deleteApp = useMutation({
    mutationFn: deleteOAuthApp,
    onSuccess: () => client.invalidateQueries({ queryKey: ["oauth-apps"] })
  });

  return (
    <section className="mt-5 rounded border border-line bg-panel p-5">
      <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
        <div>
          <h2 className="font-medium">OAuth applications</h2>
          <p className="mt-1 text-sm text-zinc-400">Register external clients for authorization-code and refresh-token flows.</p>
        </div>
        <button
          className="inline-flex items-center justify-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60"
          onClick={() => createApp.mutate()}
          disabled={createApp.isPending || !canCreateOAuthApp}
        >
          <Plus size={16} /> Create app
        </button>
      </div>
      <div className="mt-5 grid gap-4 lg:grid-cols-[0.85fr_1.15fr]">
        <div className="space-y-3">
          <label className="block">
            <span className="text-sm text-zinc-400">App name</span>
            <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={oauthName} onChange={(event) => setOAuthName(event.target.value)} />
          </label>
          <label className="block">
            <span className="text-sm text-zinc-400">Redirect URIs</span>
            <textarea className="mt-2 min-h-20 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={oauthRedirect} onChange={(event) => setOAuthRedirect(event.target.value)} />
          </label>
          <label className="block">
            <span className="text-sm text-zinc-400">Scopes</span>
            <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={oauthScopes} onChange={(event) => setOAuthScopes(event.target.value)} />
          </label>
          {latestOAuthSecret ? (
            <div className="rounded border border-accent/40 bg-accent/10 p-3">
              <div className="text-xs uppercase tracking-[0.16em] text-accent">Client secret shown once</div>
              <code className="mt-2 block break-all text-sm text-zinc-100">{latestOAuthSecret}</code>
            </div>
          ) : null}
          {createApp.error ? <p className="text-sm text-red-300">{createApp.error.message}</p> : null}
        </div>
        <div className="divide-y divide-line rounded border border-line">
          {(oauthApps.data?.data ?? []).map((app) => (
            <div key={app.id} className="p-4">
              <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-start">
                <div className="min-w-0">
                  <div className="font-medium text-zinc-100">{app.name}</div>
                  <code className="mt-1 block break-all text-xs text-zinc-500">{app.client_id}</code>
                </div>
                <button className="inline-flex items-center justify-center gap-2 rounded border border-red-900/70 px-3 py-2 text-sm text-red-300 hover:bg-red-950/40" onClick={() => deleteApp.mutate(app.id)}>
                  <Trash2 size={15} /> Delete
                </button>
              </div>
              <div className="mt-3 flex flex-wrap gap-2">
                {app.scopes.map((scope) => (
                  <span key={scope} className="rounded border border-line bg-ink px-2 py-1 text-xs text-zinc-400">{scope}</span>
                ))}
              </div>
              <div className="mt-3 space-y-1">
                {app.redirect_uris.map((uri) => (
                  <div key={uri} className="truncate text-xs text-zinc-500">{uri}</div>
                ))}
              </div>
            </div>
          ))}
          {oauthApps.data?.data.length === 0 ? <div className="p-4 text-sm text-zinc-500">No OAuth apps yet.</div> : null}
        </div>
      </div>
    </section>
  );
}
