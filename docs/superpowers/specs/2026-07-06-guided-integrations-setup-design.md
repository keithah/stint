# Guided Integrations Setup Design

## Decision

Move the Integrations page from a catalog/reference surface to a guided setup flow.

The current page is closer, but it still exposes integration mechanics too early. Normal users should not need to understand the difference between Stint CLI, WakaTime compatibility, shell heartbeats, generated keys, or manual commands before they know what to do. The page should guide users through one recommended path for their situation, then provide advanced details only when requested.

## UX Goal

The page should answer three questions in order:

1. Where do you code?
2. What should you click?
3. Did Stint connect?

Everything else is secondary.

## Page Structure

1. Header
   - Title: `Connect Stint`
   - Subtitle: short and outcome-oriented, e.g. `Install Stint once, then verify that activity is flowing.`
   - Keep `Manage keys` available, but visually secondary.

2. Setup Type Choice
   - Three choices remain:
     - `Terminal`
     - `AI agents`
     - `Editors`
   - These are not “integrations” to browse. They are paths into setup.
   - Default path is `Terminal`.

3. Primary Action Panel
   - Each path gets exactly one primary action:
     - Terminal: `Install Stint`
     - AI agents: `Install agent plugin`
     - Editors: `Install editor plugin`
   - Button labels describe the outcome, not implementation details.
   - The Terminal primary action uses the existing key-generation and command-copy behavior.
   - AI agent and editor paths can open the relevant selected setup details, but should not compete with the Terminal install action.

4. Connection Status
   - Show a persistent status block near the primary action:
     - `Not connected yet`
     - `Waiting for first check-in`
     - `Stint is connected`
     - `No check-in yet`
   - `Verify connection` remains available.
   - Success should feel final and clear.

5. Advanced Details
   - Fold details under labels like:
     - `Show command`
     - `Manual setup`
     - `Use WakaTime-compatible setup`
     - `Manage API keys`
   - WakaTime compatibility is not a normal setup choice.
   - Shell/manual heartbeat is not a normal setup choice.
   - Keep commands out of the first impression.

## Copy Principles

- Prefer human verbs:
  - `Install Stint`, not `Stint CLI`
  - `Install agent plugin`, not `marketplace plugin recipe`
  - `Verify connection`, not `validate heartbeat ingestion`
- Avoid exposing implementation details until advanced details are opened.
- Explain compatibility as reassurance, not a decision the user must make.

## Interaction Rules

- `Install Stint` creates/reuses a scoped key and copies the complete install command with `STINT_API_URL` and `STINT_API_KEY`.
- After copying setup, status changes to `Waiting for first check-in`.
- `Verify connection` refetches keys and user agents.
- Connected state succeeds when the generated key has `last_used_at` or a Stint user agent is visible.
- Selecting `AI agents` or `Editors` should not show Terminal-only command blocks in the main panel.

## Out of Scope

- New integrations.
- API changes.
- Installer behavior changes.
- Global navigation changes.
