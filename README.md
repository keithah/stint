# Stint

Stint is a self-hosted, WakaTime-compatible coding activity tracker. The current vertical includes the ingestion pipeline, GitHub/session auth, WakaTime-format API keys, OAuth app/server flows, rate limiting, Redis/Asynq stats jobs, durations, summaries, cached stats ranges, AI metrics, status-bar data, projects, machines, insights, goals, leaderboards, external durations, custom rules, data dumps, all-time totals, and a dark dashboard shell.

## Local Development

Start dependencies:

```bash
docker compose up -d postgres redis
```

Run the API:

```bash
cp .env.example .env
go run ./cmd/server
```

Run the background worker in a second terminal when using queued stats recomputation:

```bash
go run ./cmd/server worker
```

Run the web app:

```bash
cd web
npm install
npm run dev
```

Open `http://localhost:3000/login` for local `npm run dev`, or `http://localhost:3001/login` when using Docker Compose. Choose "Create local dev key", then copy the generated WakaTime config from Settings.
New empty dashboards show a dismissible editor setup modal with the WakaTime help link, local API URL, a copyable `~/.wakatime.cfg` block, and the expected two-minute activity refresh window after opening an editor.
Settings surfaces the signed-in GitHub account identity and sign-out action alongside profile preferences, API keys, OAuth apps, share tokens, data exports, imports, custom rules, AI costs, and account deletion.
Settings includes server diagnostics from `/api/v1/meta`, including the configured API URL, base URL, hostname, detected client IP, and build version.
Settings also shows the `/api/v1/editors` metadata registry so plugin setup can confirm known WakaTime-compatible editors.
Language charts use the `/api/v1/program_languages` catalog colors so WakaTime palette-compatible languages render consistently across summaries and timelines.

Regenerate the sqlc model package after changing migrations or query files:

```bash
scripts/generate-sqlc.sh
```

## WakaTime Plugin Config

Use this in `~/.wakatime.cfg`:

```ini
[settings]
api_url = http://localhost:8080/api/v1
api_key = waka_00000000-0000-4000-8000-000000000000
hide_file_names = false
timeout = 15
```

See [docs/PLUGIN_SETUP.md](docs/PLUGIN_SETUP.md) for editor setup, auth modes, and ingestion checks.

## Compatibility Smoke Test

With the API running:

```bash
scripts/smoke-wakatime.sh
```

The script creates a local dev user/key, sends WakaTime-shaped heartbeats, exercises settings resources, runs OAuth authorization-code, implicit, refresh, and revoke flows, then fetches `/api/v1/users/current/stats/last_7_days`. If `wakatime-cli` is installed, or `WAKATIME_CLI_BIN` points at a binary, the script also sends a real CLI heartbeat, verifies the resulting project, and runs `wakatime-cli --today`, `wakatime-cli --today-goal`, and `wakatime-cli --file-experts`.
The web client also exposes a typed `fileExperts` helper for `/api/v1/users/current/file_experts` so browser tools can call the same compatibility endpoint.

## Optional Local Proxy

The default Compose stack exposes API `:8080` and web `:3001` directly. To try the optional Caddy reverse proxy profile:

```bash
docker compose --profile proxy up -d caddy
```

That exposes the API through `http://localhost:8081` and the web app through `http://localhost:3002`. Override `STINT_API_SITE`, `STINT_WEB_SITE`, `STINT_API_PROXY_PORT`, and `STINT_WEB_PROXY_PORT` for custom hostnames or ports.

## OAuth Apps

Settings can create OAuth clients for WakaTime-compatible third-party apps. The backend exposes:

- `GET /oauth/authorize`
- `POST /oauth/authorize`
- `POST /oauth/token`
- `POST /oauth/revoke`
- `GET/POST /api/v1/oauth/apps`
- `DELETE /api/v1/oauth/apps/:id`

Access tokens use `Authorization: Bearer waka_tok_...` and can call the existing `/api/v1` endpoints according to granted scopes. Authorization-code and refresh-token exchanges issue 365-day access and refresh tokens; expired refresh tokens are rejected. Implicit `response_type=token` authorization issues 12-hour access tokens without a refresh token. The newest eight active tokens are retained per user/application.
OAuth token revocation requires the client secret and only revokes tokens issued to that OAuth app.
OAuth app names must be non-empty. OAuth app redirect URIs must be absolute `http` or `https` URLs and at least one redirect URI is required.
GitHub/session login uses a short-lived signed OAuth state cookie, fetches both the GitHub profile and verified email list, then issues a 30-day first-party HS256 JWT signed with `SESSION_SECRET`; dev seed responses expose it as `access_token`, and production login sets it in an HttpOnly cookie. Public `BASE_URL` deployments fail startup if `SESSION_SECRET` is missing, too short, or left at a known placeholder, and also require `GITHUB_CLIENT_ID` plus `GITHUB_CLIENT_SECRET` unless `DEV_SEED_ENABLED=true` is explicitly set for a private test environment. OAuth access tokens and API keys are checked against granted scopes for scoped endpoints; browser sessions and first-party JWTs have full local-account access. Generated first-party API keys default to the full local scope set.
Current-user profile responses include email only for browser sessions, first-party JWTs, or API/OAuth credentials with the `email` scope.
Granular summary scopes are enforced by requested data: `/durations?slice_by=language` accepts `read_summaries.languages`, project durations accept `read_summaries.projects`, and `/summaries` only includes granted project, language, category, dependency, editor, machine, or operating-system breakdowns while always returning the daily grand total.
Account-management routes such as profile updates, API key management, OAuth app registration, share token management, custom rule mutations, and AI cost writes require local-account access: a browser session, first-party JWT, or full-scope local API key.

The backend serves an OpenAPI 3.1 document at `/api/v1/docs` with per-method route metadata, auth requirements, required path parameters, query parameters for date ranges, filters, pagination, and share `callback`, OAuth form bodies, JSON request bodies for mutation endpoints, plus reusable schemas for the core heartbeat, stats, settings, goals, sharing, imports, and maintenance payloads. `GET /api/v1/meta` returns the detected client IP, runtime hostname, configured base URL, and `/api/v1` URL for WakaTime-compatible clients.

## Rate Limits

Redis-backed sliding-window limits are enabled when `REDIS_URL` is reachable, with in-memory fallback for local development:

- Heartbeat writes: 1000 requests per authenticated user per minute
- Authenticated reads: 60 requests per API key, OAuth token, session, or first-party JWT per minute
- Public reads: 60 requests per IP per minute
- OAuth token creation/refresh: 10 requests per OAuth client and 10 token creations per target user per hour

Blocked requests return `429` with a `Retry-After` header.

## Self-Host Controls

Use `ENABLE_REGISTRATION=false` after creating your account to close GitHub signups while existing users can still log in. Set `MAX_USERS` to a positive integer to cap local accounts, or leave it `0` for unlimited users. Set `ENABLE_PUBLIC_LEADERBOARD=false` to disable the public `/api/v1/leaders` endpoint without affecting private leaderboards or share tokens. `DEV_SEED_ENABLED` defaults on only for localhost `BASE_URL` values; set it explicitly for private test environments and keep it disabled on public deployments.

Data dumps require `STORAGE_TYPE=local` and write completed JSON snapshots under `STORAGE_PATH` before exposing the download URL. Startup rejects other storage types so self-hosted deployments do not silently misplace exports. The app also loads the spec's `S3_*`, `AWS_*`, `SMTP_*`, and `EMAIL_FROM` variables so self-hosted environments can keep a complete config file while remote object storage and email notifications are added later.

## Implemented API

- `GET /healthz`
- `GET /api/v1/meta`
- `GET /api/v1/docs`
- `GET /api/v1/leaders`
- `GET /api/v1/editors`
- `GET /api/v1/program_languages`
- `GET /api/v1/auth/me`
- `GET /api/v1/users/:user`
- `GET /api/v1/users/:user/stats`, `GET /api/v1/users/:user/stats/:range`
- `GET /api/v1/users/:user/summaries`
- `GET/PUT/DELETE /api/v1/users/current`
- `POST /api/v1/users/current/heartbeats`
- `GET /api/v1/users/current/heartbeats`
- `POST /api/v1/users/current/heartbeats.bulk`
- `DELETE /api/v1/users/current/heartbeats.bulk`
- `POST /api/v1/users/current/file_experts`
- `GET /api/v1/users/current/durations`
- `GET /api/v1/users/current/summaries`
- `GET /api/v1/users/current/stats`
- `GET /api/v1/users/current/stats/last_7_days`
- `GET /api/v1/users/current/stats/:range`
- `GET /api/v1/users/current/status_bar/today`
- `GET /api/v1/users/current/statusbar/today`
- `GET /api/v1/users/current/projects`
- `GET /api/v1/users/current/projects/:project`
- `GET /api/v1/users/current/projects/:project/commits`
- `GET /api/v1/users/current/projects/:project/commits/:hash`
- `GET /api/v1/users/current/machine_names`
- `GET /api/v1/users/current/insights/:insight_type/:range`
- `GET /api/v1/users/current/all_time_since_today`
- `GET/POST /api/v1/users/current/goals`
- `GET/PUT/DELETE /api/v1/users/current/goals/:goal`
- `GET/POST /api/v1/users/current/leaderboards`
- `GET/PUT/DELETE /api/v1/users/current/leaderboards/:board`
- `POST /api/v1/users/current/leaderboards/:board/members`
- `DELETE /api/v1/users/current/leaderboards/:board/members/:user`
- `GET/POST /api/v1/users/current/external_durations`
- `POST/DELETE /api/v1/users/current/external_durations.bulk`
- `GET/PUT /api/v1/users/current/custom_rules`
- `DELETE /api/v1/users/current/custom_rules/:rule_id`
- `GET/DELETE /api/v1/users/current/custom_rules_progress`
- `GET/POST /api/v1/users/current/data_dumps`
- `GET /api/v1/users/current/data_dumps/:dump/download`
- `GET/POST /api/v1/users/current/share_tokens`
- `DELETE /api/v1/users/current/share_tokens/:id`
- `POST /api/v1/users/current/imports/wakatime`
- `GET/PUT /api/v1/users/current/ai_costs`
- `GET /api/v1/users/:user/share/:token/stats`
- `GET /api/v1/users/:user/share/:token/summaries`
- `GET /api/v1/share/:token/stats`
- `GET /api/v1/share/:token/summaries`
- `GET/POST /api/v1/api_keys`
- `DELETE /api/v1/api_keys/:id`
- `GET/POST /api/v1/oauth/apps`
- `DELETE /api/v1/oauth/apps/:id`
- `GET /auth/github/login`, `GET /auth/github/callback`
- `GET/POST /oauth/authorize`, `POST /oauth/token`, `POST /oauth/revoke`

Supported stats ranges are `last_7_days`, `last_30_days`, `last_6_months`, `last_year`, `all_time`, calendar years like `2026`, and calendar months like `2026-06`.
Stats endpoints return cached data. A stale cache row is served with HTTP `202 Accepted` and `is_up_to_date:false` while a refresh job is queued; missing cache rows are computed inline for local-first usability.

Generated API keys use `waka_<uuid>` so editor plugins and WakaTime CLI `api_urls` fanout accept them. Bare UUID keys from older Stint builds are still accepted by the API for self-hosted migrations, but should be replaced before using `api_urls`. API keys are accepted through WakaTime-compatible Basic auth, Bearer auth, and the `api_key` query parameter. `POST /api/v1/api_keys` accepts optional `scopes`; blank scopes create the default full local key, while explicit scopes are validated against the OAuth/WakaTime scope list.
API key and share token creation require non-empty names; the API and database reject blank names.
Heartbeat ingestion accepts WakaTime's current `dependencies` array shape and the older string form, normalizing both for local storage and stats.
Heartbeat ingestion also treats WakaTime's `alternate_project` as a project fallback when `project` is absent.
WakaTime User-Agent parsing stores plugin name/version, editor name/version, operating system, and architecture on each heartbeat so raw heartbeat exports preserve client metadata.

`/api/v1/users/current/status_bar/today` uses a Redis-backed two-minute TTL cache when `REDIS_URL` is reachable, with an in-memory fallback for local development.
Day-based heartbeats, durations, summaries, public summaries, and status-bar totals use the user's configured profile timezone.

`/api/v1/leaders` uses a Redis-backed one-hour TTL cache refreshed by the hourly `leaderboard:update` worker job, with live computation and in-memory caching as a local fallback. The public endpoint includes avatar/display/country metadata and accepts `?language=Go` and `?country=US` style filters. Worker and live leaderboard totals include external durations in the selected range.
Private leaderboard owners can add or remove other local Stint users by GitHub username; rankings include every current board member. Private leaderboard names must be non-empty, and `time_range` accepts the supported stats ranges, calendar years, and calendar months.

Public user profile, stats, and summaries endpoints are opt-in through `has_public_profile` on the current user settings. Tokenized share links remain available for private per-link sharing.
The web app exposes opt-in public profiles at `/users/:user`, backed by the public profile, stats, and summaries endpoints.
Share tokens expose both stats and summaries through user-scoped embed URLs and token-only `/api/v1/share/:token/*` aliases used by `/share/:token`. Public share stats and summaries also accept `?callback=StintEmbed.render` for JSONP embeds.
Profile settings validate IANA timezone names, non-negative timeout and retention values, and optional two-letter country codes before updating stats-affecting preferences.

When `writes_only` is enabled, computed stats, summaries, durations, status bar results, goals, and leaderboard totals ignore non-write heartbeats. Raw heartbeat listing and heartbeat data dumps still return stored raw events.

The dashboard hero includes selected-range total, current-day total, daily average, best day, and all-time totals. The dashboard AI panel shows an AI line-share ring, AI/human line share, human review rate, follow-up edits, prompt count, average and median prompt length, token totals, estimated cost, daily/weekly/monthly agent cost tracking, an agents donut by AI line changes, agent breakdowns, a compact day heatmap, a fixed 30-day AI-vs-human daily changes trend, and a weekday activity heatmap using stored heartbeat fields. The dashboard also includes a daily stacked project chart, project/language/editor/machine/operating-system donuts, plus a category bar chart. The dashboard project grid breaks the same range down by project with AI changes, human changes, prompt volume, sessions, token volume, spend, and active time. Range stats also include per-day project slices and 24-hour project and language timelines for the dashboard.
AI cost settings require a non-empty agent name and non-negative input/output cents per million tokens; invalid values are rejected before replacing saved rates.
Project detail pages include selectable range totals, branch activity, dependency totals, language/editor splits, a paged branch-filterable commit timeline with external commit links, branch/page and single-commit API helpers, and the same AI panel scoped to that project.
Insights include range-aware breakdowns, weekday pattern rows with active-day counts and average active-day time, a daily-average trend view, and a range activity heatmap.
Reports include date-range summary export, single-day duration breakdowns by WakaTime slice, external duration creation/delete maintenance, queued data dumps, and a raw heartbeat inspector for single-day list/delete maintenance.
External durations require `external_id`, `provider`, `entity`, `type`, a positive `start_time`, and an `end_time` later than `start_time`; the same rules are enforced by API validation and database constraints.

Goals support day/week targets with project, language, and editor filters, ignored weekdays, zero-day ignoring, inverse "stay under" targets, improvement percentages, snoozing, and enabled/disabled state. Heartbeats and matching external durations both count toward goal progress. The Goals UI can create and edit goals in modal forms, then toggle or delete existing goals from their cards. The `goals:evaluate` worker job is scheduled hourly and records pass/fail rows for each enabled goal when a user reaches their local midnight hour.

`POST /api/v1/users/current/data_dumps` creates queued `heartbeats` or `daily` exports when Redis/Asynq is available. The worker writes a local JSON snapshot under `STORAGE_PATH`, marks the dump complete, and attaches the download URL; local runs without a queue generate the same snapshot inline. Downloaded dump files are raw top-level JSON arrays: heartbeat rows for `heartbeats` and daily summary rows for `daily`.
The Settings UI can trigger heartbeat or daily summary exports and download completed dump files from the latest export list.

`POST /api/v1/users/current/imports/wakatime` accepts WakaTime JSON dumps as raw JSON or multipart `file` uploads, including gzip-compressed `.json.gz` files. Imports queue `wakatime_import:process` when Redis/Asynq is available and fall back to inline processing when no queue is configured. Duplicate and invalid heartbeats are skipped.

`PUT /api/v1/users/current/custom_rules` applies rules to new heartbeats immediately and queues a retroactive rewrite for existing heartbeats. The worker refreshes stats cache after the rewrite; local runs without a queue apply rules inline.
Custom rules accept `equals`, `contains`, `starts_with`, `ends_with`, and `regex`/`matches` operations; regex patterns, action/operation enums, required source fields, and change-rule destinations are validated before rules are saved and backed by database constraints.
The Settings UI can add change/delete rules across heartbeat fields, choose match operations and destinations, delete individual rules, and monitor the retroactive apply job.
`/api/v1/users/current/custom_rules_progress` reports persisted queued/processing/completed/failed/aborted state for that retroactive rewrite, including heartbeat totals and changed/deleted counts. DELETE marks queued work as aborted so workers skip it.

The weekly `heartbeats:purge` worker job uses `HEARTBEAT_RETENTION_DAYS` as a global override when it is greater than `0`; local Compose defaults to `365`. Set it to `0` to use each user's `heartbeat_retention_days` profile setting instead, where `0` keeps all heartbeats for that user.

## Deployment

The backend Dockerfile builds a single API image. Deploy it with your own container platform and provide managed Postgres/Redis connection strings through environment variables:

```bash
docker build -t stint-api .
docker run -p 8080:8080 \
  -v stint-dumps:/data/dumps \
  -e DATABASE_URL=... \
  -e REDIS_URL=... \
  -e SESSION_SECRET="$(openssl rand -hex 32)" \
  -e GITHUB_CLIENT_ID=... \
  -e GITHUB_CLIENT_SECRET=... \
  -e BASE_URL=https://api.example.com \
  -e WEB_BASE_URL=https://app.example.com \
  -e STORAGE_PATH=/data/dumps \
  stint-api
```

Run the same image with `worker` as the command to process Asynq jobs, mounting the same dump volume when API and worker run separately.
See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for the provider-neutral service layout, environment variables, reverse proxy notes, and smoke checks.
