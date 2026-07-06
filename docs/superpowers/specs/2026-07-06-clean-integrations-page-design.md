# Clean Integrations Page Design

## Decision

Use direction **B: Setup Strip + Catalog** from the visual companion.

The current Integrations page is too busy because it mixes onboarding, health metrics, detailed recipes, telemetry education, screenshots, compatibility notes, and command references in one long surface. The redesign should feel closer to WakaTime's integrations page: a clear title, a short supporting line, and a browsable list of integrations.

WakaTime inspiration: `https://wakatime.com/integrations` presents a lightweight integrations catalog with simple entries and a separate editor plugins section. Stint should borrow the simplicity, not the exact visual styling.

## Page Structure

1. Header
   - Title: `Integrations`
   - Short subtitle focused on the job: connect Stint to editors and AI agents.
   - No large dashboard metrics in the header.

2. Recommended setup strip
   - A compact full-width strip at the top.
   - Title: `Set up Stint CLI`
   - Supporting copy: creates a scoped key, copies a complete command, and validates connection.
   - Primary button: `Copy setup`
   - Secondary button: `Verify connection`
   - Status text lives in the strip, not in a separate health dashboard.
   - The copied command must include the generated `stint_...` key. No separate key-copy step.

3. Integration catalog
   - WakaTime-style grid of simple integration cards.
   - Cards should be scannable: name, type/status, one short description.
   - Keep first-party and compatibility categories clear:
     - Stint: Stint CLI, Codex, Claude Code
     - Editors: VS Code, JetBrains, Vim/Neovim
     - Compatibility: WakaTime CLI, Shell CLI
   - Cards open/select a small detail panel below or beside the grid.

4. Detail panel
   - Shows only the selected integration's essential setup.
   - No screenshot block.
   - No multi-step command dump for the primary Stint CLI path.
   - Advanced commands should be collapsed into docs links or a small note.

5. Remove from main page
   - Extended AI telemetry panel.
   - Three status tiles.
   - Large connection health metrics.
   - Screenshot previews.
   - Long verification command blocks.

## Interaction Model

- `Copy setup` creates or reuses the scoped integration key, inserts it into the command, and copies the full one-line installer command.
- `Verify connection` refetches API keys and user agents. It succeeds when either:
  - the generated key has `last_used_at`, which happens after `stint doctor`; or
  - a Stint CLI user agent is visible.
- Selecting catalog cards changes the detail panel without navigating away.
- The page should remain useful when unauthenticated/static-rendered: placeholder command uses `stint_your_stint_key`, and authenticated users get the real key after clicking `Copy setup`.

## Visual Direction

- Quiet operational UI, not marketing.
- Dense but uncluttered.
- Use existing Stint dark shell, accent color, border radius, and lucide icons.
- Avoid nested cards and decorative sections.
- Prefer plain catalog cards with restrained borders.
- Keep button labels short and action-oriented.

## Testing

- Update source-level integration tests to assert:
  - `Copy setup` flow exists.
  - `Verify connection` exists.
  - Generated key is inserted into the copied setup command.
  - Old multi-command Stint CLI verification block is absent.
  - Extended AI telemetry panel and status tiles are absent.
  - Catalog entries remain present for Stint CLI, Codex, Claude Code, VS Code, JetBrains, Vim/Neovim, WakaTime CLI, and Shell CLI.

- Run:
  - `npm run test` in `web`
  - build/deploy web after implementation

## Out of Scope

- Changing API endpoints.
- Adding new integrations.
- Changing CLI install behavior.
- Reworking global app shell navigation.
