# Category-First Integrations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the integrations page understandable for normal users by asking where they code before showing setup details.

**Architecture:** Keep the existing React page and recipe data. Add a local category model that maps user-friendly choices to recipe IDs, renders a chooser, and filters the visible catalog/detail panel by the active choice.

**Tech Stack:** Next.js app router, React client component, TanStack Query, existing source-level tests, Playwright.

---

### Task 1: Lock the UX Contract in Tests

**Files:**
- Modify: `web/app/(console)/integrations/page-content.test.ts`
- Modify: `web/app/(console)/integrations/page.test.ts`
- Modify: `web/e2e/integrations.spec.ts`

- [ ] Add assertions for `Choose where you code`, `Terminal`, `AI agents`, `Editors`, and `activeToolCategory`.
- [ ] Assert the old first impression remains absent: no `Integration catalog` as the primary heading before the chooser.
- [ ] Update browser assertions to click category buttons before checking category-specific content.

### Task 2: Implement Category-First Layout

**Files:**
- Modify: `web/app/(console)/integrations/page.tsx`

- [ ] Add `toolCategories` mapping `terminal`, `agents`, and `editors` to labels, descriptions, primary recipe IDs, and catalog recipe IDs.
- [ ] Replace the setup strip plus broad catalog first view with:
  - header copy
  - category chooser buttons
  - active setup panel
  - secondary filtered catalog
- [ ] Preserve `copyGeneratedSetup`, `validateConnection`, hash selection, and detail-panel copy behavior.

### Task 3: Verify and Ship

**Files:**
- Modify if needed: `web/e2e/integrations.spec.ts`

- [ ] Run `npm run test -- app/\(console\)/integrations/page-content.test.ts app/\(console\)/integrations/page.test.ts`.
- [ ] Run `npm run test:browser -- e2e/integrations.spec.ts`.
- [ ] Run `npm run build`.
- [ ] Rebuild/deploy `docker compose up -d --build web`.
- [ ] Smoke check `https://stint.fyi/integrations`.
