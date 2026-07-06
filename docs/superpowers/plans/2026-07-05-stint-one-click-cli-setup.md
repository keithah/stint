# Stint One-Click CLI Setup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the native Stint setup path create `stint_` keys, write `~/.stint.cfg`, install/configure from one copied command, and show a clear connected state.

**Architecture:** Keep WakaTime compatibility as a fallback path while making Stint-native defaults first-class. The installer accepts `STINT_API_URL` and `STINT_API_KEY`, runs `stint setup`, prints version, and runs `stint doctor`. The integrations page owns the one-click flow by creating a scoped key and rendering a single install command.

**Tech Stack:** Go API/auth/CLI, Next.js App Router, TypeScript source tests, shell installer route.

---

### Task 1: Native API Key Prefix

**Files:**
- Modify: `internal/auth/apikey.go`
- Modify: `internal/auth/apikey_test.go`
- Review: `internal/db/store.go`

- [ ] **Step 1: Write the failing test**

Change `TestGenerateAPIKeyProducesWakaTimeFanoutCompatibleKey` to expect `stint_`, and add a compatibility assertion that `IsAPIKey("waka_legacy")` still returns true.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth`
Expected: FAIL because generated keys still use `waka_`.

- [ ] **Step 3: Write minimal implementation**

Set generated key prefix to `stint_`, keep `waka_` accepted in `IsAPIKey`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/auth`
Expected: PASS.

### Task 2: Stint Config Default

**Files:**
- Modify: `internal/stintcli/commands_config.go`
- Modify: `internal/stintcli/stint_config.go`
- Modify: `internal/stintcli/setup_test.go`
- Modify: CLI tests that pin `DefaultWakaTimeConfigPath` for native `stint config init`

- [ ] **Step 1: Write the failing test**

Add or update tests so `stint config init` writes `DefaultStintConfigPath()` by default, and `stint setup` writes only `~/.stint.cfg` unless `--wakatime-config` is explicitly provided.

- [ ] **Step 2: Run focused tests**

Run: `go test ./internal/stintcli -run 'TestSetup|TestConfig'`
Expected: FAIL while config init/setup still writes WakaTime by default.

- [ ] **Step 3: Write minimal implementation**

Use `DefaultStintConfigPath()` for `stint config init|read|write` defaults. Make `runSetup` write native config by default, with compatibility config opt-in.

- [ ] **Step 4: Run focused tests**

Run: `go test ./internal/stintcli -run 'TestSetup|TestConfig'`
Expected: PASS.

### Task 3: Installer Configures and Verifies

**Files:**
- Modify: `web/app/install.sh/route.ts`
- Modify: `web/app/install.sh/route.test.ts`
- Modify: `internal/stintcli/commands_http.go`
- Modify: `internal/stintcli/commands_read_test.go`

- [ ] **Step 1: Write failing tests**

Assert the installer calls `stint setup` when `STINT_API_URL` and `STINT_API_KEY` are present, then prints version and runs `stint doctor`. Add a doctor text-output test that includes CLI version, config path, API URL, authenticated user, and “Stint CLI is connected”.

- [ ] **Step 2: Run focused tests**

Run: `npm run test -- app/install.sh/route.test.ts` and `go test ./internal/stintcli -run TestRunDoctor`
Expected: FAIL while installer and doctor output are old.

- [ ] **Step 3: Write minimal implementation**

Append setup/doctor logic to the install script. Update doctor text output to include connection summary while preserving JSON output.

- [ ] **Step 4: Run focused tests**

Expected: PASS.

### Task 4: One-Click Integrations UI

**Files:**
- Modify: `web/app/(console)/integrations/page.tsx`
- Modify: `web/app/(console)/integrations/recipes.ts`
- Modify: `web/app/(console)/integrations/page-content.test.ts`
- Modify: `web/components/settings/api-keys-card.tsx`
- Modify: `web/app/(console)/dashboard/page.tsx`

- [ ] **Step 1: Write failing source tests**

Assert the Stint CLI recipe renders a one-command setup using `STINT_API_URL` and `STINT_API_KEY`, uses `stint_` placeholders, includes `~/.stint.cfg`, and shows “Stint CLI is connected”.

- [ ] **Step 2: Run web tests**

Run: `npm run test -- app/(console)/integrations/page-content.test.ts`
Expected: FAIL while the page still uses separated install/config/verify copy.

- [ ] **Step 3: Write minimal implementation**

Generate the scoped key from the integrations page, set it as `latestKey`, render the one-command setup as the primary copy block, and show connected status from `userAgents` when a Stint CLI client is present.

- [ ] **Step 4: Run web tests**

Run: `npm run test`
Expected: PASS.

### Task 5: Full Verification

**Files:**
- All changed files.

- [ ] **Step 1: Run full Go tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 2: Run full web tests**

Run: `npm run test` in `web`
Expected: PASS.

- [ ] **Step 3: Check worktree**

Run: `git status --short`
Expected: only intended changes plus the pre-existing `web/next-env.d.ts` dirty file.
