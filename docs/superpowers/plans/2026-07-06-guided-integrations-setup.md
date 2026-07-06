# Guided Integrations Setup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the Integrations page into a guided setup flow with one primary action and clear connection status.

**Architecture:** Keep the existing page component, recipe data, key generation, and verification APIs. Refactor the visible UI model so categories drive one primary action panel, while recipe command details move into folded advanced sections.

**Tech Stack:** Next.js app router, React client component, TanStack Query, existing source-level tests, Playwright.

---

### Task 1: Test the Guided Setup Contract

**Files:**
- Modify: `web/app/(console)/integrations/page-content.test.ts`
- Modify: `web/app/(console)/integrations/page.test.ts`
- Modify: `web/e2e/integrations.spec.ts`

- [ ] Add source assertions for `Install Stint`, `Install agent plugin`, `Install editor plugin`, `Not connected yet`, `Waiting for first check-in`, and `Stint is connected`.
- [ ] Assert normal UI excludes `Shell CLI` and `WakaTime CLI` cards.
- [ ] Assert implementation terms such as `Stint CLI` are not used as the primary action label.
- [ ] Update Playwright to verify the primary Terminal action is `Install Stint`, and that setup details are folded until opened.

### Task 2: Refactor the Primary Action Panel

**Files:**
- Modify: `web/app/(console)/integrations/page.tsx`

- [ ] Replace the current `Terminal setup` action copy with outcome copy: `Install Stint`.
- [ ] Add a derived `connectionStatus` model from `latestKeyId`, `setupMessage`, `validateMessage`, `keys`, and `userAgents`.
- [ ] Render status text near the primary action, using the approved states.
- [ ] Keep `copyGeneratedSetup` behavior unchanged except for status copy.

### Task 3: Move Mechanics Behind Advanced Details

**Files:**
- Modify: `web/app/(console)/integrations/page.tsx`

- [ ] Rename the details fold from `Setup details` to context-sensitive labels such as `Show command` for Terminal and `Show setup details` for other paths.
- [ ] Keep commands and compatibility content inside the fold.
- [ ] Keep WakaTime compatibility as a note/helper, not as a selectable card.
- [ ] Keep Shell/manual heartbeat out of the normal UI.

### Task 4: Verify and Deploy

**Files:**
- Modify if needed: `web/e2e/integrations.spec.ts`

- [ ] Run `npm run test -- app/\(console\)/integrations/page-content.test.ts app/\(console\)/integrations/page.test.ts`.
- [ ] Run `npm run test:browser -- e2e/integrations.spec.ts`.
- [ ] Run `npm run build`.
- [ ] Rebuild/deploy with `docker compose up -d --build web`.
- [ ] Smoke check `https://stint.fyi/integrations` for `Install Stint` and absence of normal `Shell CLI` / `WakaTime CLI` choices.
