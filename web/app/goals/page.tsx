"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, Goal as GoalIcon, PauseCircle, Pencil, Plus, Power, Save, Trash2, X } from "lucide-react";
import { useState, type ReactNode } from "react";
import { AppShell } from "@/components/app-shell";
import { PageHeader, SecondaryButton } from "@/components/ui";
import { createGoal, deleteGoal, listGoals, updateGoal, type Goal, type GoalPayload, type GoalProgress } from "@/lib/api";
import { boundedPercent } from "@/lib/chart-percent";

const weekdayOptions = ["monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"];

type GoalDraft = {
  id: string;
  title: string;
  seconds: number;
  delta: "day" | "week";
  project: string;
  language: string;
  editor: string;
  ignoreDays: string[];
  ignoreZeroDays: boolean;
  isInverse: boolean;
  improveByPercent: string;
  isEnabled: boolean;
  isSnoozed: boolean;
  snoozeUntil: string;
};

export default function GoalsPage() {
  return (
    <AppShell>
      <GoalsContent />
    </AppShell>
  );
}

function GoalsContent() {
  const client = useQueryClient();
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [createDraft, setCreateDraft] = useState<GoalDraft>(() => defaultGoalDraft());
  const [editing, setEditing] = useState<GoalDraft | null>(null);
  const goals = useQuery({ queryKey: ["goals"], queryFn: listGoals, retry: false });
  const create = useMutation({
    mutationFn: () => createGoal(goalPayloadFromInputs(createDraft)),
    onSuccess: () => {
      setCreateDraft(defaultGoalDraft());
      setCreateModalOpen(false);
      client.invalidateQueries({ queryKey: ["goals"] });
    }
  });
  const update = useMutation({
    mutationFn: (draft: GoalDraft) => updateGoal(draft.id, goalPayloadFromInputs(draft)),
    onSuccess: () => {
      setEditing(null);
      client.invalidateQueries({ queryKey: ["goals"] });
    }
  });
  const toggleEnabled = useMutation({
    mutationFn: (goal: Goal) => updateGoal(goal.id, { ...goalPayloadFromGoal(goal), is_enabled: !goal.is_enabled }),
    onSuccess: () => client.invalidateQueries({ queryKey: ["goals"] })
  });
  const remove = useMutation({
    mutationFn: deleteGoal,
    onSuccess: () => client.invalidateQueries({ queryKey: ["goals"] })
  });

  return (
    <div className="mx-auto max-w-6xl px-5 py-6 lg:px-8">
      <PageHeader
        icon={<GoalIcon size={14} />}
        caption="Personal targets"
        title="Goals"
        sub="Track daily or weekly coding targets against real heartbeat durations."
        actions={
          <button className="inline-flex items-center justify-center gap-2 rounded bg-accent px-4 py-2 text-sm font-medium text-ink" onClick={() => setCreateModalOpen(true)}>
            <Plus size={16} /> Create goal
          </button>
        }
      />

      {createModalOpen ? (
        <GoalModal mode="create" title="Create goal" onClose={() => setCreateModalOpen(false)}>
          <GoalEditorFields draft={createDraft} onChange={setCreateDraft} />
          <div className="mt-6 flex justify-end gap-2">
            <SecondaryButton onClick={() => setCreateModalOpen(false)}>
              <X size={15} /> Cancel
            </SecondaryButton>
            <button className="inline-flex items-center gap-2 rounded bg-accent px-3 py-2 text-sm font-medium text-ink disabled:opacity-60" onClick={() => create.mutate()} disabled={create.isPending || !validGoalDraft(createDraft)}>
              <Plus size={15} /> Create
            </button>
          </div>
        </GoalModal>
      ) : null}

      {editing ? (
        <GoalModal mode="edit" title="Edit goal" onClose={() => setEditing(null)}>
          <GoalEditorFields draft={editing} onChange={setEditing} />
          <div className="mt-6 flex justify-end gap-2">
            <SecondaryButton onClick={() => setEditing(null)}>
              <X size={15} /> Cancel
            </SecondaryButton>
            <button className="inline-flex items-center gap-2 rounded bg-accent px-3 py-2 text-sm font-medium text-ink disabled:opacity-60" onClick={() => update.mutate(editing)} disabled={update.isPending || !validGoalDraft(editing)}>
              <Save size={15} /> Save
            </button>
          </div>
        </GoalModal>
      ) : null}

      <section className="grid gap-4 lg:grid-cols-2">
        {(goals.data?.data ?? []).map((item) => {
          return (
            <div key={item.goal.id} className={`rounded border bg-panel p-5 ${item.goal.is_enabled ? "border-line" : "border-zinc-800 opacity-75"}`}>
              <div className="flex items-start justify-between gap-4">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <h2 className="font-medium text-zinc-100">{item.goal.custom_title || item.goal.title}</h2>
                    {item.is_complete ? <CheckCircle2 className="text-moss" size={16} /> : null}
                    {item.is_snoozed ? <PauseCircle className="text-amber-300" size={16} /> : null}
                    {!item.goal.is_enabled ? <span className="rounded border border-zinc-700 px-2 py-0.5 text-xs text-zinc-500">disabled</span> : null}
                  </div>
                  <p className="mt-1 text-sm text-zinc-500">
                    {item.goal.delta === "week" ? "Weekly" : "Daily"} target: {item.human_readable_target}
                    {item.goal.is_inverse ? " max" : ""}
                    {item.goal.improve_by_percent != null ? `, ${item.goal.improve_by_percent}% improvement` : ""}
                  </p>
                  <div className="mt-3 flex flex-wrap gap-2">
                    {goalTags(item).map((tag) => (
                      <span key={tag} className="rounded border border-line bg-ink px-2 py-1 text-xs text-zinc-400">
                        {tag}
                      </span>
                    ))}
                  </div>
                </div>
                <div className="flex shrink-0 gap-2">
                  <button className="rounded border border-line p-2 text-zinc-300 hover:bg-white/5" onClick={() => setEditing(draftFromGoal(item.goal))}>
                    <Pencil size={16} />
                  </button>
                  <button className="rounded border border-line p-2 text-zinc-300 hover:bg-white/5" onClick={() => toggleEnabled.mutate(item.goal)} disabled={toggleEnabled.isPending}>
                    <Power size={16} />
                  </button>
                  <button className="rounded border border-red-900/70 p-2 text-red-300 hover:bg-red-950/40" onClick={() => remove.mutate(item.goal.id)}>
                    <Trash2 size={16} />
                  </button>
                </div>
              </div>
              <div className="mt-6">
                <div className="mb-2 flex justify-between gap-3 text-sm">
                  <span className="text-zinc-300">{item.is_snoozed ? "Snoozed" : item.is_ignored ? "Ignored" : item.human_readable_actual}</span>
                  <span className={item.is_complete ? "text-moss" : "text-zinc-500"}>{item.percent}%</span>
                </div>
                <div className="h-2 overflow-hidden rounded bg-white/5">
                  <div className={`h-full rounded ${item.is_snoozed ? "bg-amber-300" : "bg-accent"}`} style={{ width: `${boundedPercent(item.percent)}%` }} />
                </div>
              </div>
            </div>
          );
        })}
        {goals.data?.data.length === 0 ? <div className="rounded border border-line bg-panel p-5 text-sm text-zinc-500">No goals yet.</div> : null}
      </section>
    </div>
  );
}

function GoalModal({ mode, title, onClose, children }: { mode: "create" | "edit"; title: string; onClose: () => void; children: ReactNode }) {
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/70 px-4 py-6 backdrop-blur-sm">
      <section className="max-h-[90vh] w-full max-w-4xl overflow-y-auto rounded border border-line bg-panel shadow-glow" role="dialog" aria-modal="true" aria-labelledby={`goal-${mode}-title`}>
        <div className="flex items-start justify-between gap-4 border-b border-line p-5">
          <div>
            <div className="mb-3 inline-flex items-center gap-2 rounded border border-accent/30 bg-accent/10 px-3 py-1 text-xs uppercase tracking-[0.18em] text-accent">
              <GoalIcon size={14} /> {mode === "create" ? "New target" : "Goal settings"}
            </div>
            <h2 id={`goal-${mode}-title`} className="text-2xl font-semibold tracking-tight">{title}</h2>
          </div>
          <button className="rounded border border-line p-2 text-zinc-400 hover:bg-white/5 hover:text-zinc-100" onClick={onClose} aria-label={`Close ${title}`}>
            <X size={18} />
          </button>
        </div>
        <div className="p-5">{children}</div>
      </section>
    </div>
  );
}

function GoalEditorFields({ draft, onChange }: { draft: GoalDraft; onChange: (draft: GoalDraft) => void }) {
  return (
    <div>
      <div className="grid gap-3 lg:grid-cols-[minmax(180px,1.4fr)_120px_120px_minmax(150px,1fr)]">
        <label className="grid gap-1 text-xs text-zinc-500">
          Title
          <input className="rounded border border-line bg-ink px-3 py-2 text-sm text-zinc-100 outline-none focus:border-accent" value={draft.title} onChange={(event) => onChange({ ...draft, title: event.target.value })} />
        </label>
        <label className="grid gap-1 text-xs text-zinc-500">
          Seconds
          <input className="rounded border border-line bg-ink px-3 py-2 text-sm text-zinc-100 outline-none focus:border-accent" type="number" min={0} step={60} value={draft.seconds} onChange={(event) => onChange({ ...draft, seconds: Number(event.target.value) })} />
        </label>
        <label className="grid gap-1 text-xs text-zinc-500">
          Period
          <select className="rounded border border-line bg-ink px-3 py-2 text-sm text-zinc-100 outline-none focus:border-accent" value={draft.delta} onChange={(event) => onChange({ ...draft, delta: event.target.value as "day" | "week" })}>
            <option value="day">Daily</option>
            <option value="week">Weekly</option>
          </select>
        </label>
        <label className="grid gap-1 text-xs text-zinc-500">
          Improve %
          <input className="rounded border border-line bg-ink px-3 py-2 text-sm text-zinc-100 outline-none focus:border-accent" type="number" min={0} step={1} placeholder="Optional" value={draft.improveByPercent} onChange={(event) => onChange({ ...draft, improveByPercent: event.target.value })} />
        </label>
      </div>

      <div className="mt-3 grid gap-3 md:grid-cols-3">
        <label className="grid gap-1 text-xs text-zinc-500">
          Projects
          <input className="rounded border border-line bg-ink px-3 py-2 text-sm text-zinc-100 outline-none focus:border-accent" placeholder="stint, api" value={draft.project} onChange={(event) => onChange({ ...draft, project: event.target.value })} />
        </label>
        <label className="grid gap-1 text-xs text-zinc-500">
          Languages
          <input className="rounded border border-line bg-ink px-3 py-2 text-sm text-zinc-100 outline-none focus:border-accent" placeholder="Go, TypeScript" value={draft.language} onChange={(event) => onChange({ ...draft, language: event.target.value })} />
        </label>
        <label className="grid gap-1 text-xs text-zinc-500">
          Editors
          <input className="rounded border border-line bg-ink px-3 py-2 text-sm text-zinc-100 outline-none focus:border-accent" placeholder="vscode, vim" value={draft.editor} onChange={(event) => onChange({ ...draft, editor: event.target.value })} />
        </label>
      </div>

      <div className="mt-4 flex flex-wrap gap-2">
        {weekdayOptions.map((day) => (
          <button
            key={day}
            className={`rounded border px-3 py-1.5 text-xs capitalize ${
              draft.ignoreDays.includes(day) ? "border-accent bg-accent/15 text-accent" : "border-line bg-ink text-zinc-400 hover:border-zinc-700"
            }`}
            type="button"
            onClick={() => onChange({ ...draft, ignoreDays: draft.ignoreDays.includes(day) ? draft.ignoreDays.filter((item) => item !== day) : [...draft.ignoreDays, day] })}
          >
            {day.slice(0, 3)}
          </button>
        ))}
      </div>

      <div className="mt-4 grid gap-2 sm:grid-cols-4">
        <ToggleLabel label="Enabled" checked={draft.isEnabled} onChange={(checked) => onChange({ ...draft, isEnabled: checked })} />
        <ToggleLabel label="Ignore zero" checked={draft.ignoreZeroDays} onChange={(checked) => onChange({ ...draft, ignoreZeroDays: checked })} />
        <ToggleLabel label="Under target" checked={draft.isInverse} onChange={(checked) => onChange({ ...draft, isInverse: checked })} />
        <ToggleLabel label="Snoozed" checked={draft.isSnoozed} onChange={(checked) => onChange({ ...draft, isSnoozed: checked })} />
      </div>

      {draft.isSnoozed ? (
        <label className="mt-3 grid max-w-sm gap-1 text-xs text-zinc-500">
          Snooze until
          <input className="rounded border border-line bg-ink px-3 py-2 text-sm text-zinc-100 outline-none focus:border-accent" type="datetime-local" value={draft.snoozeUntil} onChange={(event) => onChange({ ...draft, snoozeUntil: event.target.value })} />
        </label>
      ) : null}
    </div>
  );
}

function defaultGoalDraft(): GoalDraft {
  return {
    id: "create",
    title: "Code 1 hour",
    seconds: 3600,
    delta: "day",
    project: "",
    language: "",
    editor: "",
    ignoreDays: [],
    ignoreZeroDays: false,
    isInverse: false,
    improveByPercent: "",
    isEnabled: true,
    isSnoozed: false,
    snoozeUntil: ""
  };
}

function listFromInput(value: string) {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function validGoalDraft(draft: GoalDraft | null) {
  if (!draft) {
    return false;
  }
  const hasValidSeconds = Number.isFinite(draft.seconds) && draft.seconds >= 0;
  const improveByPercent = draft.improveByPercent.trim();
  const hasValidImprovement = improveByPercent === "" || (Number.isFinite(Number(improveByPercent)) && Number(draft.improveByPercent) >= 0);
  return hasValidSeconds && hasValidImprovement;
}

function ToggleLabel({ label, checked, onChange }: { label: string; checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <label className="flex min-h-10 items-center gap-2 rounded border border-line bg-ink px-3 text-sm text-zinc-300">
      <input className="accent-accent" type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} />
      {label}
    </label>
  );
}

function draftFromGoal(goal: Goal): GoalDraft {
  return {
    id: goal.id,
    title: goal.title,
    seconds: goal.seconds,
    delta: goal.delta,
    project: (goal.projects ?? []).join(", "),
    language: (goal.languages ?? []).join(", "),
    editor: (goal.editors ?? []).join(", "),
    ignoreDays: goal.ignore_days ?? [],
    ignoreZeroDays: goal.ignore_zero_days,
    isInverse: goal.is_inverse,
    improveByPercent: goal.improve_by_percent == null ? "" : String(goal.improve_by_percent),
    isEnabled: goal.is_enabled,
    isSnoozed: goal.is_snoozed,
    snoozeUntil: toDateTimeLocal(goal.snooze_until)
  };
}

function goalPayloadFromGoal(goal: Goal): GoalPayload {
  return {
    title: goal.title,
    custom_title: goal.custom_title,
    seconds: goal.seconds,
    delta: goal.delta,
    projects: goal.projects ?? [],
    languages: goal.languages ?? [],
    editors: goal.editors ?? [],
    ignore_days: goal.ignore_days ?? [],
    ignore_zero_days: goal.ignore_zero_days,
    improve_by_percent: goal.improve_by_percent,
    is_enabled: goal.is_enabled,
    is_inverse: goal.is_inverse,
    is_snoozed: goal.is_snoozed,
    snooze_until: goal.snooze_until
  };
}

function goalPayloadFromInputs(input: Omit<GoalDraft, "id">): GoalPayload {
  const improveByPercent = input.improveByPercent.trim();
  return {
    title: input.title,
    seconds: input.seconds,
    delta: input.delta,
    projects: listFromInput(input.project),
    languages: listFromInput(input.language),
    editors: listFromInput(input.editor),
    ignore_days: input.ignoreDays,
    ignore_zero_days: input.ignoreZeroDays,
    improve_by_percent: improveByPercent === "" ? undefined : Number(improveByPercent),
    is_enabled: input.isEnabled,
    is_inverse: input.isInverse,
    is_snoozed: input.isSnoozed,
    snooze_until: input.isSnoozed && input.snoozeUntil ? new Date(input.snoozeUntil).toISOString() : undefined
  };
}

function toDateTimeLocal(value?: string) {
  if (!value) {
    return "";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  const offsetMs = date.getTimezoneOffset() * 60 * 1000;
  return new Date(date.getTime() - offsetMs).toISOString().slice(0, 16);
}

function goalTags(item: GoalProgress) {
  const tags = [
    ...(item.goal.projects ?? []).map((project) => `project:${project}`),
    ...(item.goal.languages ?? []).map((language) => `lang:${language}`),
    ...(item.goal.editors ?? []).map((editor) => `editor:${editor}`),
    ...(item.goal.ignore_days ?? []).map((day) => `skip:${day.slice(0, 3)}`)
  ];
  if (item.goal.ignore_zero_days) {
    tags.push("skip:zero");
  }
  if (item.goal.snooze_until) {
    tags.push(`until:${new Date(item.goal.snooze_until).toLocaleDateString()}`);
  }
  if (item.is_ignored) {
    tags.push("ignored");
  }
  return tags;
}
