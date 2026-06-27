"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RotateCcw, Save, Trash2 } from "lucide-react";
import { useState } from "react";
import { abortCustomRulesProgress, customRulesProgress, deleteCustomRule, listCustomRules, replaceCustomRules } from "@/lib/api";
import { boundedPercent } from "@/lib/chart-percent";
import { SecondaryButton } from "@/components/ui";

const ruleFields = ["entity", "type", "category", "project", "branch", "language", "editor", "operating_system"];
const ruleOperations = [
  { value: "equals", label: "Equals" },
  { value: "contains", label: "Contains" },
  { value: "starts_with", label: "Starts with" },
  { value: "ends_with", label: "Ends with" },
  { value: "regex", label: "Regex" }
];

export function CustomRulesCard() {
  const client = useQueryClient();
  const customRules = useQuery({ queryKey: ["custom-rules"], queryFn: listCustomRules, });
	  const ruleProgress = useQuery({ queryKey: ["custom-rules-progress"], queryFn: customRulesProgress });
  const [ruleAction, setRuleAction] = useState<"change" | "delete">("change");
  const [ruleSource, setRuleSource] = useState("entity");
  const [ruleOperation, setRuleOperation] = useState("contains");
  const [ruleSourceValue, setRuleSourceValue] = useState("legacy");
  const [ruleDestination, setRuleDestination] = useState("project");
  const [ruleDestinationValue, setRuleDestinationValue] = useState("modernized");
  const [rulePriority, setRulePriority] = useState(1);
  const canSaveCustomRule = ruleSourceValue.trim().length > 0 && Number.isFinite(rulePriority) && rulePriority >= 1 && (ruleAction === "delete" || ruleDestinationValue.trim().length > 0);
  const saveRule = useMutation({
    mutationFn: () =>
      replaceCustomRules([
        ...(customRules.data?.data ?? []),
        {
          action: ruleAction,
          source: ruleSource,
          operation: ruleOperation,
          source_value: ruleSourceValue.trim(),
          priority: rulePriority,
          destinations: ruleAction === "change" ? [{ destination: ruleDestination, destination_value: ruleDestinationValue.trim() }] : []
        }
      ]),
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ["custom-rules"] });
      client.invalidateQueries({ queryKey: ["custom-rules-progress"] });
    }
  });
  const removeCustomRule = useMutation({
    mutationFn: deleteCustomRule,
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ["custom-rules"] });
      client.invalidateQueries({ queryKey: ["custom-rules-progress"] });
    }
  });
  const abortRuleProgress = useMutation({
    mutationFn: abortCustomRulesProgress,
    onSuccess: () => client.invalidateQueries({ queryKey: ["custom-rules-progress"] })
  });

  return (
    <section className="mt-5 rounded border border-line bg-panel p-5">
      <h2 className="font-medium">Custom rules</h2>
      <p className="mt-1 text-sm text-zinc-400">Apply personal rewrite or delete rules before heartbeats are stored, then reprocess existing rows.</p>
      <div className="mt-4 grid gap-3 lg:grid-cols-[120px_150px_150px_1fr]">
        <label className="block">
          <span className="text-sm text-zinc-400">Action</span>
          <select className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={ruleAction} onChange={(event) => setRuleAction(event.target.value as "change" | "delete")}>
            <option value="change">Change</option>
            <option value="delete">Delete</option>
          </select>
        </label>
        <label className="block">
          <span className="text-sm text-zinc-400">Source</span>
          <select className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={ruleSource} onChange={(event) => setRuleSource(event.target.value)}>
            {ruleFields.map((field) => (
              <option key={field} value={field}>{field}</option>
            ))}
          </select>
        </label>
        <label className="block">
          <span className="text-sm text-zinc-400">Operation</span>
          <select className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={ruleOperation} onChange={(event) => setRuleOperation(event.target.value)}>
            {ruleOperations.map((operation) => (
              <option key={operation.value} value={operation.value}>{operation.label}</option>
            ))}
          </select>
        </label>
        <label className="block">
          <span className="text-sm text-zinc-400">Match value</span>
          <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" value={ruleSourceValue} onChange={(event) => setRuleSourceValue(event.target.value)} />
        </label>
      </div>
      <div className="mt-3 grid gap-3 lg:grid-cols-[150px_1fr_120px_auto]">
        <label className="block">
          <span className="text-sm text-zinc-400">Destination</span>
          <select
            className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent disabled:opacity-50"
            value={ruleDestination}
            onChange={(event) => setRuleDestination(event.target.value)}
            disabled={ruleAction === "delete"}
          >
            {ruleFields.map((field) => (
              <option key={field} value={field}>{field}</option>
            ))}
          </select>
        </label>
        <label className="block">
          <span className="text-sm text-zinc-400">Destination value</span>
          <input
            className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent disabled:opacity-50"
            value={ruleDestinationValue}
            onChange={(event) => setRuleDestinationValue(event.target.value)}
            disabled={ruleAction === "delete"}
          />
        </label>
        <label className="block">
          <span className="text-sm text-zinc-400">Priority</span>
          <input className="mt-2 w-full rounded border border-line bg-ink px-3 py-2 text-sm outline-none focus:border-accent" type="number" min={1} value={rulePriority} onChange={(event) => setRulePriority(Math.max(1, Number(event.target.value) || 1))} />
        </label>
        <button className="mt-7 inline-flex items-center justify-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink disabled:opacity-60" onClick={() => saveRule.mutate()} disabled={saveRule.isPending || !canSaveCustomRule}>
          <Save size={16} /> Add rule
        </button>
      </div>
      {saveRule.error ? <p className="mt-3 text-sm text-red-300">{saveRule.error.message}</p> : null}
      <div className="mt-4 divide-y divide-line rounded border border-line">
        {(customRules.data?.data ?? []).map((rule) => (
          <div key={rule.id ?? `${rule.source}-${rule.source_value}`} className="flex flex-col justify-between gap-3 px-3 py-3 text-sm sm:flex-row sm:items-center">
            <div>
              <span className="font-medium text-zinc-100">{rule.action}</span>
              <span className="ml-2 text-zinc-500">{rule.source} {rule.operation} {rule.source_value}</span>
              {rule.destinations?.length ? (
                <div className="mt-2 flex flex-wrap gap-2">
                  {rule.destinations.map((destination) => (
                    <span key={`${destination.destination}-${destination.destination_value}`} className="rounded border border-line bg-ink px-2 py-1 text-xs text-zinc-400">
                      {destination.destination}: {destination.destination_value}
                    </span>
                  ))}
                </div>
              ) : null}
            </div>
            {rule.id ? (
              <button className="inline-flex items-center justify-center gap-2 rounded border border-red-900/70 px-3 py-2 text-sm text-red-300 hover:bg-red-950/40" onClick={() => removeCustomRule.mutate(rule.id!)} disabled={removeCustomRule.isPending}>
                <Trash2 size={15} /> Delete
              </button>
            ) : null}
          </div>
        ))}
        {customRules.data?.data.length === 0 ? <div className="p-3 text-sm text-zinc-500">No rules saved yet.</div> : null}
      </div>
      <div className="mt-4 rounded border border-line bg-ink p-4">
        <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
          <div>
            <div className="text-sm font-medium text-zinc-100">Retroactive apply</div>
            <div className="mt-1 text-sm text-zinc-500">
              {ruleProgress.data?.data.status ?? "NotStarted"} · {ruleProgress.data?.data.percent_complete ?? 0}% · {ruleProgress.data?.data.changed ?? 0} changed · {ruleProgress.data?.data.deleted ?? 0} deleted
            </div>
          </div>
          <SecondaryButton
            className="disabled:opacity-60"
            onClick={() => abortRuleProgress.mutate()}
            disabled={abortRuleProgress.isPending}
          >
            <RotateCcw size={15} /> Abort job
          </SecondaryButton>
        </div>
        <div className="mt-3 h-2 overflow-hidden rounded bg-white/5">
          <div className="h-full rounded bg-accent" style={{ width: `${boundedPercent(ruleProgress.data?.data.percent_complete ?? 0)}%` }} />
        </div>
        {ruleProgress.data?.data.error ? <p className="mt-3 text-sm text-red-300">{ruleProgress.data.data.error}</p> : null}
      </div>
    </section>
  );
}
