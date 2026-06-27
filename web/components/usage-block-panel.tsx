import { Activity, Flame } from "lucide-react";
import type { UsageCurrentBlock } from "@/lib/usage-api";
import { blockProgress } from "@/lib/usage-billing";
import { compactNumber, formatUSD } from "@/lib/number-format";

// Visual current-block card: a 5-hour elapsed progress bar, the live burn rate,
// and projected block / day / month costs. Inactive and missing states are
// handled inline so the panel never collapses or shifts layout.
export function UsageBlockPanel({ block }: { block: UsageCurrentBlock | null }) {
  if (!block || !block.is_active) {
    return (
      <div className="rounded border border-line bg-panel/95 p-5 shadow-[0_1px_0_rgba(255,255,255,0.04)]">
        <div className="mb-4 flex items-center gap-2 text-sm font-medium text-zinc-300">
          <span className="text-accent"><Flame size={16} /></span>
          Current 5-hour block
        </div>
        <div className="grid min-h-32 place-items-center rounded border border-dashed border-line bg-ink/40 p-6 text-center">
          <div className="text-sm text-zinc-500">No active block</div>
          <p className="mt-1 max-w-xs text-xs text-zinc-600">Start coding to open a new 5-hour usage window and watch the burn rate live.</p>
        </div>
      </div>
    );
  }

  const progress = blockProgress(block);
  const endLabel = new Date(block.end).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });

  return (
    <div className="rounded border border-line bg-panel/95 p-5 shadow-[0_1px_0_rgba(255,255,255,0.04)]">
      <div className="mb-4 flex items-center justify-between gap-3">
        <div className="flex items-center gap-2 text-sm font-medium text-zinc-300">
          <span className="text-accent"><Flame size={16} /></span>
          Current 5-hour block
        </div>
        <span className="inline-flex items-center gap-1.5 rounded-full border border-accent/30 bg-accent/10 px-2 py-0.5 text-[11px] font-medium text-accent">
          <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-accent" /> Active
        </span>
      </div>

      <div className="flex items-end justify-between gap-3">
        <div>
          <div className="text-xs uppercase tracking-[0.14em] text-zinc-500">Spent this block</div>
          <div className="mt-1 text-3xl font-semibold tracking-tight text-zinc-50">{formatUSD(block.cost_usd)}</div>
        </div>
        <div className="text-right text-xs text-zinc-500">
          {compactNumber(block.tokens)} tokens
        </div>
      </div>

      <div className="mt-4">
        <div className="mb-1.5 flex items-center justify-between text-xs text-zinc-500">
          <span>{progress.elapsedMinutes} min elapsed</span>
          <span>ends {endLabel} · {progress.remainingMinutes} min left</span>
        </div>
        <div className="h-2.5 overflow-hidden rounded-full bg-white/5" role="progressbar" aria-valuenow={Math.round(progress.percent)} aria-valuemin={0} aria-valuemax={100}>
          <div className="h-full origin-left rounded-full bg-accent transition-transform duration-700" style={{ transform: `scaleX(${progress.percent / 100})` }} />
        </div>
      </div>

      <div className="mt-4 flex items-center gap-2 rounded border border-line bg-ink/50 px-3 py-2 text-sm">
        <Activity size={14} className="shrink-0 text-accent" />
        <span className="font-medium text-zinc-200">{formatUSD(block.burn_rate_cost_per_hour)}/hr</span>
        <span className="text-zinc-600">·</span>
        <span className="text-zinc-400">{Math.round(block.burn_rate_tokens_per_min).toLocaleString()} tok/min</span>
      </div>

      <div className="mt-4 grid grid-cols-3 gap-2">
        <ProjectedStat label="Block end" value={formatUSD(block.projected_block_cost_usd)} />
        <ProjectedStat label="Today" value={formatUSD(block.projected_day_cost_usd)} />
        <ProjectedStat label="This month" value={formatUSD(block.projected_month_cost_usd)} />
      </div>
      <p className="mt-2 text-[11px] leading-4 text-zinc-600">Projections extrapolate the current burn rate.</p>
    </div>
  );
}

function ProjectedStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-line bg-ink/50 px-2.5 py-2">
      <div className="truncate text-[10px] uppercase tracking-[0.12em] text-zinc-500">{label}</div>
      <div className="mt-1 truncate text-sm font-semibold text-zinc-100">{value}</div>
    </div>
  );
}
