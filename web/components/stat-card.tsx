export function StatCard({ label, value, detail }: { label: string; value: string; detail?: string }) {
  return (
    <div className="rounded-md border border-line bg-panel/95 p-5 shadow-[0_1px_0_rgba(255,255,255,0.04)]">
      <div className="text-xs uppercase tracking-[0.16em] text-zinc-500">{label}</div>
      <div className="mt-3 text-3xl font-semibold tracking-tight text-zinc-50">{value}</div>
      {detail ? <div className="mt-2 text-sm text-zinc-400">{detail}</div> : null}
    </div>
  );
}
