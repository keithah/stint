# Editor Client Setup

Stint collects editor and agent activity through `/api/v1`. Existing WakaTime-compatible plugins can point at Stint by changing `api_url` and `api_key`, while native Stint clients can add richer model, provider, token, and cost metadata.

## 1. Create an API Key

For local development, open `http://localhost:3000/login` when running `npm run dev`, or `http://localhost:3001/login` when using Docker Compose. Choose **Create local dev key**, then open Settings and create/copy the generated Stint API key.

For a deployed instance, sign in through GitHub, open Settings, and create an API key.

## 2. Configure an Existing Editor Client

### Native Stint CLI

Install the latest prebuilt CLI:

```bash
curl -fsSL https://stint.fyi/install.sh | sh
```

Initialize a WakaTime-compatible config file:

```bash
stint config init \
  --api-url http://localhost:8080/api/v1 \
  --api-key waka_00000000-0000-4000-8000-000000000000
```

Send a smoke-test heartbeat:

```bash
stint doctor
stint heartbeat \
  --entity "$PWD/main.go" \
  --write \
  --project stint \
  --plugin stint-cli/release
```

Send an AI-enriched heartbeat directly:

```bash
stint heartbeat \
  --entity "$PWD/main.go" \
  --category "ai coding" \
  --ai-model gpt-5-codex \
  --ai-provider openai \
  --ai-agent codex \
  --ai-agent-version release \
  --metadata '{"source":"manual"}'
```

Common follow-up commands:

```bash
stint today
stint user-agents
stint data-dumps download DUMP_ID
stint offline sync
stint --sync-ai-activity --ai-agent codex
```

The CLI also accepts WakaTime-style root flags, so editor integrations that shell
out to `wakatime-cli --entity ...` can use `stint --entity ...` with the same
`~/.wakatime.cfg` API URL and key.
Offline heartbeats are stored in the WakaTime-compatible BoltDB queue at
`~/.wakatime/offline_heartbeats.bdb`, and request logs default to
`~/.wakatime/wakatime.log`; when `WAKATIME_HOME` is set, config lives at
`$WAKATIME_HOME/.wakatime.cfg` and runtime resources live under
`$WAKATIME_HOME/`. Backoff and heartbeat-rate-limit state live in the
WakaTime-compatible internal config file at `~/.wakatime/wakatime-internal.cfg`.
Offline sync also migrates WakaTime's old `.wakatime.bdb` legacy queue.
Existing JSONL queues still work when
passed explicitly with `--offline-queue-file`.
Offline sync removes WakaTime-style duplicate queued heartbeats for the same
entity when their timestamps are within one second, drops permanent bad-request
results, and requeues transient failures or missing per-heartbeat API results.
For file heartbeats, the CLI detects project and branch from `.wakatime-project`,
Git, Mercurial, and Subversion folders. It also sends lightweight dependency
metadata from common source imports across Go, Python, JavaScript/TypeScript,
Rust `extern crate` declarations, C/C++, Java, C#, Kotlin, Scala, Haskell, Elm,
Haxe, HTML, Objective-C, PHP, Swift, VB.NET, and `package.json` unless
`hide_dependencies = true` is set.
`package.json`, `bower.json`, and `component.json` also include the WakaTime
package-manager markers (`npm` or `bower`) in the dependency list. Dependency
lists preserve first-seen WakaTime parser order, deduplicate repeated names,
follow the resolved or explicit heartbeat language when choosing the parser,
honor WakaTime language aliases such as `CSharp`, `CPP`, `ObjectiveC`, and
`Visual Basic .NET`, and
detect side-effect and multi-line JavaScript/TypeScript imports, Scala grouped
imports, multiline HTML script sources, Rust `extern crate` declarations, and
Gruntfile activity as `grunt`, with payloads capped at 1000 entries. Automatic
local file stats follow WakaTime and read at most the first 5
MiB for language, dependency, and line metadata; unsaved file entities skip
automatic line and dependency detection while still being tracked.
`--guess-language` also honors Vim modelines such as `vim: ft=python`, and
C-family headers are disambiguated from matching source files or neighboring
C/C++ files. Objective-C, Matlab, and Delphi ambiguities use the same
neighbor-file hints as `wakatime-cli`, `.fs` files are scored as F# or Forth
from their contents, and WakaTime top-language aliases such as `crontab`,
`.ruby-version`, `.Rprofile`, `.sublime-settings`, `.vue`, `.svh`, `.xaml`,
`.xpl`, `.inc`, `.i`, `.j`, `.mo`, `.re`, `.swg`, and `.vm` are normalized to
WakaTime display names.
For remote editor entities, pass the remote `ssh://` or `sftp://` path as
`--entity`. Stint preserves the remote entity in the heartbeat and, when
`--local-file` is omitted, downloads up to 512 KB over SFTP with an `scp`
fallback for local stats. Pass a local mirror as `--local-file` when the editor
already has one. `~/.ssh/config` host aliases are honored for `HostName`,
`User`, `Port`, `IdentityFile`, `UserKnownHostsFile`, `HostKeyAlias`, and
`StrictHostKeyChecking`. Credentials embedded in remote URLs are stripped before
the heartbeat is sent. Remote entities follow WakaTime by skipping local
file-existence and `.wakatime-project` include-only filtering.
Heartbeat requests include WakaTime-compatible `X-Machine-Name`, `Timezone`,
`user_agent`, and `project_root_count`, and accept Stint's richer AI fields:
`--ai-model`, `--ai-provider`, `--ai-agent`, `--ai-agent-version`,
`--ai-agent-complexity`, `--commit-hash`, `--plugin-version`, `--editor`,
`--editor-version`, and JSON `--metadata`.
The config loader follows WakaTime's layered config model: `settings.import_cfg`
is loaded after the main `~/.wakatime.cfg`, and the nearest project `.wakatime`
file is applied for file heartbeats. Use that project file for repo-specific
API keys, include/exclude filters, privacy settings, or heartbeat rate limits.
`[DEFAULT]` keys are treated as top-level WakaTime config values, and section
names plus scalar setting keys are case-insensitive. Quoted scalar values are
read without their surrounding quotes, matching `wakatime-cli`. Root
`--config-write key=value,with,commas` preserves single key/value values that
contain commas, and comma-splits only WakaTime-style multi-pair inputs. Native
`stint config read` and `stint config write` accept the same `--config-section`
section flag as WakaTime's root config commands.
`stint today` matches WakaTime status-bar output by printing total human-readable
time by default, simplified JSON with `--output json`, and the full statusbar
payload with `--output raw-json`. Use `--today-hide-categories=false` or
`settings.status_bar_show_categories = true` to show category breakdowns.
Set `settings.status_bar_enabled = false` to return an empty status text when
an editor disables its WakaTime status bar.
Set `settings.status_bar_coding_activity = false` to return an empty status text
when an editor should show only its WakaTime icon.
For heartbeat categories, the CLI follows WakaTime: ordinary file heartbeats
omit `category` and let the API default them to `coding`, while test/spec
paths are sent as `writing tests` and `.md`/`.mdx` files are sent as
`writing docs`.
`stint today-goal GOAL_ID` matches WakaTime goal output by printing today's
current progress text by default, with the full goal payload available through
`--output raw-json`.
`stint --sync-ai-activity` scans local AI transcript files, then sends
WakaTime-shaped `ai coding` heartbeats without requiring `--entity`. Normal file
heartbeat sends run the same scan automatically unless `--sync-ai-disabled` is
set or `settings.sync_ai_disabled = true` is configured, so editor plugins get
upstream-style AI sync by default. Current file-based sources include Codex,
Claude, Continue dev data with session workspace mapping, Amp, Copilot CLI and VS Code workspace storage, Gemini,
Antigravity Desktop/IDE/CLI, Pi, Qoder history, Qwen Code, OpenCode, Kiro,
Cline, Roo Code, and Cody logs. Lightweight SQLite state readers cover Cursor,
Windsurf and Windsurf Next, Goose, Qoder, OpenCode, and Cody chat history. AI
file heartbeats respect the same `include`, `exclude`, and `ignore` filters as
normal file heartbeats. When the local transcript exposes tool activity,
including Codex successful `apply_patch` calls, Amp `apply_patch` logs, Gemini
project-root tool calls, Kiro workspace actions, and Qwen Code function-call
tools, Stint preserves prompt length, read/write intent, and WakaTime-compatible
`ai_line_changes` on generated file heartbeats. It records the newest synced timestamp in
`internal.ai_logs_last_parsed_at` inside `wakatime-internal.cfg` and also reads
the older Stint `internal.ai_sync_after` key as a fallback; use
`--sync-ai-after UNIX_SECONDS` to force a specific lower bound.

The native CLI reads the common WakaTime config sections:

```ini
[settings]
api_url = https://api.example.com/api/v1
api_key = waka_00000000-0000-4000-8000-000000000000
api_key_vault_cmd = op read op://Engineering/Stint/api_key
debug = false
import_cfg = ~/.wakatime/private.cfg
proxy = http://localhost:8888
no_ssl_verify = false
ssl_certs_file = /path/to/private-ca.pem
log_file = ~/.wakatime/wakatime.log
send_diagnostics_on_errors = false
heartbeat_rate_limit_seconds = 120
hide_project_folder = false
hide_dependencies = false
ignore =
    node_modules/

[projectmap]
^/home/me/work/api/ = work-api
^/home/me/clients/client([0-9]+)/ = client-{0}

[git]
project_from_git_remote = false
submodules_disabled = ^/home/me/work/vendor/

[git_submodule_projectmap]
^/home/me/work/.git/modules/lib/billing$ = billing-lib

[project_api_key]
^/home/me/client/ = waka_client_specific_key

[api_urls]
^/home/me/work/ = https://work.example.com/api/v1|waka_work_key
```

For repo-local overrides, create `.wakatime` inside the project:

```ini
[settings]
api_key = waka_project_specific_key
include =
    .*\.go$
exclude =
    /vendor/
hide_file_names = false
heartbeat_rate_limit_seconds = 30
```

`projectmap` changes the project name sent with the heartbeat and supports
WakaTime-style regex capture placeholders such as `{0}`,
`git_submodule_projectmap` overrides detected Git submodule project names,
`git.submodules_disabled` disables submodule-specific detection for matching
paths,
`git.project_from_git_remote` uses the `origin` remote repository path as the
project name when no `.wakatime-project` or `projectmap` override applies and
for `.wakatime-project` `{project}` placeholder interpolation,
`project_api_key` changes the default destination key for matching files, and
`api_urls` adds extra matching destinations while still sending to the default
`api_url`.
When patterns overlap, `projectmap`, `git_submodule_projectmap`, and
`project_api_key` use the first matching entry in file order; `api_urls` fans
out to every matching entry in file order.
Regex values follow WakaTime CLI behavior: matching is case-insensitive unless
the pattern already carries its own inline flags, `--include` and `--exclude`
flags accept comma-separated patterns, config regex lists are newline-separated
and preserve commas inside a pattern, Perl-style regex features such as
lookahead are accepted when Go's native regex engine cannot compile the
pattern, and boolean values such as `true` and `false` are accepted for
include/exclude-style filters.
When `hide_project_folder = true`, local file entities are sent relative to the
detected project root; if no root can be detected, only the filename is sent.
When `hide_project_names` matches a file or project path, the CLI creates or
reuses `.wakatime-project` in that folder and sends the alias instead of the
original project name.
The CLI also accepts common legacy key names such as `apikey`, `hidefilenames`,
`hide_projectnames`, `hideprojectnames`, `hide_branchnames`, and
`hidebranchnames`.
`proxy` accepts WakaTime-compatible `http://`, `https://`, `socks5://`, and bare
`host:port` forms. Bare proxy hosts default to HTTP, and NTLM credential strings
such as `domain\\user:password` are accepted without replacing the Stint API
`Authorization` header.
`api_url`, `api-url`, and `apiurl` values are validated and normalized before
any request is sent.
`timeout = 0` disables the HTTP client timeout, while positive values are
seconds. `heartbeat_rate_limit_seconds = 0` disables heartbeat rate limiting;
non-integer rate-limit config values fall back to the WakaTime default, and
negative rate limits from flags, shared config, and project `.wakatime` files
disable rate limiting like `wakatime-cli`.
When both `--verbose` and `--send-diagnostics-on-errors` are set, or when
`send_diagnostics_on_errors = true` is combined with `--verbose`, CLI command
failures post a WakaTime-shaped diagnostics payload to `/api/v1/plugins/errors`.

### Existing WakaTime-Compatible Plugins

Use this in `~/.wakatime.cfg`:

```ini
[settings]
api_url = https://api.example.com/api/v1
api_key = waka_00000000-0000-4000-8000-000000000000
hide_file_names = false
timeout = 15
```

For local Compose, use:

```ini
[settings]
api_url = http://localhost:8080/api/v1
api_key = waka_00000000-0000-4000-8000-000000000000
hide_file_names = false
timeout = 15
```

For Codex or other clients using multi-destination fanout, use an `api_urls` entry instead of `api_url`:

```ini
[api_urls]
.* = https://api.example.com/api/v1|waka_00000000-0000-4000-8000-000000000000
```

Some existing clients validate `api_urls` keys more strictly than normal `api_key` settings. Stint-generated keys use `waka_<uuid>` so they work there; older bare UUID Stint keys still authenticate with the API but should be replaced for fanout configs.

## 3. Verify Ingestion

Send one heartbeat from your editor, then check:

```bash
curl -fsS -H "Authorization: Bearer waka_00000000-0000-4000-8000-000000000000" \
  "https://api.example.com/api/v1/users/current/stats/last_7_days"
```

For local development, the project smoke test exercises the same path:

```bash
scripts/smoke-wakatime.sh
```

The smoke test sends curl-based activity payloads every run. If `wakatime-cli` is installed, or `WAKATIME_CLI_BIN` points at a binary, it also sends a real CLI heartbeat, verifies the project appears, and runs `wakatime-cli --today`, `wakatime-cli --today-goal`, and `wakatime-cli --file-experts` against the local API.

To verify the native CLI directly:

```bash
STINT_API_KEY=waka_00000000-0000-4000-8000-000000000000 \
  stint --api-url http://localhost:8080/api/v1 --today
```

## Auth Modes

Generated API keys use `waka_<uuid>` so existing editor plugins and fanout configs accept them. Bare UUID keys from older Stint builds are still accepted by the API for self-hosted migrations, but should be replaced before using `api_urls`. Keys can be supplied in all compatibility forms:

- `Authorization: Basic base64(API_KEY:)`
- `Authorization: Bearer API_KEY`
- `?api_key=API_KEY`

OAuth app tokens use `Authorization: Bearer waka_tok_...` and are scope-checked.

## Common Checks

- `api_url` must include `/api/v1`.
- `stint` reads `~/.wakatime.cfg`, `settings.import_cfg`, project `.wakatime`, `WAKATIME_API_KEY`, `STINT_API_KEY`, and `api_key_vault_cmd`.
- `stint --sync-ai-activity` emits native WakaTime-shaped AI heartbeats for Codex successful apply-patch calls, Claude, Continue dev data with session workspace mapping, Amp apply-patch logs, Copilot CLI and VS Code workspace storage, Gemini project-root tool calls, Antigravity Desktop/IDE/CLI, Pi, Qoder history, Qwen Code function-call tools, OpenCode, Kiro workspace actions, Cline, Roo Code, Cody logs, and Cursor, Windsurf/Windsurf Next, Goose, Qoder, OpenCode, and Cody SQLite chat history; prompt length, read/write intent, and `ai_line_changes` are included when available; `stint collect` still runs the bundled Stint collector helper.
- The API service must be reachable from the machine running the editor plugin.
- If Stint is behind a reverse proxy, preserve the request body and `Authorization` header.
- Dashboard totals update after stats recomputation; `/status_bar/today` uses a short cache.
