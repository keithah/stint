# stint-collect

`stint-collect` scans the local data files written by your AI coding agents,
normalizes them into canonical usage events, and posts them to a Stint server,
which prices them and serves a unified cross-agent cost dashboard. See
[`AI_COST.md`](../../AI_COST.md) for the architecture and event schema.

The collector is **local-file → events → POST**. It never streams in real time;
it scans, posts, optionally sleeps, and repeats.

## Build

```sh
make collect            # builds ./bin/stint-collect
make collect-install    # builds + installs to $GOBIN (or $GOPATH/bin, or ~/.local/bin)
```

Or directly:

```sh
go build -o bin/stint-collect ./cmd/collect/
```

## Quick start

```sh
# 1. Write a starter config to ~/.stint/collect.json
stint-collect config init

# 2. Edit it: set api_url and api_key (see collect.example.json for all fields)

# 3. Dry-run to see what would be sent (no POST, no API key needed)
stint-collect --dry-run

# 4. Real run
stint-collect

# Inspect the effective config (api_key is redacted)
stint-collect --print-config
```

## Configuration

Settings are resolved from these layers, **highest precedence first**:

1. **Explicit command-line flags** (e.g. `--api-url`)
2. **Environment variables** (`STINT_API_URL`, ...)
3. **Config file** (`~/.stint/collect.json`, override with `--config PATH`)
4. **Built-in defaults**

A flag only overrides lower layers when you actually pass it; a flag left at its
default does not clobber an env var or config file value.

### Config file

Default path `~/.stint/collect.json` (override with `--config`). Stdlib JSON,
unknown fields rejected. See [`collect.example.json`](./collect.example.json).

| field | type | meaning |
|---|---|---|
| `api_url` | string | Stint API base URL, e.g. `https://stint.example.com/api/v1` |
| `api_key` | string | Stint API key (sent as `Bearer`) |
| `cost_mode` | string | cost-mode hint (`calculate` \| `provided`); default `calculate` |
| `state_path` | string | incremental-state file; default `~/.stint/collector-state.json` |
| `watch` | bool | run a poll loop instead of a single scan; default `false` |
| `interval` | string | poll interval when watching, Go duration e.g. `"5m"`; default `5m` |
| `agents` | string[] | optional allowlist of agent ids to scan (default: all registered) |
| `agent_paths` | object | per-agent base-dir overrides: `{"<id>": ["/custom/dir", ...]}` |

`~` is expanded in `state_path` and all `agent_paths` entries.

`agent_paths` overrides the registry's built-in default paths for an agent. It
takes precedence over the per-agent env override `STINT_COLLECT_<AGENT>_DIR`
(e.g. `STINT_COLLECT_CLAUDE_DIR`), which still works when `agent_paths` has no
entry for that agent.

### Environment variables

| var | maps to |
|---|---|
| `STINT_API_URL` | `api_url` |
| `STINT_API_KEY` | `api_key` |
| `STINT_COST_MODE` | `cost_mode` |
| `STINT_STATE_PATH` | `state_path` |
| `STINT_WATCH` | `watch` (`1`/`true`/`yes`/`on`) |
| `STINT_INTERVAL` | `interval` |
| `STINT_COLLECT_<AGENT>_DIR` | per-agent base dirs (OS path-list separated) |

### Flags

| flag | meaning |
|---|---|
| `--config PATH` | config file path (default `~/.stint/collect.json`) |
| `--api-url URL` | Stint API base URL |
| `--api-key KEY` | Stint API key |
| `--cost-mode MODE` | cost-mode hint |
| `--state PATH` | incremental-state file path |
| `--agent ID` | scan only this agent (overrides the config `agents` allowlist) |
| `--watch` | poll loop instead of a single scan |
| `--interval DUR` | poll interval when `--watch` (e.g. `5m`) |
| `--once` | force a single scan and exit (default true; cleared by `--watch`) |
| `--dry-run` | scan and report only; do not POST (no api key required) |
| `--print-config` | print the resolved config (api key redacted) and exit |
| `--init-config` | write a starter config to `--config` if absent, then exit |

`config init` is also available as a subcommand: `stint-collect config init [--config PATH]`.

## Watch / cron usage

The collector does not stream. Pick one of two scheduling models:

**Built-in watch loop** — every `interval` it scans all selected agents, posts
new events, prints a per-cycle summary, then sleeps:

```sh
stint-collect --watch --interval 5m
# or set "watch": true, "interval": "5m" in the config file
```

**Cron / systemd timer** — let the OS schedule single runs (leave `watch`
false). Incremental state means each run only reads new file content:

```cron
*/5 * * * * /home/you/.local/bin/stint-collect >> ~/.stint/collect.log 2>&1
```

Either way is safe to run repeatedly: posts are deduped server-side by
`event_id`, and the local state cursor advances only after a successful POST.

## Docker Compose collector

The repo ships an opt-in `collector` service (profile `collector`) that runs
this binary in a container on a `--watch` loop against the local `api` service:

```sh
docker compose --profile collector up -d collector
```

Configure it via the `STINT_COLLECT_*` block in `.env` (copy from
`.env.example`). `STINT_COLLECT_API_KEY` is required.

**Set `STINT_COLLECT_HOME` to your absolute home path** (e.g. `/home/youruser`).
Compose does not shell-expand `${HOME}` from a `.env` file value, so the literal
must be set there; the `${STINT_COLLECT_HOME:-${HOME}}` default in the compose
file only fills in when `HOME` is exported in the shell running `docker compose`.

### Mount layout

The container runs with `HOME=/host-home` so each adapter's `~/.<agent>` default
path resolves. Rather than mounting the whole host home (which would expose
`~/.ssh`, `~/.aws`, `~/.netrc`, etc.), only the per-agent data roots are bind
mounted read-only into `/host-home`:

Only the dirs the **implemented** adapters read are mounted by default:

| Host path (under `$STINT_COLLECT_HOME`) | Container path | Adapters |
|---|---|---|
| `.claude` | `/host-home/.claude` | Claude Code |
| `.codex` | `/host-home/.codex` | Codex |
| `.gemini` | `/host-home/.gemini` | Gemini |
| `.local/share` | `/host-home/.local/share` | OpenCode, Goose, Zed (SQLite; broader) |

`docker-compose.yml` carries commented-out lines for the stubbed agents
(`.cursor` `.copilot` `.qwen` `.kimi` `.kiro` `.factory` `.hermes` `.openclaw`
`.pi` `.config/Code`). Docker errors if a bind-mount **source** directory is
missing, so only uncomment a line once that adapter ships *and* the dir exists
on your host.

### State directory

Incremental scan state is written to `/state/collector-state.json`, backed by
the committed `./.collector-state` bind mount (override with
`STINT_COLLECT_STATE_DIR`). This dir is committed (with a `.gitkeep`) so it
already exists owned by the cloning user — if it were missing, Docker would
auto-create the bind source as **root**, and the collector (running as
`STINT_COLLECT_UID`, default 1000) could not write it, so state would never
persist and every restart would re-scan and re-post everything. The state dir
**must be writable by `STINT_COLLECT_UID`**.

## Supported agents

Real adapters: `claude`, `codex`, `gemini`, `opencode`, `goose`, `zed`.

Registered stubs (discoverable, no events yet): `cursor`, `copilot`, `amp`,
`qwen`, `kimi`, `kiro`, `kilo`, `roo`, `cline`, `hermes`, `pi-agent`,
`openclaw`, `factory-droid`, `crush`, `octofriend`.

Run `stint-collect --dry-run` to see each agent's default scan paths and counts.
See [`AI_COST.md`](../../AI_COST.md) for per-agent details.
