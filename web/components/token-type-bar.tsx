import { compactNumber } from "@/lib/number-format";
import type { UsageTotal } from "@/lib/usage-api";
import { boundedPercent } from "@/lib/chart-percent";

type Segment = {
  key: string;
  label: string;
  tokens: number;
  className: string;
  swatch: string;
};

// A labeled, single stacked bar makes the cache-vs-fresh share legible at a
// glance — far clearer than a donut for "how much context came from cache".
// Cache-read is the cheap, high-volume segment; fresh input and output are the
// billed-at-full-price segments.
export function TokenTypeBar({ total }: { total: UsageTotal }) {
  const segments: Segment[] = [
    { key: "input", label: "Input (fresh)", tokens: Math.max(0, total.input_tokens), className: "bg-accent", swatch: "#00b4d8" },
    { key: "output", label: "Output", tokens: Math.max(0, total.output_tokens), className: "bg-moss", swatch: "#84cc16" },
    { key: "cache_create", label: "Cache create", tokens: Math.max(0, total.cache_create_tokens), className: "bg-ember", swatch: "#f97316" },
    { key: "cache_read", label: "Cache read", tokens: Math.max(0, total.cache_read_tokens), className: "bg-zinc-500", swatch: "#71717a" },
    { key: "reasoning", label: "Reasoning", tokens: Math.max(0, total.reasoning_tokens), className: "bg-violet-500", swatch: "#8b5cf6" }
  ].filter((segment) => segment.tokens > 0);

  const sum = segments.reduce((acc, segment) => acc + segment.tokens, 0);
  const cacheRead = Math.max(0, total.cache_read_tokens);
  const cacheShare = sum > 0 ? cacheRead / sum : 0;

  return (
    <div className="ops-chart-panel rounded border border-line bg-panel/95 p-4 shadow-[0_1px_0_rgba(255,255,255,0.04)]">
      <div className="mb-4 flex items-center justify-between gap-3">
        <div className="text-sm font-medium text-zinc-300">Token type mix</div>
        <div className="text-xs text-zinc-500">{(cacheShare * 100).toFixed(0)}% from cache</div>
      </div>

      {sum > 0 ? (
        <>
          <div className="flex h-4 overflow-hidden rounded-full bg-white/5">
            {segments.map((segment) => (
              <div
                key={segment.key}
                className={`${segment.className} h-full first:rounded-l-full last:rounded-r-full`}
                style={{ width: `${boundedPercent((segment.tokens / sum) * 100)}%` }}
                title={`${segment.label}: ${compactNumber(segment.tokens)} tokens (${boundedPercent((segment.tokens / sum) * 100)}%)`}
              />
            ))}
          </div>

          <div className="mt-4 grid gap-2.5 sm:grid-cols-2">
            {segments.map((segment) => (
              <div key={segment.key} className="flex items-center justify-between gap-3 text-sm">
                <span className="flex min-w-0 items-center gap-2 text-zinc-300">
                  <span className="h-2.5 w-2.5 shrink-0 rounded-sm" style={{ background: segment.swatch }} />
                  <span className="truncate">{segment.label}</span>
                </span>
                <span className="shrink-0 tabular-nums text-zinc-500">
                  {compactNumber(segment.tokens)} · {boundedPercent((segment.tokens / sum) * 100)}%
                </span>
              </div>
            ))}
          </div>
        </>
      ) : (
        <p className="text-sm text-zinc-500">No token usage recorded in this range.</p>
      )}
    </div>
  );
}
