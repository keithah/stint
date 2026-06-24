import { formatUSD, compactNumber } from "@/lib/number-format";
import type { UsageSlice } from "@/lib/usage-api";
import { billingBadge } from "@/lib/usage-billing";

// A ranked cost breakdown with a proportion bar per row. Optionally clickable to
// drive drill-down (agents), and optionally badging each row's effective billing
// (subscription vs API) derived from cost vs marginal.
export function UsageBreakdown({
  title,
  rows,
  showBilling = false,
  onSelect,
  selected
}: {
  title: string;
  rows: UsageSlice[];
  showBilling?: boolean;
  onSelect?: (name: string) => void;
  selected?: string | null;
}) {
  const ranked = [...rows].sort((a, b) => b.cost_usd - a.cost_usd).slice(0, 8);
  const max = Math.max(1e-9, ...ranked.map((row) => row.cost_usd));

  return (
    <div className="ops-chart-panel rounded border border-line bg-panel/95 p-4 shadow-[0_1px_0_rgba(255,255,255,0.04)]">
      <div className="mb-4 flex items-center justify-between gap-3">
        <div className="text-sm font-medium text-zinc-300">{title}</div>
        {onSelect ? <div className="text-xs text-zinc-600">Click to filter</div> : null}
      </div>

      {ranked.length > 0 ? (
        <div className="space-y-3">
          {ranked.map((row) => {
            const badge = billingBadge(row);
            const interactive = Boolean(onSelect);
            const isSelected = selected === row.name;
            const Tag = interactive ? "button" : "div";
            return (
              <Tag
                key={row.name}
                type={interactive ? "button" : undefined}
                onClick={interactive ? () => onSelect?.(row.name) : undefined}
                className={`block w-full text-left transition ${interactive ? "cursor-pointer rounded px-1 -mx-1 py-1 hover:bg-white/5" : ""} ${isSelected ? "rounded bg-accent/10 px-1 -mx-1 py-1 ring-1 ring-accent/40" : ""}`}
              >
                <div className="mb-1.5 flex items-center justify-between gap-3 text-sm">
                  <span className="flex min-w-0 items-center gap-2 text-zinc-300">
                    <span className="truncate">{row.name}</span>
                    {showBilling ? (
                      <span className={`shrink-0 rounded border px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide ${badge.className}`}>
                        {badge.label}
                      </span>
                    ) : null}
                  </span>
                  <span className="shrink-0 tabular-nums text-zinc-400">{formatUSD(row.cost_usd)}</span>
                </div>
                <div className="h-1.5 overflow-hidden rounded bg-white/5">
                  <div className="h-full rounded bg-accent/70" style={{ width: `${Math.max(2, (row.cost_usd / max) * 100)}%` }} />
                </div>
                <div className="mt-1 flex items-center justify-between text-[11px] text-zinc-600">
                  <span>{compactNumber(row.tokens)} tok · {row.event_count.toLocaleString()} events</span>
                  {showBilling && badge.kind === "subscription" && row.cost_usd > row.marginal_usd ? (
                    <span className="text-moss/80">{formatUSD(row.marginal_usd)} out-of-pocket</span>
                  ) : null}
                </div>
              </Tag>
            );
          })}
        </div>
      ) : (
        <p className="text-sm text-zinc-500">No data in this range.</p>
      )}
    </div>
  );
}
