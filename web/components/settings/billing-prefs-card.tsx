"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Save, Trash2 } from "lucide-react";
import { useState } from "react";
import { deleteBillingPref, listBillingPrefs, upsertBillingPref, type BillingPref } from "@/lib/usage-api";

const BILLING_LABELS: Record<BillingPref["billing_type"], string> = {
  subscription: "Subscription (flat-rate, $0 marginal)",
  api: "API (metered, marginal = cost)"
};

export function BillingPrefsCard() {
  const client = useQueryClient();
  const billingPrefs = useQuery({ queryKey: ["billing-prefs"], queryFn: listBillingPrefs, retry: false });
  const [agent, setAgent] = useState("claude-code");
  const [billingType, setBillingType] = useState<BillingPref["billing_type"]>("subscription");
  const canSave = agent.trim() !== "";
  const savePref = useMutation({
    mutationFn: () => upsertBillingPref({ agent: agent.trim(), billing_type: billingType }),
    onSuccess: () => client.invalidateQueries({ queryKey: ["billing-prefs"] })
  });
  const removePref = useMutation({
    mutationFn: deleteBillingPref,
    onSuccess: () => client.invalidateQueries({ queryKey: ["billing-prefs"] })
  });

  return (
    <section className="mt-5 rounded border border-line bg-panel p-5">
      <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-end">
        <div>
          <h2 className="font-medium">Per-agent billing mode</h2>
          <p className="mt-1 text-sm text-zinc-400">Declare which agents are flat-rate subscription versus metered API. Subscription agents contribute $0 marginal cost (the spend is already covered by your subscription); the equivalent-API cost is still shown so you can see what the usage would have cost. This overrides the billing type the collector recorded.</p>
        </div>
        <button className="inline-flex items-center justify-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60" onClick={() => savePref.mutate()} disabled={savePref.isPending || !canSave}>
          <Save size={16} /> Save override
        </button>
      </div>
      <div className="mt-4 grid gap-3 md:grid-cols-2">
        <label className="block">
          <span className="text-sm text-zinc-400">Agent</span>
          <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={agent} onChange={(event) => setAgent(event.target.value)} placeholder="claude-code" />
        </label>
        <label className="block">
          <span className="text-sm text-zinc-400">Billing mode</span>
          <select className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={billingType} onChange={(event) => setBillingType(event.target.value as BillingPref["billing_type"])}>
            <option value="subscription">Subscription (flat-rate, $0 marginal)</option>
            <option value="api">API (metered, marginal = cost)</option>
          </select>
        </label>
      </div>
      <div className="mt-4 divide-y divide-line rounded border border-line">
        {(billingPrefs.data?.data ?? []).map((pref) => (
          <div key={pref.agent} className="grid items-center gap-2 px-3 py-3 text-sm sm:grid-cols-[1fr_auto]">
            <div className="grid gap-1">
              <span className="font-medium text-zinc-100">{pref.agent}</span>
              <span className="text-zinc-500">{BILLING_LABELS[pref.billing_type]}</span>
            </div>
            <button
              className="inline-flex items-center justify-center gap-2 rounded border border-line px-3 py-2 text-sm text-zinc-300 hover:border-accent hover:text-accent disabled:opacity-60"
              onClick={() => {
                setAgent(pref.agent);
                setBillingType(pref.billing_type);
              }}
            >
              Edit
            </button>
            <button
              className="inline-flex items-center justify-center gap-2 rounded border border-line px-3 py-2 text-sm text-zinc-300 hover:border-red-400 hover:text-red-300 disabled:opacity-60 sm:col-start-2"
              onClick={() => removePref.mutate(pref.agent)}
              disabled={removePref.isPending}
            >
              <Trash2 size={16} /> Remove
            </button>
          </div>
        ))}
        {billingPrefs.data?.data.length === 0 ? <div className="p-3 text-sm text-zinc-500">No billing overrides yet. Add one above to mark an agent as subscription or API.</div> : null}
      </div>
      {savePref.error ? <p className="mt-3 text-sm text-red-300">{savePref.error.message}</p> : null}
      {removePref.error ? <p className="mt-3 text-sm text-red-300">{removePref.error.message}</p> : null}
    </section>
  );
}
