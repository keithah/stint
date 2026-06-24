# Stint Frontend — Cleanup & Polish Spec (for the coding CLI)

> Run this from the repo root. Work in `web/`. This is a sequenced spec: do **Phase 1** (structural cleanup) fully and get tests green before touching Phase 2+. Do **not** redesign anything in Phase 1 — it's pure de-duplication and must be visually identical.

## Stack & current state (already inspected — trust this)

Next.js 16 (App Router) + React 19 + Tailwind 3.4 + Recharts + lucide-react + @tanstack/react-query + zustand. Dark "ops console" theme defined in `web/tailwind.config.ts` (`ink #0d0d0d`, `panel #171717`, `rail #242424`, `line #303030`, `accent #00b4d8`, `ember #f97316`, `moss #84cc16`) and `web/app/globals.css` (grid background, Geist font). Strong but **brittle** test suite (~69 test files, 43 of them read source as strings).

What's good: modern stack, solid AI-cost page, good test coverage. What's wrong: heavy duplication, no shared primitives beyond `StatCard`, no loading/skeleton states, thin a11y, jargon-dense headers, and dead marker CSS classes.

## ⚠️ Hard constraints — read before editing (these will bite you)

The `web` test script (`package.json`) is `tsc --noEmit` plus a long list of `sucrase-node …test.ts` files. Most tests **read source files as text and assert substrings**. Therefore:

1. **Do NOT move or rename page files.** 43 tests hardcode paths like `readFileSync("app/dashboard/page.tsx")`. A route-group layout (`app/(console)/…`) would break them. Keep every page where it is.
2. **Preserve these exact strings** where tests pin them:
   - `app/dashboard/page.tsx` must still contain: `<OpsStatusHeader`, `ops-dashboard-header`, `freshnessLabel(data)`, `href="/settings"`.
   - `components/dashboard-charts.tsx` must still contain: `const chartPanelClass`, `ops-chart-panel`. (Don't touch this file in Phase 1.)
   - `components/shell.tsx` must still contain: `usePathname`, `const navItems`, `desktopNavClass`, `mobileNavClass`, `Operations console`.
3. **Run `npm test` in `web/` after every page edit.** If a string-match test breaks because of a legitimate refactor, update that test to assert the new reality — but keep its intent, never delete coverage to make it pass.
4. These are **untested and safe to extract freely**: `rangeOptions`, `costModeOptions`, `HeaderReadout`, `LiveHeader`, the `<Providers><Shell>` wrapper, and the "Login required" auth block.

> If you run **Phase 0** below first, it converts the brittle string-match tests into behavioral tests and removes the specific string pins in items 2–3. After Phase 0, those pins (`ops-dashboard-header`, `ops-chart-panel`, `freshnessLabel(data)`, the `grid gap-4 md:grid-cols-5` className, exact-JSX assertions, `Operations console`) are no longer required, and the dead marker classes can actually be deleted. If you skip Phase 0, obey items 2–3 as written.

---

## Phase 0 — De-brittle the tests (optional but recommended; do it first)

The frontend suite has ~18 tests that `readFileSync` a component and assert literal substrings — classNames, marker classes, and exact JSX. They pin styling and markup, so they actively block the cleanup and restyle in Phases 1–3 (and they pass even when the UI is broken, since they never render anything). Convert them to behavioral tests before refactoring.

Find them all:

```
cd web && grep -rl "readFileSync" --include=*.test.ts app components
```

Two reference styles already exist in the repo — copy the good one:
- **Bad (brittle):** `app/dashboard/ops-polish.test.ts`, `app/dashboard/current-day-card.test.ts`, `app/dashboard/project-stacked-chart.test.ts`, `components/ai-human-title.test.ts`, `components/shell-ops-polish.test.ts` — assert source strings.
- **Good (behavioral):** `lib/ai-ring.test.ts`, `lib/activity-heatmap.test.ts` — `import` a pure function and assert its output.

### Conversion strategy

1. **Extract embedded logic into pure `lib/` modules, then test the module.** Several brittle tests are really asserting logic that's trapped inside a page component:
   - `freshnessLabel`, `todayDetail`, `formatSeconds` (in `app/dashboard/page.tsx`) → move to `lib/dashboard-format.ts` and write `lib/dashboard-format.test.ts` that imports and asserts return values (e.g. `freshnessLabel({is_up_to_date:true})` → `"cache fresh"`; `todayDetail("api","Go")` → `"api · Go"`).
   - `isActive(pathname, href)` (in `components/shell.tsx`) → move to `lib/nav.ts` with `navItems`, test the active-matching behavior directly. Drop the assertions that merely check the strings `desktopNavClass`/`mobileNavClass`/`usePathname` exist.
   - Any range/cost logic → covered by `lib/ranges.ts` + `lib/ranges.test.ts` from Phase 1.

2. **Delete pure-styling assertions outright.** Assertions that only check a className or marker class is present (`ops-dashboard-header`, `ops-chart-panel`, `grid gap-4 md:grid-cols-5`, `chartPanelClass`) verify nothing behavioral and exist only to freeze the layout. Remove them. This is what frees you to delete the dead marker classes and restyle in Phase 1/3.

3. **Replace exact-JSX assertions with render-independent checks.** Tests like `assertIncludes(..., "<ProjectStackedArea days={data?.days ?? []} />")` should become either (a) a behavioral test of the chart's data-shaping helper, or (b) deleted if they only assert "this component is referenced." Don't assert literal JSX.

4. **Keep the registration convention.** Each test file is listed in the `web/package.json` `test` script and several assert their own path is present in `package.json`. When you add/rename/remove a test, update that script string accordingly (and keep or drop the self-registration assertion consistently).

### Example conversion

Before — `app/dashboard/ops-polish.test.ts` (brittle):
```ts
const dashboardSource = readFileSync("app/dashboard/page.tsx", "utf8");
assertIncludes("...", dashboardSource, "ops-dashboard-header");
assertIncludes("...", dashboardSource, "freshnessLabel(data)");
```

After — `lib/dashboard-format.test.ts` (behavioral):
```ts
import { freshnessLabel, todayDetail } from "./dashboard-format";
assertEqual("fresh cache", freshnessLabel({ is_up_to_date: true } as any), "cache fresh");
assertEqual("refreshing cache", freshnessLabel({ is_up_to_date: false } as any), "cache refreshing");
assertEqual("loading cache", freshnessLabel(undefined), "loading cache");
assertEqual("project + language", todayDetail("api", "Go"), "api · Go");
```
…and the `ops-dashboard-header`/`ops-chart-panel`/className assertions are simply removed.

### Gate
`cd web && npm test` green, `npm run lint` clean, `npx tsc --noEmit` clean. Report which tests you converted, which you deleted (and why each deleted assertion was non-behavioral), and confirm coverage of real logic did not drop.

---

## Phase 1 — Structural cleanup (DRY). Visually identical. Test-safe.

### 1.1 Create `web/lib/ranges.ts`

`rangeOptions` is currently duplicated in `app/dashboard/page.tsx`, `app/ai-costs/page.tsx`, and `app/projects/[name]/page.tsx`; `costModeOptions` lives in `app/ai-costs/page.tsx`. Centralize:

```ts
import type { StatsRange } from "@/lib/api";
import type { UsageCostMode } from "@/lib/usage-api";

export const rangeOptions: ReadonlyArray<{ value: StatsRange; label: string }> = [
  { value: "last_7_days", label: "7 days" },
  { value: "last_30_days", label: "30 days" },
  { value: "last_6_months", label: "6 months" },
  { value: "last_year", label: "Year" },
  { value: "all_time", label: "All time" }
];

export const costModeOptions: ReadonlyArray<{ value: UsageCostMode; label: string }> = [
  { value: "auto", label: "Auto" },
  { value: "calculate", label: "Calculate" },
  { value: "display", label: "Display" }
];

export function rangeLabel(range: StatsRange): string {
  return rangeOptions.find((o) => o.value === range)?.label ?? rangeOptions[0].label;
}
```

Replace the local `const rangeOptions`/`const costModeOptions` arrays in those pages with imports. Add a unit test `lib/ranges.test.ts` and append it to the `test` script.

### 1.2 Create `web/components/ui.tsx`

Shared presentational primitives. Keep markup byte-identical to the originals so the diff is purely structural:

```tsx
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
export function SegmentedToggle<T extends string>({ options, value, onChange, className = "", size = "sm" }:
  { options: ReadonlyArray<{ value: T; label: string }>; value: T; onChange: (v: T) => void; className?: string; size?: "sm" | "xs"; }) {
  const pad = size === "xs" ? "px-3 py-2 text-xs" : "px-3 py-2 text-sm";
  return (
    <div className={className}>
      {options.map((o) => (
        <button key={o.value} type="button" aria-pressed={value === o.value}
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
```

### 1.3 Create `web/components/app-shell.tsx`

```tsx
"use client";
import type { ReactNode } from "react";
import { Providers } from "@/components/providers";
import { Shell } from "@/components/shell";

export function AppShell({ children }: { children: ReactNode }) {
  return <Providers><Shell>{children}</Shell></Providers>;
}
```

> Why `AppShell` and not a real App Router layout: the correct idiom is `app/(console)/layout.tsx`, but that requires moving the 8 console pages into a route group, which breaks the 43 path-pinned tests. `AppShell` gets the same de-duplication with zero file moves. Note this in `web/components/app-shell.tsx` as a comment and as a follow-up once tests are de-brittled.

### 1.4 Swap the wrapper on all 8 console pages

Pages: `dashboard`, `projects`, `projects/[name]`, `integrations`, `insights`, `ai-costs`, `goals`, `leaderboards`, `reports`, `settings`. In each, replace:

```tsx
import { Providers } from "@/components/providers";
import { Shell } from "@/components/shell";
// …
<Providers><Shell><XContent /></Shell></Providers>
```

with:

```tsx
import { AppShell } from "@/components/app-shell";
// …
<AppShell><XContent /></AppShell>
```

Public pages (`login`, `share/[…]`, `users/[…]`) keep their own `<Providers>` and must **not** gain the nav shell. The landing `app/page.tsx` stays untouched.

### 1.5 Collapse the duplicated pieces inside dashboard & ai-costs

- `app/dashboard/page.tsx`: delete the local `HeaderReadout`, `rangeOptions`, and the "Login required" block; import `HeaderReadout`, `SegmentedToggle`, `AuthGate` from `@/components/ui` and `rangeOptions` from `@/lib/ranges`. Replace the range-button `.map(...)` with `<SegmentedToggle options={rangeOptions} value={range} onChange={setRange} className="grid grid-cols-2 gap-2" />`. **Keep** the `OpsStatusHeader` function name, the `ops-dashboard-header` class, and the `freshnessLabel(data)` call (test-pinned).
- `app/ai-costs/page.tsx`: same treatment. Use the shared `costModeOptions` with `<SegmentedToggle size="xs" className="grid grid-cols-3 gap-2" />`. `LiveHeader` is untested — free to simplify.

### 1.6 Delete dead marker classes — *except the pinned ones*

`ops-dashboard-header` and `ops-chart-panel` have no CSS behind them but are asserted by `app/dashboard/ops-polish.test.ts`. **Leave them** (or, better, give them a real definition in `globals.css` and keep the names). Do not introduce new no-op marker classes.

**Gate:** `cd web && npm test` green, `npm run lint` clean, `npx tsc --noEmit` clean before moving on.

---

## Phase 2 — Tokens, states, accessibility (visual polish, low risk)

1. **Design tokens.** Promote the palette to CSS variables in `globals.css` (`--surface`, `--surface-2`, `--border`, `--text`, `--text-muted`, `--accent`) and reference them from `tailwind.config.ts` so the theme is changeable in one place. Keep the existing Tailwind color names working (alias them) so no class churn.
2. **Loading skeletons.** There are currently **zero** (`grep animate-pulse` → none). Add a `Skeleton` primitive (`animate-pulse rounded bg-white/5`) and use it for the dashboard stat row, ai-costs summary, and chart panels while `useQuery` is `isLoading`, instead of popping in or bare "Loading…" text.
3. **Consistent empty states.** Extract the repeated dashed-border empty panel into `<EmptyState icon title hint action?>` and use it everywhere (dashboard project grid, ai-costs "No AI usage yet", projects list, leaderboards).
4. **Accessibility pass.** Only ~11 files use `aria-`. Add: `aria-label` on icon-only buttons (refresh, dismiss, account), `focus-visible` ring utilities globally, `aria-pressed` on toggles (already in `SegmentedToggle`), and a non-color cue in the cost/activity heatmaps (title attr exists — add a legend).

---

## Phase 3 — Visual direction: **Direction B, Calmer modern** (chosen)

Apply Direction B to `/dashboard` and `/ai-costs` first as the template, then roll the same patterns out to the other pages. Direction A (dense "ops" look) was rejected — do not keep the status-chip row or the 32px grid background.

### Direction B spec — "calmer modern, metric-first"

Build these as reusable tokens/components (not one-off styles), since every page will adopt them:

- **Surfaces:** page background and panels move off pure `#0d0d0d`/`#171717` to a softer base — panel `#161618`, hairline borders `#26262b` (replace the heavy `#303030` lines). **Remove the grid background** from `globals.css` (the `linear-gradient` 48px grid on `body`).
- **Header = hero metric.** Each page leads with its primary number large (`text-[44px] font-medium tracking-[-1px] leading-none`), a muted caption above it (e.g. "Coding activity · last 7 days"), and a one-line plain-English subline below (e.g. "2h 7m today · averaging 8h 53m a day"). Kill the jargon chips ("Live pipeline", "% calculated", "cache fresh", "cache mode", "Coding activity ops"). If freshness must be shown, use a single small dot + tooltip.
- **Stat tiles:** borderless, separated from the header by one top divider (`border-t border-[#26262b] pt-5`), generous `gap` (≈`18px`), labels `13px text-zinc-500`, values `22px font-medium`. The AI/cost metric keeps the cyan `#00b4d8` accent — and **only** that one.
- **Range / cost-mode control:** a single inset segmented pill (`bg #1f1f23`, `border #2e2e34`, `rounded`, `p-[3px]`, active segment `bg-accent text-ink`), not the current 2×N button grid. Implement via the `SegmentedToggle` primitive from Phase 1 by passing a pill wrapper className.
- **Spacing rhythm:** increase section gaps (the current uniform `mt-5` → `mt-6`/`mt-8` between major sections; `26px`/`24px` panel padding) so the app breathes.
- **Accent discipline:** cyan is reserved for AI/cost emphasis and the active control state only — not borders, not headings.

Reference values come from the approved mockup (the "Direction B" frame). Match its surface colors, type scale, and spacing. Keep it dark — this is a calmer dark theme, not a light theme.

---

## Verification (required)

- `cd web && npm test` (tsc + full sucrase suite) green after **each phase**.
- `npm run lint` clean; `npx next build` succeeds.
- Phase 1 must produce **no visual change** — confirm by diffing rendered output or screenshotting `/dashboard` and `/ai-costs` before/after.
- For any test you had to update, show the before/after of the assertion and confirm it still tests the same behavior.
- Report a short summary: files added, pages touched, tests updated, and any string-pinned constraint you had to work around.
```
