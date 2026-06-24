"use client";
import Link from "next/link";
import { ArrowRight } from "lucide-react";
import type { ReactNode } from "react";

export function Container({ children, size = "7xl", className = "" }:
  { children: ReactNode; size?: "6xl" | "7xl"; className?: string }) {
  const max = size === "6xl" ? "max-w-6xl" : "max-w-7xl";
  return <div className={`mx-auto ${max} px-5 py-6 lg:px-8 ${className}`}>{children}</div>;
}

export function Panel({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <div className={`rounded border border-line bg-panel/95 p-5 shadow-[0_1px_0_rgba(255,255,255,0.04)] ${className}`}>{children}</div>;
}

export function HeaderReadout({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-36 rounded border border-line bg-ink px-3 py-2">
      <div className="text-xs uppercase tracking-[0.14em] text-zinc-500">{label}</div>
      <div className="mt-1 truncate text-lg font-semibold text-zinc-100">{value}</div>
    </div>
  );
}

// Generic segmented control. Caller supplies the grid wrapper via className.
// `optionTitle` is optional so callers that rendered a native tooltip per button
// (e.g. the ai-costs cost-mode control) keep byte-identical markup.
export function SegmentedToggle<T extends string>({ options, value, onChange, className = "", size = "sm", optionTitle }:
  { options: ReadonlyArray<{ value: T; label: string }>; value: T; onChange: (v: T) => void; className?: string; size?: "sm" | "xs"; optionTitle?: (option: { value: T; label: string }) => string; }) {
  const pad = size === "xs" ? "px-3 py-2 text-xs" : "px-3 py-2 text-sm";
  return (
    <div className={className}>
      {options.map((o) => (
        <button key={o.value} type="button" aria-pressed={value === o.value}
          title={optionTitle?.(o)}
          onClick={() => onChange(o.value)}
          className={`rounded border ${pad} transition ${value === o.value
            ? "border-accent bg-accent text-ink"
            : "border-line bg-ink text-zinc-300 hover:border-zinc-500"}`}>
          {o.label}
        </button>
      ))}
    </div>
  );
}

export function AuthGate({ message }: { message: string }) {
  return (
    <div className="grid min-h-screen place-items-center px-6">
      <div className="max-w-md rounded border border-line bg-panel p-6">
        <h1 className="text-xl font-semibold">Login required</h1>
        <p className="mt-2 text-sm text-zinc-400">{message}</p>
        <Link className="mt-5 inline-flex items-center gap-2 rounded bg-accent px-4 py-2 font-medium text-ink" href="/login">
          Login <ArrowRight size={16} />
        </Link>
      </div>
    </div>
  );
}
