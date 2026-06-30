# First Class Stint Integrations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Stint-owned setup the primary integration path, add Claude Code, and document both CLI and Desktop flows for Codex and Claude.

**Architecture:** Keep the existing integrations page structure and update only the recipe data, screenshots, and tests. Stint-owned plugin instructions are first, CLI setup is second, and WakaTime-compatible setup is a compatibility option.

**Tech Stack:** Next.js app router, TypeScript source-string tests, Playwright smoke tests, local SVG screenshots.

---

### Task 1: Test First-Class Stint Options

**Files:**
- Modify: `web/app/(console)/integrations/page-content.test.ts`
- Modify: `web/e2e/integrations.spec.ts`

- [ ] Add source assertions for Claude Code, Stint-owned plugin copy, WakaTime-compatible copy, and CLI/Desktop setup strings.
- [ ] Run `npm --prefix web test` and confirm the new assertions fail before implementation.

### Task 2: Update Integration Recipes

**Files:**
- Modify: `web/app/(console)/integrations/recipes.ts`
- Add: `web/public/integrations/screenshots/claude-code.svg`

- [ ] Add a Claude Code client and recipe.
- [ ] Update Codex to mention Codex CLI and Codex Desktop.
- [ ] Make VS Code and JetBrains options lead with Stint-owned plugin setup.
- [ ] Keep WakaTime-compatible plugin links as explicit compatibility options.

### Task 3: Verify and Deploy

**Files:**
- No code changes unless verification finds defects.

- [ ] Run `npm --prefix web test`.
- [ ] Run `npm --prefix web run build`.
- [ ] Run `npm --prefix web run test:browser -- --grep "integration names"`.
- [ ] Run `git diff --check`.
- [ ] Rebuild/deploy the web container and verify `https://stint.fyi/integrations` with Playwright.
