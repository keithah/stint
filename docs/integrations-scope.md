# Editor & IDE Integrations — Scope & Architecture

How Stint gets first-class support for **everything WakaTime integrates with**,
while maintaining the **fewest moving parts**.

Locked decisions (full log in [§8](#8-decision-log)):

- **Posture:** reuse the existing plugin ecosystem + own one cohesive setup
  layer. No plugin forks.
- **Config:** `~/.stint.cfg` is Stint's native config; `~/.wakatime.cfg` is read
  with the same parser as a compatibility fallback, and is written to so
  upstream plugins reach Stint.
- **AI data:** hybrid — `stint-collect` (file scan) for token/cost, upstream AI
  hook plugins (heartbeats) for in-editor AI lines/prompts/sessions.
- **Surface:** first-class for the full WakaTime integration list (minus a few
  scope cuts in §8).
- **Shape:** extend the existing **stint CLI** (`cmd/collect` → a unified
  `stint` command, cobra); add a cross-platform **stint-desktop** (Tauri) that
  embeds it.

---

## 1. The core insight (why this is cheap to maintain)

WakaTime maintains ~60 integration repos, but they collapse into **five
ingestion patterns**, and almost all funnel through one chokepoint: the
`wakatime-cli` binary reading `~/.wakatime.cfg`. That binary already supports an
`api_url` override, and Stint already ingests WakaTime-shaped heartbeats.

> **Therefore: every wakatime-cli-based plugin already works against Stint the
> moment `api_url` points at your server.** Stint does not write, fork, or
> vendor a single editor plugin. "First-class support for everything" = verify
> the wire contract once + make the *config* one step + document.

What Stint owns is small and high-leverage:

1. **`stint` CLI** (extend `cmd/collect`) — detect editors, write/repair config,
   manage the `wakatime-cli` binary, install AI hook plugins, run the collector,
   and (later) print stats.
2. **stint-desktop** — one cross-platform app that wraps the CLI: GUI onboarding
   + system app-usage tracking + scheduled collection. Replaces the *two*
   separate apps WakaTime maintains (Electron `desktop-wakatime` + Swift
   `macos-wakatime`).
3. **Server wire-compatibility** — already ~done (see
   [`wakatime-ai-compat.md`](./wakatime-ai-compat.md)).

We reuse upstream `wakatime-cli` as-is. **No fork** — forking means owning
language detection, dependency parsing, the offline queue, project/git
detection, and cross-platform release engineering forever.

---

## 2. Configuration model

Two files, one parser, clear precedence. The constraint that drives everything:
**upstream `wakatime-cli` (and therefore every editor/AI plugin) reads only
`~/.wakatime.cfg`** — we can't make it read anything else.

| File | Read by | Role |
|---|---|---|
| `~/.stint.cfg` | `stint` CLI, stint-desktop | Stint-native config (primary) |
| `~/.wakatime.cfg` | `wakatime-cli` + all plugins **and** Stint | compatibility fallback for Stint; the bridge that makes plugins reach Stint |

**Read precedence (Stint's own tools):**
flags → `STINT_*` env → `~/.stint.cfg` → `~/.wakatime.cfg` → built-in defaults.

So a user who already had WakaTime set up keeps working with no reconfiguration,
and a Stint-native user's `~/.stint.cfg` always wins.

**Write behavior — `stint setup` writes both:**

- `~/.stint.cfg` — native keys (api_url, api_key, collector options, …).
- `~/.wakatime.cfg` `[settings]` — `api_url` + `api_key` only, **preserving any
  existing keys**, so upstream plugins/`wakatime-cli` post to Stint.

Both use the same INI `[settings]` format, so one parser serves both; `.stint.cfg`
may add Stint-only sections (e.g. `[collect]`).

```ini
# ~/.wakatime.cfg  (what plugins read)
[settings]
api_url = https://your-stint/api/v1
api_key = waka_xxxxxxxx
```

**Bootstrap UX:** the Settings page shows a copyable
`stint setup --server <url> --key <key>`; the CLI also honors `STINT_API_URL` /
`STINT_API_KEY` for scripted installs.

---

## 3. The five ingestion patterns

| Pattern | How it sends data | Stint compat requirement | Stint maintenance |
|---|---|---|---|
| **A. Editor/IDE plugins** | shell out to `wakatime-cli`, read `~/.wakatime.cfg` | accept heartbeats (✅); CLI sets `api_url` | none (config only) |
| **B. AI agent hook plugins** | install `wakatime-cli`, send **AI heartbeats** on prompt/file-edit hooks | accept AI heartbeat fields (✅, see compat doc); CLI automates install | none (config only) |
| **C. Direct-POST extensions** | POST heartbeats straight to the API (sandboxed, no subprocess) | accept heartbeats (✅); **needs a custom `api_url` field in the extension's own settings** | doc + verify |
| **D. Desktop system trackers** | watch active-app usage, shell to `wakatime-cli` | accept heartbeats (✅) | **unify into stint-desktop** |
| **E. Core CLI** | `wakatime-cli` itself + pip wrapper | reuse upstream binary | vendor/download only |

---

## 4. Full parity checklist

"Status" is what Stint must do for **first-class** support. Unless noted, the
work is **config + one verification heartbeat**, not code.

### Pattern A — Editor / IDE plugins (work via `api_url`)

VS Code (`vscode-wakatime`, also covers Cursor / Windsurf / VSCodium) · JetBrains
all IDEs + Android Studio (`jetbrains-wakatime`) · Vim/Neovim (`vim-wakatime`) ·
Zed (`zed-wakatime`) · Sublime Text · Visual Studio · Emacs (`wakatime-mode`) ·
Atom · Brackets · Cloud9 · Eclipse · NetBeans · Geany · Gedit · Kate · Komodo ·
Micro · Notepad++ · TextMate · Kakoune · Xcode · Nova · JupyterLab · SSMS ·
Delphi · Eric6 · Wing · SlickEdit. **Tier-3 creative/niche** (same shim, verified
on request): Godot · Blender · Unity · Roblox Studio · Figma · Sketch · Adobe XD
· Coda · Processing · TeXstudio · Camunda Modeler · Recaf · ReclassEx · IDA Pro ·
Office · Obsidian · Zotero · REPL prompts (python/lua/tcl/perl).

**Status:** ✅ already compatible. First-class = `stint connect` knows how to
configure each (mostly `~/.wakatime.cfg`; VS Code/JetBrains optionally get
editor-level settings via `--deep`), + a verified heartbeat in CI's smoke test,
+ a docs row. **Tier-1 first:** VS Code family, JetBrains, Vim/Neovim, Zed.

### Pattern B — AI agent hook plugins (the new AI stats)

`claude-code-wakatime` · `codex-cli-wakatime` · `codex-wakatime` ·
`antigravity-wakatime` · `amp-cli-wakatime` · `copilot-cli-wakatime`.

**Status:** ✅ server accepts the AI heartbeat fields (per compat doc). These
install via each agent's own plugin marketplace and read `~/.wakatime.cfg`.
First-class = `stint plugin install claude-code|codex|antigravity|amp|copilot`
automates the marketplace-add + writes the api key. This is the hybrid's "AI
heartbeat" half; the collector covers token/cost in parallel.

### Pattern C — Direct-POST extensions (need a custom API URL)

`browser-wakatime` (Chrome/Firefox/Edge) · `discord-wakatime` / `vencord-wakatime`.

**Status:** ⚠️ low priority. These can't shell to a CLI, so they POST to a base
URL set in their own options. Stint must (a) confirm that option accepts a
self-hosted base; (b) accept that a **sandboxed extension cannot be auto-
configured** by the CLI — so this stays a documented manual step. Discord is a
**scope cut** (S1); browser is manual-only (S2).

### Pattern D — Desktop trackers → **stint-desktop**

`desktop-wakatime` (Windows + Linux, Electron) · `macos-wakatime` (macOS, Swift).

**Status:** 🔨 build one cross-platform `stint-desktop` (Tauri, see §5) that does
app-usage tracking on all three OSes, replacing both.

### Pattern E — Core CLI

`wakatime-cli` (Go, the chokepoint) · `wakatime-cli-pip`.

**Status:** reuse upstream binary. `stint cli install` downloads/version-checks
it into `~/.wakatime/` exactly like the plugins do (shared binary). No fork.

### Out of scope / later

`discord-wakatime`, `vencord-wakatime`, `wakatime-mobile` (S1/S4) · `WakaTimeCLI`
terminal report → covered by future `stint today`/`stint stats` · `wakadump` →
Stint already has dumps/imports.

---

## 5. The `stint` CLI (extend `cmd/collect`, cobra)

Today `cmd/collect` is scan-and-post only. Grow it into one cobra command with
subcommands; the current behavior becomes `stint collect`. Keep a `stint-collect`
alias/symlink for back-compat.

| Subcommand | Purpose | Notes |
|---|---|---|
| `stint collect` | current collector (file scan → `usage_events`) | `--watch` etc. unchanged |
| `stint setup` | capture api key + server, write `~/.stint.cfg` **and** `~/.wakatime.cfg` | cohesive onboarding entry point |
| `stint connect` | **detect installed editors** and configure each; `--deep` for VS Code/JetBrains | data-driven editor registry |
| `stint plugin install <agent>` | install an AI hook plugin and wire the api key | automates `claude plugin marketplace add …`, etc. |
| `stint cli install` | download/verify/update upstream `wakatime-cli` into `~/.wakatime/` | pinned version, checksum-verified, `STINT_WAKATIME_CLI` override |
| `stint doctor` | health check: config present, api reachable, cli installed, last heartbeat, agents/editors detected | mirrors `claude doctor` |
| `stint today` / `stint stats` | print stats in the terminal (later) | replaces `WakaTimeCLI`; reads Stint API |

`stint connect` is the leverage point — a **data-driven registry** (same pattern
as the collector's agent registry): each entry is `{editor id, how to detect it,
how to configure it}`. For ~90% of editors "configure" is just ensuring
`~/.wakatime.cfg` has `api_url`/`api_key`. Adding an editor = one registry row,
never a new plugin.

---

## 6. stint-desktop (Tauri, cross-platform)

One app for macOS + Windows + Linux that **embeds the `stint` CLI** and adds a
GUI, unifying four things WakaTime spreads across separate projects:

1. **Onboarding GUI** — generate/paste api key, set server URL, click "connect"
   → runs `stint connect` + `stint cli install`, writing both config files.
2. **System app-usage tracking** — the `desktop-wakatime`/`macos-wakatime`
   function: watch the focused app, send heartbeats for non-editor apps.
3. **Scheduled collection** — runs `stint collect --watch` in the tray.
4. **Status** — surfaces `stint doctor`.

**Toolkit: Tauri** (Rust shell + system webview) — ~10× smaller than Electron,
one codebase for all three OSes, shells to the Go `stint`/`wakatime-cli` binaries
rather than reimplementing tracking. **The app owns no tracking logic beyond
app-focus watching**, which is the only OS-specific code (macOS Accessibility,
Windows Win32 hooks, Linux X11/Wayland) — kept in a thin platform module.
Ship order: macOS → Windows → Linux.

---

## 7. Maintenance budget

| Stint owns | Size | Why it stays small |
|---|---|---|
| Server wire-compat | small, ~done | one heartbeat schema; verified in smoke tests |
| `stint` CLI | medium | editor config is a registry of rows, not N programs |
| stint-desktop | medium | thin Tauri GUI over the CLI; only app-focus watching is OS-specific |
| `wakatime-cli` | ~zero | reused upstream, never forked |
| Editor plugins (~50) | **zero** | reused upstream via `api_url` |

The number that matters: **plugins maintained = 0.** New editor support is a
config registry row + a smoke-test heartbeat — bounded, testable, decoupled from
each editor's release cycle.

---

## 8. Decision log

Resolved decisions driving this scope. (AI-side items also annotated in
[`wakatime-ai-compat.md`](./wakatime-ai-compat.md).)

### Config
- **C1 — Dual config.** `~/.stint.cfg` primary; `~/.wakatime.cfg` read with the
  same INI parser as a fallback. Precedence: flags → `STINT_*` env →
  `~/.stint.cfg` → `~/.wakatime.cfg` → defaults.
- **C2 — Write both.** `stint setup` writes native keys to `~/.stint.cfg` and
  `api_url`+`api_key` into `~/.wakatime.cfg` (preserving existing keys) so
  upstream plugins reach Stint.
- **C3 — Bootstrap.** Settings page shows a copyable
  `stint setup --server <url> --key <key>`; CLI honors `STINT_API_URL` /
  `STINT_API_KEY`.

### CLI
- **B1 — Toolkit: cobra** (nested subcommands + good `--help`). Swappable.
- **B2 — Naming.** Primary `stint`; keep `stint-collect` alias/symlink.
- **B3 — Auto-config depth.** Default writes config for all editors; `--deep`
  installs the extension + editor-level keys for VS Code & JetBrains only.
- **B4 — wakatime-cli.** Download upstream release into `~/.wakatime/`, pin a
  known-good version, verify checksum, allow `STINT_WAKATIME_CLI` override.
  **Never fork.**

### Desktop
- **D1 — Toolkit: Tauri.**
- **D2 — Boundary.** App owns only per-OS app-focus watching; everything else
  delegates to the `stint` CLI.
- **D3 — Ship order.** macOS → Windows → Linux.

### AI compatibility
- **A1 — additions/deletions.** Wire sends a combined line count; expose as
  `ai_additions` with `ai_deletions: 0`, documented. Revisit deriving a true
  split from `lines` deltas later.
- **A2 — Cost units.** Keep `estimated_cost_cents` (native) **and** add USD-float
  WakaTime aliases (`cents/100`).
- **A3 — Prompt insights.** `PromptCount` is already one prompt per heartbeat
  carrying `ai_prompt_length`; group that signal and prompt-length by
  `ai_session` for avg/median-per-session.

### Scope cuts
- **S1 — Discord** (`discord-wakatime`/`vencord-wakatime`): **out of scope** —
  app-usage vanity tracking, not coding.
- **S2 — Browser extension:** low priority, **manual config only** (sandboxed;
  CLI can't configure it).
- **S3 — Creative/niche app plugins:** tier-3 — "works via config, verified on
  request," not in the initial CI smoke matrix.
- **S4 — Mobile / read-only apps:** out of scope; `stint today`/`stats` covers
  the terminal-report case.

### Still open
- License/attribution check for reusing the upstream `wakatime-cli` binary
  (BSD-3) — confirm terms before bundling in releases.
- Verify each Pattern-C extension's "API URL" option actually accepts a
  self-hosted base (browser-wakatime).

---

## 9. Phased plan

1. **CLI foundation.** Restructure `cmd/collect` into cobra `stint <subcommand>`;
   `collect` keeps current behavior. Add `stint setup` (dual-config write) +
   `stint cli install`.
2. **`stint connect` + editor registry.** Tier-1 first (VS Code family,
   JetBrains, Vim/Neovim, Zed), then fan out the registry to the full Pattern-A
   list.
3. **`stint plugin install`** for the AI hook plugins (claude-code, codex,
   antigravity, amp, copilot) — completes the hybrid AI story.
4. **`stint doctor`** + extend `scripts/smoke-wakatime.sh` to assert a real
   heartbeat per representative plugin (one per pattern).
5. **stint-desktop** (Tauri): onboarding GUI → app-focus tracking → tray
   collector → status. macOS first.
6. **`stint today`/`stats`** terminal output (optional, replaces `WakaTimeCLI`).
