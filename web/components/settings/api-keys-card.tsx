"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, Plus } from "lucide-react";
import { useMemo, useState, useSyncExternalStore } from "react";
import { createKey, me, wakatimeAPIURL } from "@/lib/api";
import { SecondaryButton } from "@/components/ui";
import { noopSubscribe, serverWakaTimeAPIURL } from "@/components/settings/shared";

export function ApiKeysCard() {
  const client = useQueryClient();
  const user = useQuery({ queryKey: ["me"], queryFn: me, });
  const [latestKey, setLatestKey] = useState("");
  const [name, setName] = useState("Workstation");
  const [keyScopes, setKeyScopes] = useState("");
  const apiURL = useSyncExternalStore(noopSubscribe, wakatimeAPIURL, serverWakaTimeAPIURL);
  const timeoutMinutes = user.data?.data.timeout_minutes ?? 15;
  const canCreateAPIKey = name.trim().length > 0;
  const create = useMutation({
    mutationFn: (keyName: string) => createKey(keyName, keyScopes.split(/[,\s]+/).map((value) => value.trim()).filter(Boolean)),
    onSuccess: (result) => {
      setLatestKey(result.data.api_key);
      client.invalidateQueries({ queryKey: ["api-keys"] });
    }
  });
  const configBlock = useMemo(
    () => `[settings]\napi_url = ${apiURL}\napi_key = ${latestKey || "waka_00000000-0000-4000-8000-000000000000"}\nhide_file_names = false\ntimeout = ${timeoutMinutes}`,
    [apiURL, latestKey, timeoutMinutes]
  );
  const fanoutConfigBlock = useMemo(
    () => `[api_urls]\n.* = ${apiURL}|${latestKey || "waka_00000000-0000-4000-8000-000000000000"}`,
    [apiURL, latestKey]
  );

  return (
    <section className="grid gap-5 lg:grid-cols-[1fr_1fr]">
      <div className="rounded border border-line bg-panel p-5">
        <h2 className="font-medium">Create API key</h2>
        <div className="mt-4 grid gap-2">
          <input
            className="min-w-0 rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent"
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
          <div className="flex gap-2">
            <input
              className="min-w-0 flex-1 rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent"
              placeholder="Scopes, blank for full access"
              value={keyScopes}
              onChange={(event) => setKeyScopes(event.target.value)}
            />
            <button
              className="inline-flex items-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60"
              onClick={() => create.mutate(name.trim())}
              disabled={create.isPending || !canCreateAPIKey}
            >
              <Plus size={16} /> Create
            </button>
          </div>
        </div>
        {latestKey ? (
          <div className="mt-4 rounded border border-accent/40 bg-accent/10 p-3">
            <div className="text-xs uppercase tracking-[0.16em] text-accent">Shown once</div>
            <code className="mt-2 block break-all text-sm text-zinc-100">{latestKey}</code>
          </div>
        ) : null}
      </div>

      <div className="rounded border border-line bg-panel p-5">
        <div className="flex items-center justify-between gap-3">
          <h2 className="font-medium">Editor config file</h2>
          <SecondaryButton onClick={() => navigator.clipboard.writeText(configBlock)}>
            <Copy size={15} /> Copy
          </SecondaryButton>
        </div>
        <pre className="mt-4 overflow-x-auto rounded border border-line bg-ink p-4 text-sm leading-6 text-zinc-200">{configBlock}</pre>
      </div>

      <div className="rounded border border-line bg-panel p-5">
        <div className="flex items-center justify-between gap-3">
          <h2 className="font-medium">api_urls fanout</h2>
          <SecondaryButton onClick={() => navigator.clipboard.writeText(fanoutConfigBlock)}>
            <Copy size={15} /> Copy
          </SecondaryButton>
        </div>
        <p className="mt-2 text-sm text-zinc-400">Use this form when sending the same Codex or editor activity to multiple services.</p>
        <pre className="mt-4 overflow-x-auto rounded border border-line bg-ink p-4 text-sm leading-6 text-zinc-200">{fanoutConfigBlock}</pre>
      </div>
    </section>
  );
}
