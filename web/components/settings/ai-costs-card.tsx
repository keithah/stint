"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Save } from "lucide-react";
import { useState } from "react";
import { listAICosts, replaceAICosts } from "@/lib/api";

export function AICostsCard() {
  const client = useQueryClient();
  const aiCosts = useQuery({ queryKey: ["ai-costs"], queryFn: listAICosts, retry: false });
  const [costAgent, setCostAgent] = useState("Codex");
  const [inputCost, setInputCost] = useState(3);
  const [outputCost, setOutputCost] = useState(12);
  const canSaveAICosts = costAgent.trim().length > 0 && Number.isFinite(inputCost) && Number.isFinite(outputCost) && inputCost >= 0 && outputCost >= 0;
  const saveCosts = useMutation({
    mutationFn: () => replaceAICosts([{ agent: costAgent.trim(), input_cost_per_million_cents: inputCost, output_cost_per_million_cents: outputCost }]),
    onSuccess: () => client.invalidateQueries({ queryKey: ["ai-costs"] })
  });

  return (
    <section className="mt-5 rounded border border-line bg-panel p-5">
      <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-end">
        <div>
          <h2 className="font-medium">AI model cost rates</h2>
          <p className="mt-1 text-sm text-zinc-400">Estimate spend from stored AI input/output token counts by model or agent.</p>
        </div>
        <button className="inline-flex items-center justify-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60" onClick={() => saveCosts.mutate()} disabled={saveCosts.isPending || !canSaveAICosts}>
          <Save size={16} /> Save rates
        </button>
      </div>
      <div className="mt-4 grid gap-3 md:grid-cols-3">
        <label className="block">
          <span className="text-sm text-zinc-400">Model or agent</span>
          <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={costAgent} onChange={(event) => setCostAgent(event.target.value)} />
        </label>
        <label className="block">
          <span className="text-sm text-zinc-400">Input cents / 1M</span>
          <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" type="number" min={0} value={inputCost} onChange={(event) => setInputCost(Math.max(0, Number(event.target.value)))} />
        </label>
        <label className="block">
          <span className="text-sm text-zinc-400">Output cents / 1M</span>
          <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" type="number" min={0} value={outputCost} onChange={(event) => setOutputCost(Math.max(0, Number(event.target.value)))} />
        </label>
      </div>
      <div className="mt-4 divide-y divide-line rounded border border-line">
        {(aiCosts.data?.data ?? []).map((setting) => (
          <div key={setting.agent} className="grid gap-2 px-3 py-3 text-sm sm:grid-cols-3">
            <span className="font-medium text-zinc-100">{setting.agent}</span>
            <span className="text-zinc-500">Input {setting.input_cost_per_million_cents}c / 1M</span>
            <span className="text-zinc-500">Output {setting.output_cost_per_million_cents}c / 1M</span>
          </div>
        ))}
        {aiCosts.data?.data.length === 0 ? <div className="p-3 text-sm text-zinc-500">Default rates are active.</div> : null}
      </div>
      {saveCosts.error ? <p className="mt-3 text-sm text-red-300">{saveCosts.error.message}</p> : null}
    </section>
  );
}
