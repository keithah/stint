# AI Cost Tracking

Stint tracks multi-agent AI coding spend as a first-class, private, cross-agent
view. A local **collector** reads each agent's own data files, normalizes them
to canonical usage events, and posts them to the Stint server, which prices them
with a LiteLLM-sourced engine and serves unified breakdowns.

This subsystem is **separate from** the heartbeat `ai_*` fields (which remain the
source of truth for line-change/session metrics and WakaTime compatibility).

## Architecture

```
dev machine                         Stint server (Docker)
  cmd/collect (stint-collect)          POST /usage_events.bulk  -> usage_events table
   ├─ adapter registry        ─────►   (request-level dedup, ON CONFLICT)
   ├─ normalize -> usage.Event          GET  /usage_events/summary -> pricing.Engine -> breakdowns
   ├─ dedup (eventId)                   GET  /usage_events        -> raw export
   └─ incremental offset state        web/app/ai-costs -> unified dashboard
```

Three decoupled layers — adding an agent never touches pricing or UI:
1. **Source adapters** (`internal/collector`) — one per agent, pure file → events.
2. **Canonical event** (`internal/usage`) — the shared normalized record.
3. **Pricing engine** (`internal/pricing`) — model + token-type → USD.

## Canonical event schema (`internal/usage/event.go`)

| field | meaning |
|---|---|
| `event_id` | stable dedup key (see below) |
| `message_id`, `request_id` | provider ids when present |
| `agent`, `session_id`, `project` | source identity |
| `model` | raw provider string (normalized at pricing time) |
| `input_tokens`, `output_tokens` | fresh tokens |
| `cache_create_5m_tokens`, `cache_create_1h_tokens` | ephemeral cache writes |
| `cache_read_tokens` | cache hits (cheap) |
| `reasoning_tokens` | separately-reported thinking tokens |
| `cost_usd_provided` | provider's own cost, if any |
| `timestamp`, `tz_offset_minutes` | RFC3339 UTC + original local offset |
| `billing_type` | `api` or `subscription` (flat-rate ⇒ $0 marginal) |

**eventId / dedup:** `messageId+requestId` when both exist, else `messageId`/
`requestId` alone, else a hash of `(agent, sessionId, timestamp, model, token
shape)`. Dedup happens both in the collector and at ingest (`ON CONFLICT (user_id,
event_id) DO NOTHING`), so re-scanning or re-posting never double-counts.

## Pricing formula

Per-token-type, mirroring ccusage:

```
cost = input          * inputPrice
     + output         * outputPrice
     + cacheCreate5m  * cacheCreatePrice
     + cacheCreate1h  * (cache1hPrice, or inputPrice*2 if unspecified)
     + cacheRead      * cacheReadPrice
     + reasoning      * outputPrice
```

- **Source of truth:** LiteLLM `model_prices_and_context_window.json`, bundled at
  `internal/pricing/data/litellm_prices.json` for fully-offline operation.
- **Lookup priority:** user override → LiteLLM (cache-accurate, ccusage parity) →
  **OpenRouter fallback** (`internal/pricing/data/openrouter_prices.json`, from
  `GET https://openrouter.ai/api/v1/models` — broad coverage of proxy/free/new
  models LiteLLM lacks; refreshable via `Engine.SetFallbackFromOpenRouter`) →
  unpriced. OpenRouter has no 5m/1h cache split, so LiteLLM is consulted first to
  keep Anthropic cache costs exact; no API key is needed (the catalog is public).
- **Cost modes:** `auto` (provider cost if present, else calculate), `calculate`
  (always recompute — consistent cross-period), `display` (provider cost only).
- **Model normalization:** region/proxy prefixes stripped (`us.anthropic.`,
  `openrouter/…`), trailing `-YYYYMMDD` removed, alias table for the rest.
- **Overrides:** a user `custom-pricing.json` (per-token prices keyed by model)
  prices private/proxied/unknown models; an unknown model is flagged **unpriced**,
  never silently $0.
- **Subscription:** subscription usage reports equivalent-API `cost_usd` and
  `marginal_usd = 0`, so totals don't imply real out-of-pocket spend.

## Supported source paths

Implemented: **Claude Code, Codex, Gemini, OpenCode, Goose, Zed**.
Registered/stubbed (parser pending): cursor, copilot, amp, qwen, kimi, kiro,
kilo, roo, cline, hermes, pi-agent, openclaw, factory-droid, crush, octofriend.

| Agent | Path | Format | Status |
|---|---|---|---|
| Claude Code | `~/.claude/projects/**/*.jsonl` | JSONL | ✅ verified |
| Codex | `~/.codex/sessions/**/rollout-*.jsonl` | JSONL | ✅ verified |
| Gemini | `~/.gemini/tmp/**/chats/session-*.json` | JSON | ✅ verified |
| OpenCode | `~/.local/share/opencode/opencode.db` | SQLite | ✅ verified |
| Goose | `~/.local/share/goose/sessions/sessions.db` | SQLite | schema-only |
| Zed | `~/.local/share/zed/threads/threads.db` | SQLite | schema-only |
| Cursor | `~/.cursor` usage export | CSV/SQLite | stub |
| Copilot | `~/.copilot/otel/` | OTEL | stub |

Per-agent token quirks the adapters handle: Claude writes the same message
multiple times while streaming (growing `output_tokens`) — the adapter keeps the
**max-output** row per `messageId+requestId` before dedup, matching ccusage.
Codex reports cumulative `total_token_usage` plus per-turn `last_token_usage`;
the adapter emits the **per-turn delta** to avoid double-counting. Codex and
Gemini report input **inclusive** of cached tokens, so the adapter stores
`input − cached` and routes cached to `cache_read`, and reasoning/thoughts to
`reasoning_tokens`. OpenCode is read from the authoritative SQLite `message`
table (not the JSON files). All SQLite DBs are opened read-only.

"verified" = cross-checked against real data on a dev host (Claude additionally
against `ccusage --mode calculate`, all token types within ~0.7%). "schema-only"
= implemented to the documented schema with fixture tests, pending real data.

Every base path is overridable via `STINT_COLLECT_<AGENT>_DIR` (e.g.
`STINT_COLLECT_CLAUDE_DIR`). VS Code remote installs (`~/.vscode-server`,
`~/.config/Code`) are resolved for agents stored under `globalStorage/`.

## Running the collector

```bash
go build -o stint-collect ./cmd/collect
STINT_API_URL=https://stint.fyi/api/v1 STINT_API_KEY=waka_... ./stint-collect
# flags: --agent <id>  --dry-run  --once (default)  --watch --interval  --state <path>
```

State (incremental offsets) lives at `~/.stint/collector-state.json`; re-runs only
read new bytes/lines.

## Adding a new agent

1. Add an `AgentSpec` (id, default paths, format) to the registry in
   `internal/collector/registry.go`.
2. Write a parser: `func(...) ([]usage.Event, ScanReport, error)` that reads the
   files, skips non-usage/malformed lines (count, never throw), and maps tokens
   into the canonical event — **preserving cache granularity**.
3. Add fixture files under `internal/collector/testdata/<agent>/` and a test
   asserting token sums, dedup, and skip behavior.
4. If the model strings are novel, extend the alias table in `internal/pricing`.

No pricing, storage, or UI changes are needed.

## Verifying against ccusage

`scripts/ccusage-crosscheck.sh` diffs Stint's Claude-only `calculate`-mode totals
against `ccusage daily --mode calculate --json`. Input and cache-read should match
within rounding; treat a real divergence as a bug.
