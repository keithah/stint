"use client";
import Link from "next/link";
import { ArrowRight } from "lucide-react";
import type { ReactNode } from "react";

// Loading placeholder. aria-hidden so screen readers don't announce the
// shimmer; the live region / final content carries the real semantics.
export function Skeleton({ className = "" }: { className?: string }) {
  return <div aria-hidden="true" className={`animate-pulse rounded bg-white/5 ${className}`} />;
}

// Consistent empty state: a dashed panel with an icon, title, hint, and an
// optional call-to-action. Replaces the one-off dashed divs scattered per page.
export function EmptyState({ icon, title, hint, action }:
  { icon?: ReactNode; title: string; hint?: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center gap-3 rounded border border-dashed border-line bg-panel/70 px-6 py-10 text-center">
      {icon ? <div className="flex h-10 w-10 items-center justify-center rounded bg-white/5 text-zinc-400">{icon}</div> : null}
      <div className="text-sm font-medium text-zinc-200">{title}</div>
      {hint ? <p className="max-w-sm text-sm leading-6 text-zinc-500">{hint}</p> : null}
      {action ? <div className="mt-1">{action}</div> : null}
    </div>
  );
}

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

// Generic segmented control. Caller supplies the grid/pill wrapper via className.
// `variant` "boxed" (default) is the original bordered-button grid; "pill" is the
// Direction-B inset pill (borderless segments, active fills with accent).
// `optionTitle` is optional so callers that rendered a native tooltip per button
// (e.g. the ai-costs cost-mode control) keep byte-identical markup.
export function SegmentedToggle<T extends string>({ options, value, onChange, className = "", size = "sm", variant = "boxed", optionTitle }:
  { options: ReadonlyArray<{ value: T; label: string }>; value: T; onChange: (v: T) => void; className?: string; size?: "sm" | "xs"; variant?: "boxed" | "pill"; optionTitle?: (option: { value: T; label: string }) => string; }) {
  const pad = size === "xs" ? "px-3 py-2 text-xs" : "px-3 py-2 text-sm";
  const button = (active: boolean) =>
    variant === "pill"
      ? `rounded-[3px] ${size === "xs" ? "px-2.5 py-1 text-xs" : "px-3 py-1.5 text-sm"} transition ${active
          ? "bg-accent text-ink"
          : "text-zinc-400 hover:text-zinc-100"}`
      : `rounded border ${pad} transition ${active
          ? "border-accent bg-accent text-ink"
          : "border-line bg-ink text-zinc-300 hover:border-zinc-500"}`;
  return (
    <div className={className}>
      {options.map((o) => (
        <button key={o.value} type="button" aria-pressed={value === o.value}
          title={optionTitle?.(o)}
          onClick={() => onChange(o.value)}
          className={button(value === o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  );
}

// Direction-B inset pill wrapper for SegmentedToggle variant="pill".
export const pillWrapperClass = "inline-flex gap-1 rounded border border-[#2e2e34] bg-rail p-[3px]";

// Direction-B hero header: a muted caption, the page's primary metric rendered
// large, an optional plain-English subline, an optional freshness dot (color +
// tooltip), and right-aligned controls. Cyan is reserved for the metric/accent.
export function HeroHeader({ caption, value, accentValue = false, subline, freshness, controls }:
  { caption: string; value: string; accentValue?: boolean; subline?: ReactNode; freshness?: string; controls?: ReactNode }) {
  return (
    <header className="flex flex-col justify-between gap-5 lg:flex-row lg:items-start">
      <div className="min-w-0">
        <div className="flex items-center gap-2 text-xs uppercase tracking-[0.16em] text-zinc-500">
          {caption}
          {freshness ? (
            <span className="inline-flex items-center" title={freshness} aria-label={freshness}>
              <span className="h-1.5 w-1.5 rounded-full bg-moss" />
            </span>
          ) : null}
        </div>
        <div className={`mt-2 text-[44px] font-medium leading-none tracking-[-1px] ${accentValue ? "text-accent" : "text-zinc-50"}`}>
          {value}
        </div>
        {subline ? <p className="mt-3 text-sm leading-6 text-zinc-400">{subline}</p> : null}
      </div>
      {controls ? <div className="flex shrink-0 flex-col items-stretch gap-3 sm:flex-row sm:items-center lg:flex-col lg:items-end">{controls}</div> : null}
    </header>
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
