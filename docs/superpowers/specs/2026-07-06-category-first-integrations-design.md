# Category-First Integrations Design

## Decision

Use the visual companion direction **B: Choose Your Tool**.

The current page is simpler than the previous dashboard-style version, but it still starts with CLI commands and a dense catalog. A normal user should first answer a plain-language question: where do you code? The page should then show only the setup path that matches that choice.

## Page Structure

1. Header
   - Keep the page in the existing Stint console shell.
   - Use approachable copy: `Connect Stint` / `Choose where you code. Stint will show the right setup.`
   - Keep `Manage keys` available, but not visually primary.

2. Tool chooser
   - Three large choices:
     - `Terminal` with `Recommended`
     - `AI agents` with `Codex and Claude`
     - `Editors` with `VS Code, JetBrains, Vim`
   - The selected choice controls the setup panel and visible integration list.
   - Default selection is `Terminal`.

3. Setup panel
   - For Terminal, lead with `Copy setup`; explain that it creates the key, installs Stint, writes config, and checks the connection.
   - For AI agents, lead with Codex and Claude marketplace plugin options plus the same CLI setup as prerequisite/support.
   - For Editors, lead with editor plugins and WakaTime compatibility.
   - Keep commands in compact blocks. Do not show every possible command before the user chooses a path.

4. Supporting catalog
   - Keep all integrations reachable, but visually secondary.
   - Filter/reorder by selected category so the current path is obvious.
   - The detail panel remains available for specific integrations.

## Interaction

- `Copy setup` still creates or reuses a scoped key and copies the complete `STINT_API_URL` + `STINT_API_KEY` installer command.
- `Verify connection` still refetches keys and user agents.
- Selecting a tool category should not navigate away.
- Selecting an individual integration should keep the relevant category active.

## Out of Scope

- API changes.
- New integrations.
- Installer behavior changes.
- Global navigation redesign.
