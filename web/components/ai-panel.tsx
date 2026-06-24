"use client";

import type { AIMetrics } from "@/lib/api";
import { aiAgentDonutRows } from "@/lib/ai-agent-donut";
import { aiDayPercentage, aiHeatmapClass, aiHeatmapTitle, formatCents } from "@/lib/ai-heatmap";
import { aiRingStyle } from "@/lib/ai-ring";
import { fallbackPalette } from "@/lib/language-colors";
import { compactNumber } from "@/lib/number-format";
import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from "recharts";

export function AIPanel({ metrics }: { metrics?: AIMetrics }) {
  const ai = metrics ?? {
    ai_line_changes: 0,
    human_line_changes: 0,
    ai_percentage: 0,
    human_review_percentage: 0,
    follow_up_edits: 0,
    ai_input_tokens: 0,
    ai_output_tokens: 0,
    ai_prompt_length: 0,
    prompt_count: 0,
    average_prompt_length: 0,
    median_prompt_length: 0,
    session_count: 0,
    estimated_cost_cents: 0,
    agents: [],
    days: [],
    costs: []
  };
  const totalTokens = ai.ai_input_tokens + ai.ai_output_tokens;
  const activeDays = ai.days.filter((day) => day.ai_line_changes > 0 || day.ai_input_tokens > 0 || day.estimated_cost_cents > 0);
  const heatmapDays = ai.days.slice(-35);
  const fullAIDays = heatmapDays.filter((day) => aiDayPercentage(day) === 100).length;

  return (
    <section className="rounded-md border border-line bg-panel/95 p-5 shadow-[0_1px_0_rgba(255,255,255,0.04)]">
      <div className="flex flex-col justify-between gap-5 lg:flex-row">
        <div className="flex items-center gap-4">
          <AICodingRing percentage={ai.ai_percentage} />
          <div>
            <div className="text-xs uppercase tracking-[0.18em] text-zinc-500">AI coding</div>
            <div className="flex items-end gap-3">
              <div className="text-5xl font-semibold text-zinc-50">{ai.ai_percentage}%</div>
              <div className="pb-2 text-sm text-zinc-400">AI line share</div>
            </div>
          </div>
        </div>
        <div className="grid flex-1 gap-3 sm:grid-cols-3 xl:grid-cols-6">
          <Metric label="AI lines" value={compactNumber(ai.ai_line_changes)} />
          <Metric label="Human lines" value={compactNumber(ai.human_line_changes)} />
          <Metric label="Review rate" value={`${ai.human_review_percentage}%`} />
          <Metric label="Sessions" value={compactNumber(ai.session_count)} />
          <Metric label="Tokens" value={compactNumber(totalTokens)} />
          <Metric label="Cost" value={formatCents(ai.estimated_cost_cents)} />
        </div>
      </div>

      <div className="mt-5 grid gap-3 md:grid-cols-3 xl:grid-cols-6">
        <Metric label="Input tokens" value={compactNumber(ai.ai_input_tokens)} />
        <Metric label="Output tokens" value={compactNumber(ai.ai_output_tokens)} />
        <Metric label="Prompts" value={compactNumber(ai.prompt_count)} />
        <Metric label="Avg prompt" value={`${compactNumber(ai.average_prompt_length)} chars`} />
        <Metric label="Median prompt" value={`${compactNumber(ai.median_prompt_length)} chars`} />
        <Metric label="Follow-up edits" value={compactNumber(ai.follow_up_edits)} />
      </div>

      <CostTracker rows={ai.costs ?? []} />

      <div className="mt-5 grid gap-5 xl:grid-cols-[1fr_1fr_0.9fr]">
        <div className="rounded-md border border-line bg-ink p-4">
          <div className="flex items-center justify-between gap-4">
            <div>
              <h3 className="text-sm font-medium text-zinc-200">Agent breakdown</h3>
              <p className="mt-1 text-xs text-zinc-500">{activeDays.length.toLocaleString()} active AI days in this range</p>
            </div>
          </div>
          <div className="mt-4 divide-y divide-line">
            {(ai.agents.length ? ai.agents : [emptyAgent()]).slice(0, 6).map((agent) => (
              <div key={agent.name} className="grid grid-cols-[1fr_96px_84px] gap-3 py-3 text-sm">
                <span className="truncate font-medium text-zinc-100">{agent.name}</span>
                <span className="text-right text-zinc-400">{compactNumber(agent.ai_line_changes)} lines</span>
                <span className="text-right text-zinc-500">{formatCents(agent.estimated_cost_cents)}</span>
                <span className="col-span-3 text-xs text-zinc-600">
                  {compactNumber(agent.session_count)} sessions · {compactNumber(agent.ai_input_tokens + agent.ai_output_tokens)} tokens · {agent.session_count > 0 ? compactNumber(Math.round(agent.ai_prompt_length / agent.session_count)) : 0} avg prompt
                </span>
              </div>
            ))}
          </div>
        </div>

        <AgentsDonut agents={ai.agents} />

        <div className="rounded-md border border-line bg-ink p-4">
          <div className="flex items-center justify-between gap-4">
            <div>
              <h3 className="text-sm font-medium text-zinc-200">AI heatmap</h3>
              <p className="mt-1 text-xs text-zinc-500">{formatCents(sumCosts(heatmapDays))} across visible days · {fullAIDays} fully AI days</p>
            </div>
          </div>
          <div className="mt-4 grid grid-cols-7 gap-1.5">
            {heatmapDays.map((day) => (
              <div
                key={day.name}
                className={`aspect-square rounded border transition ${aiHeatmapClass(day)}`}
                title={aiHeatmapTitle(day)}
              />
            ))}
            {heatmapDays.length === 0 ? <div className="col-span-7 text-sm text-zinc-500">No AI activity yet.</div> : null}
          </div>
        </div>
      </div>
    </section>
  );
}

function AICodingRing({ percentage }: { percentage: number }) {
  return (
    <div className="grid h-20 w-20 shrink-0 place-items-center rounded-full border border-accent/30 p-1 shadow-[0_0_28px_rgba(47,155,255,0.18)]" style={aiRingStyle(percentage)}>
      <div className="grid h-full w-full place-items-center rounded-full bg-panel text-sm font-semibold text-zinc-100">{Math.max(0, Math.min(100, Math.round(percentage)))}%</div>
    </div>
  );
}

function AgentsDonut({ agents }: { agents: AIMetrics["agents"] }) {
  const rows = aiAgentDonutRows(agents);
  const hasData = rows.some((row) => row.value > 0);
  return (
    <div className="rounded-md border border-line bg-ink p-4">
      <div className="mb-3 flex items-center justify-between gap-4">
        <div>
          <h3 className="text-sm font-medium text-zinc-200">Agents donut</h3>
          <p className="mt-1 text-xs text-zinc-500">AI line changes by agent</p>
        </div>
      </div>
      <div className="grid gap-4 sm:grid-cols-[140px_1fr] xl:grid-cols-1 2xl:grid-cols-[140px_1fr]">
        <div className="h-36">
          <ResponsiveContainer width="100%" height="100%">
            <PieChart>
              <Pie data={rows} dataKey="value" nameKey="name" innerRadius={42} outerRadius={62} paddingAngle={3}>
                {rows.map((row, index) => (
                  <Cell key={row.name} fill={hasData ? fallbackPalette[index % fallbackPalette.length] : "#26262b"} />
                ))}
              </Pie>
              <Tooltip contentStyle={{ background: "#0f1722", border: "1px solid #223040", borderRadius: 6 }} />
            </PieChart>
          </ResponsiveContainer>
        </div>
        <div className="space-y-3 self-center">
          {rows.slice(0, 5).map((row, index) => (
            <div key={row.name} className="flex items-center justify-between gap-3 text-sm">
              <span className="flex min-w-0 items-center gap-2 text-zinc-300">
                <span className="h-2.5 w-2.5 shrink-0 rounded-sm" style={{ background: hasData ? fallbackPalette[index % fallbackPalette.length] : "#26262b" }} />
                <span className="truncate">{row.name}</span>
              </span>
              <span className="shrink-0 text-zinc-500">{row.label}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function CostTracker({ rows }: { rows: NonNullable<AIMetrics["costs"]> }) {
  const visibleRows = rows.length ? rows.slice(0, 5) : [{ agent: "No cost data", daily_cents: 0, weekly_cents: 0, monthly_cents: 0, total_cents: 0 }];
  return (
    <div className="mt-5 rounded-md border border-line bg-ink p-4">
      <div className="mb-3 flex items-center justify-between gap-3">
        <h3 className="text-sm font-medium text-zinc-200">Cost tracker</h3>
        <span className="text-xs text-zinc-500">Daily / weekly / monthly</span>
      </div>
      <div className="overflow-x-auto">
        <div className="min-w-[520px] divide-y divide-line">
          <div className="grid grid-cols-[1fr_82px_82px_82px_82px] gap-3 pb-2 text-xs uppercase tracking-[0.14em] text-zinc-600">
            <span>Agent</span>
            <span className="text-right">Daily</span>
            <span className="text-right">Weekly</span>
            <span className="text-right">Monthly</span>
            <span className="text-right">Total</span>
          </div>
          {visibleRows.map((row) => (
            <div key={row.agent} className="grid grid-cols-[1fr_82px_82px_82px_82px] gap-3 py-2 text-sm">
              <span className="truncate font-medium text-zinc-100">{row.agent}</span>
              <span className="text-right text-zinc-500">{formatCents(row.daily_cents)}</span>
              <span className="text-right text-zinc-500">{formatCents(row.weekly_cents)}</span>
              <span className="text-right text-zinc-500">{formatCents(row.monthly_cents)}</span>
              <span className="text-right text-zinc-300">{formatCents(row.total_cents)}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-line bg-ink px-3 py-2">
      <div className="text-xs text-zinc-500">{label}</div>
      <div className="mt-1 text-lg font-semibold text-zinc-100">{value}</div>
    </div>
  );
}

function emptyAgent() {
  return {
    name: "No AI agent data",
    ai_seconds: 0,
    ai_line_changes: 0,
    human_line_changes: 0,
    ai_input_tokens: 0,
    ai_output_tokens: 0,
    ai_prompt_length: 0,
    session_count: 0,
    estimated_cost_cents: 0
  };
}

function sumCosts(days: Array<{ estimated_cost_cents: number }>) {
  return days.reduce((sum, day) => sum + day.estimated_cost_cents, 0);
}
