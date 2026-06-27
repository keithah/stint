# Stint — Open-Source WakaTime Rebuild Spec

**Goal:** A self-hostable, open-source coding activity tracker that is 100% API-compatible with WakaTime, beautiful by default, and built with an AI-first lens. GitHub-only SSO. No teams feature. Free forever.

---

## 1. Project Overview

### Name
**Stint** (or pick your own — the codebase should make the name easy to swap)

### What it is
A drop-in replacement for WakaTime's backend + dashboard. Any IDE plugin (wakatime-cli, VS Code extension, Zed, Neovim, etc.) that already sends heartbeats to WakaTime can point at Stint with zero plugin changes — just update the `api_url` in `~/.wakatime.cfg`.

### What it is NOT
- Not a team/org product (no org dashboards, no org billing)
- Not multi-provider SSO (GitHub only)
- Not a SaaS with per-seat billing

### Why it wins over existing OSS alternatives
- **Wakapi** (Go) — API-compatible but minimal UI
- **Wakana** (TypeScript) — better UI but incomplete API and unmaintained
- Stint: complete API + a polished dark-mode dashboard that looks better than WakaTime's

---

## 2. Tech Stack

### Backend
- **Runtime:** Go 1.22+
- **Framework:** [Echo](https://echo.labstack.com/) or [Gin](https://gin-gonic.com/) — pick Echo for cleaner middleware
- **Database:** PostgreSQL 15+ (primary), Redis 7+ (caching + job queues)
- **Job queue:** [Asynq](https://github.com/hibiken/asynq) (Redis-backed, simple, battle-tested)
- **ORM:** [sqlc](https://sqlc.dev/) for type-safe queries, raw migrations via [golang-migrate](https://github.com/golang-migrate/migrate)
- **Auth:** GitHub OAuth 2.0 only (no password auth, no other providers)
- **API key:** `waka_<uuid>` token, stored hashed; bare UUID tokens from older Stint builds accepted for self-hosted migrations

### Frontend
- **Framework:** [Next.js 14](https://nextjs.org/) with App Router
- **Styling:** [Tailwind CSS](https://tailwindcss.com/) + [shadcn/ui](https://ui.shadcn.com/) components
- **Charts:** [Recharts](https://recharts.org/) for time-series + [Nivo](https://nivo.rocks/) for heatmaps and donuts
- **State:** [Zustand](https://github.com/pmndrs/zustand) (UI state) + [TanStack Query](https://tanstack.com/query) (server state)
- **Design system:** Dark-mode-first, WakaTime-inspired color palette — deep charcoal backgrounds, vibrant accent colors per language/editor (match WakaTime's exact hex codes)

### Infrastructure (self-hosted defaults)
- Single `docker-compose.yml` with: app, postgres, redis, optional reverse-proxy (Caddy)
- 12-factor config via environment variables
- Container-platform-neutral deployment; run the API and worker containers wherever you host Postgres and Redis

---

## 3. Authentication

### GitHub OAuth
1. User clicks "Sign in with GitHub"
2. Redirect to `https://github.com/login/oauth/authorize?scope=read:user,user:email`
3. Exchange code for token, fetch `/user` and `/user/emails` from GitHub API
4. Upsert user record keyed on GitHub user ID
5. Issue a session cookie (HttpOnly, SameSite=Strict, 30-day expiry) + a JWT for API access

### API Keys
- Users can generate named API keys from Settings
- Keys are UUID-format strings so `wakatime-cli` accepts them before sending a request
- Stored as a versioned SHA-256 token hash in DB; legacy bcrypt hashes verify once and are lazily upgraded
- Keys support the same scopes as WakaTime OAuth tokens
- Accept via:
  - `Authorization: Basic base64(api_key:)` (wakatime-cli default)
  - `Authorization: Bearer API_KEY`
  - `?api_key=API_KEY` query param

### OAuth 2.0 Server (for third-party apps)
Implement a full OAuth 2.0 authorization server so apps that authenticate against WakaTime can authenticate against Stint:
- `GET /oauth/authorize` — show consent screen, redirect with code
- `POST /oauth/token` — exchange code → access token + refresh token
- `POST /oauth/revoke` — revoke token
- Scopes mirror WakaTime exactly (see Section 5)
- Access tokens: `waka_tok_` prefix, 365-day expiry (code flow) / 12-hour expiry (implicit)
- Refresh tokens: 365-day expiry
- Token revocation requires the OAuth client secret and only revokes tokens issued to that client
- Max 8 active tokens per user per app; oldest revoked when exceeded

---

## 4. Data Model

### Core Tables

```sql
users
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid()
  github_id       bigint UNIQUE NOT NULL
  github_username text NOT NULL
  email           text
  full_name       text
  avatar_url      text
  timezone        text DEFAULT 'UTC'
  timeout_minutes int DEFAULT 15
  writes_only     boolean DEFAULT false
  is_hireable     boolean DEFAULT false
  has_public_profile boolean DEFAULT false
  api_key_hash    text  -- primary personal API key
  created_at      timestamptz DEFAULT now()
  modified_at     timestamptz DEFAULT now()

api_keys
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid()
  user_id         uuid REFERENCES users(id) ON DELETE CASCADE
  name            text NOT NULL
  key_hash        text NOT NULL
  scopes          text[] DEFAULT '{}'
  last_used_at    timestamptz
  created_at      timestamptz DEFAULT now()

heartbeats
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid()
  user_id         uuid REFERENCES users(id) ON DELETE CASCADE
  entity          text NOT NULL        -- file path, url, app name
  type            text NOT NULL        -- file | app | url | domain
  category        text                 -- coding | debugging | etc
  time            double precision NOT NULL  -- unix epoch with fractional seconds
  project         text
  branch          text
  language        text
  machine_name_id uuid REFERENCES machine_names(id)
  editor          text                 -- parsed from User-Agent
  operating_system text               -- parsed from User-Agent
  dependencies    text
  lines           int
  line_number     int
  cursor_pos      int
  is_write        boolean DEFAULT false
  -- AI fields
  ai_line_changes     int
  human_line_changes  int
  ai_session          text
  ai_input_tokens     int
  ai_output_tokens    int
  ai_prompt_length    int
  ai_subscription_plan text
  ai_model            text
  ai_agent            text
  ai_agent_version    text
  ai_agent_complexity text
  created_at      timestamptz DEFAULT now()

-- Index heavily:
-- (user_id, time) BRIN index for time-range queries
-- (user_id, project, time) for project-filtered queries
-- (user_id, language, time) for language stats

machine_names
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid()
  user_id         uuid REFERENCES users(id) ON DELETE CASCADE
  name            text NOT NULL
  value           text
  ip              inet
  timezone        text
  last_seen_at    timestamptz
  created_at      timestamptz DEFAULT now()

projects
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid()
  user_id         uuid REFERENCES users(id) ON DELETE CASCADE
  name            text NOT NULL
  color           text
  has_public_url  boolean DEFAULT false
  badge           text
  first_heartbeat_at timestamptz
  last_heartbeat_at  timestamptz
  created_at      timestamptz DEFAULT now()
  UNIQUE(user_id, name)

goals
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid()
  user_id         uuid REFERENCES users(id) ON DELETE CASCADE
  title           text
  custom_title    text
  delta           text DEFAULT 'day'   -- day | week
  seconds         int NOT NULL
  languages       text[]
  editors         text[]
  projects        text[]
  ignore_days     text[]
  ignore_zero_days boolean DEFAULT false
  improve_by_percent float
  is_enabled      boolean DEFAULT true
  is_inverse      boolean DEFAULT false
  is_snoozed      boolean DEFAULT false
  snooze_until    timestamptz
  created_at      timestamptz DEFAULT now()
  modified_at     timestamptz DEFAULT now()

goal_evaluations
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid()
  user_id         uuid REFERENCES users(id) ON DELETE CASCADE
  goal_id         uuid REFERENCES goals(id) ON DELETE CASCADE
  period_start    timestamptz NOT NULL
  period_end      timestamptz NOT NULL
  actual_seconds  int NOT NULL
  target_seconds  int NOT NULL
  percent         int NOT NULL
  is_complete     boolean NOT NULL
  evaluated_at    timestamptz DEFAULT now()
  UNIQUE(goal_id, period_start, period_end)

leaderboards
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid()
  user_id         uuid REFERENCES users(id) ON DELETE CASCADE
  name            text NOT NULL
  time_range      text DEFAULT 'last_7_days'
  created_at      timestamptz DEFAULT now()
  modified_at     timestamptz DEFAULT now()

leaderboard_members
  leaderboard_id  uuid REFERENCES leaderboards(id) ON DELETE CASCADE
  user_id         uuid REFERENCES users(id) ON DELETE CASCADE
  role            text DEFAULT 'member'  -- owner | admin | member
  PRIMARY KEY (leaderboard_id, user_id)

external_durations
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid()
  user_id         uuid REFERENCES users(id) ON DELETE CASCADE
  external_id     text NOT NULL
  provider        text NOT NULL
  entity          text NOT NULL
  type            text NOT NULL
  category        text
  start_time      double precision NOT NULL
  end_time        double precision NOT NULL
  project         text
  branch          text
  language        text
  meta            text
  created_at      timestamptz DEFAULT now()
  UNIQUE(user_id, provider, external_id)

custom_rules
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid()
  user_id         uuid REFERENCES users(id) ON DELETE CASCADE
  action          text NOT NULL   -- change | delete
  source          text NOT NULL
  operation       text NOT NULL   -- equals | contains | starts with | ends with
  source_value    text NOT NULL
  priority        int NOT NULL
  created_at      timestamptz DEFAULT now()
  modified_at     timestamptz DEFAULT now()

custom_rule_destinations
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid()
  rule_id         uuid REFERENCES custom_rules(id) ON DELETE CASCADE
  destination     text NOT NULL
  destination_value text NOT NULL

custom_rules_progress
  user_id         uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE
  status          text NOT NULL
  percent_complete int DEFAULT 0
  total           int DEFAULT 0
  changed         int DEFAULT 0
  deleted         int DEFAULT 0
  error           text
  started_at      timestamptz
  completed_at    timestamptz
  modified_at     timestamptz DEFAULT now()

data_dumps
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid()
  user_id         uuid REFERENCES users(id) ON DELETE CASCADE
  type            text NOT NULL   -- daily | heartbeats
  status          text DEFAULT 'Pending…'
  percent_complete float DEFAULT 0
  download_url    text
  is_processing   boolean DEFAULT false
  is_stuck        boolean DEFAULT false
  has_failed      boolean DEFAULT false
  expires_at      timestamptz
  created_at      timestamptz DEFAULT now()

oauth_apps
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid()
  user_id         uuid REFERENCES users(id) ON DELETE CASCADE
  name            text NOT NULL
  client_id       text UNIQUE NOT NULL
  client_secret_hash text NOT NULL
  redirect_uris   text[] NOT NULL
  scopes          text[]
  created_at      timestamptz DEFAULT now()

oauth_tokens
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid()
  app_id          uuid REFERENCES oauth_apps(id) ON DELETE CASCADE
  user_id         uuid REFERENCES users(id) ON DELETE CASCADE
  access_token_hash  text NOT NULL
  refresh_token_hash text
  scopes          text[]
  expires_at      timestamptz
  revoked_at      timestamptz
  created_at      timestamptz DEFAULT now()

-- Stats cache: precomputed aggregates
stats_cache
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid()
  user_id         uuid REFERENCES users(id) ON DELETE CASCADE
  range           text NOT NULL   -- last_7_days | last_30_days | last_6_months | last_year | all_time | YYYY | YYYY-MM
  data            jsonb NOT NULL
  is_up_to_date   boolean DEFAULT false
  percent_calculated int DEFAULT 0
  computed_at     timestamptz
  created_at      timestamptz DEFAULT now()
  UNIQUE(user_id, range)
```

---

## 5. API Endpoints

Base URL: `/api/v1/`

All responses: `Content-Type: application/json`
All errors: `{"errors": ["message"]}` or `{"error": "message"}`

### Authentication Scopes (identical to WakaTime)
```
read_summaries
read_summaries.categories
read_summaries.dependencies
read_summaries.editors
read_summaries.languages
read_summaries.machines
read_summaries.operating_systems
read_summaries.projects
read_stats
read_stats.best_day
read_stats.categories
read_stats.dependencies
read_stats.editors
read_stats.languages
read_stats.machines
read_stats.operating_systems
read_stats.projects
read_goals
read_private_leaderboards
write_private_leaderboards
read_heartbeats
write_heartbeats
email
```

### Endpoint Index

#### Public (no auth)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/meta` | Server IP info |
| GET | `/api/v1/docs` | OpenAPI document |
| GET | `/api/v1/leaders` | Public leaderboard |
| GET | `/api/v1/editors` | IDE plugin list + versions |
| GET | `/api/v1/program_languages` | All supported languages |

#### Auth and API Keys
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/auth/me` | Current authenticated user |
| GET | `/api/v1/api_keys` | List API keys |
| POST | `/api/v1/api_keys` | Create API key |
| DELETE | `/api/v1/api_keys/:id` | Revoke API key |

#### Users
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users/current` | Current user profile |
| PUT | `/api/v1/users/current` | Update current user |
| DELETE | `/api/v1/users/current` | Delete current user and owned data |
| GET | `/api/v1/users/:user` | Public user profile |

#### Heartbeats
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users/current/heartbeats` | Heartbeats for a day |
| POST | `/api/v1/users/current/heartbeats` | Send single heartbeat |
| POST | `/api/v1/users/current/heartbeats.bulk` | Send up to 25 heartbeats |
| DELETE | `/api/v1/users/current/heartbeats.bulk` | Delete heartbeats by date+ids |
| POST | `/api/v1/users/current/usage_events.bulk` | Ingest up to 5000 AI usage events (idempotent) |
| GET | `/api/v1/users/current/usage_events` | Export AI usage events for a time range |
| GET | `/api/v1/users/current/usage_events/summary` | Aggregated AI cost/token summary |
| GET | `/api/v1/users/current/usage_events/blocks` | 5-hour usage blocks + burn rate |
| GET | `/api/v1/users/current/custom_pricing` | List custom AI pricing overrides |
| PUT | `/api/v1/users/current/custom_pricing` | Upsert a custom AI pricing override |
| DELETE | `/api/v1/users/current/custom_pricing/:model` | Delete a custom AI pricing override |
| GET | `/api/v1/users/current/pricing/sources` | List AI price sources and freshness |
| GET | `/api/v1/users/current/pricing/models` | List resolved per-model AI prices |
| GET | `/api/v1/users/current/billing_prefs` | List per-agent billing-mode overrides |
| PUT | `/api/v1/users/current/billing_prefs` | Upsert a per-agent billing-mode override |
| DELETE | `/api/v1/users/current/billing_prefs/:agent` | Delete a per-agent billing-mode override |
| POST | `/api/v1/users/current/file_experts` | WakaTime-compatible file experts |

#### Durations
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users/current/durations` | Durations for a day |

#### Summaries
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users/current/summaries` | Daily summaries for date range |
| GET | `/api/v1/users/:user/summaries` | Public summaries |

#### Stats
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users/current/stats` | Coding stats (all time ranges) |
| GET | `/api/v1/users/current/stats/last_7_days` | Cached last-7-days stats |
| GET | `/api/v1/users/current/stats/:range` | Stats for specific range |
| GET | `/api/v1/users/:user/stats` | Public stats |
| GET | `/api/v1/users/:user/stats/:range` | Public stats for range |

#### All Time
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users/current/all_time_since_today` | Total time since signup |

#### Insights
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users/current/insights/:insight_type/:range` | insight_type: stats, projects, languages, editors, machines, operating_systems, categories, dependencies, days, hours, weekdays, best_day, daily_average, daily_average_trend, ai_agents, ai_days |

#### Projects
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users/current/projects` | List projects |
| GET | `/api/v1/users/current/projects/:project` | Project detail with stats |
| GET | `/api/v1/users/current/projects/:project/commits` | Commits with time |
| GET | `/api/v1/users/current/projects/:project/commits/:hash` | Single commit |

#### Status Bar
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users/current/status_bar/today` | Quick today stats for status bars |
| GET | `/api/v1/users/current/statusbar/today` | WakaTime-compatible status bar payload |

#### Goals
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users/current/goals` | List goals |
| POST | `/api/v1/users/current/goals` | Create goal |
| GET | `/api/v1/users/current/goals/:goal` | Single goal |
| PUT | `/api/v1/users/current/goals/:goal` | Update goal |
| DELETE | `/api/v1/users/current/goals/:goal` | Delete goal |

#### Leaderboards
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users/current/leaderboards` | List private leaderboards |
| POST | `/api/v1/users/current/leaderboards` | Create leaderboard |
| GET | `/api/v1/users/current/leaderboards/:board` | Leaderboard with rankings |
| PUT | `/api/v1/users/current/leaderboards/:board` | Update leaderboard |
| DELETE | `/api/v1/users/current/leaderboards/:board` | Delete leaderboard |
| POST | `/api/v1/users/current/leaderboards/:board/members` | Add leaderboard member |
| DELETE | `/api/v1/users/current/leaderboards/:board/members/:user` | Remove leaderboard member |
| GET | `/api/v1/users/current/events` | Server-sent job progress invalidation events |

#### Machine Names
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users/current/machine_names` | List machines |
| GET | `/api/v1/users/current/user_agents` | List plugins/editors that sent data |

#### External Durations
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users/current/external_durations` | List external durations |
| POST | `/api/v1/users/current/external_durations` | Create external duration |
| POST | `/api/v1/users/current/external_durations.bulk` | Bulk create (up to 1000) |
| DELETE | `/api/v1/users/current/external_durations.bulk` | Bulk delete |

#### Custom Rules
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users/current/custom_rules` | List custom rules |
| PUT | `/api/v1/users/current/custom_rules` | Replace/update rules |
| DELETE | `/api/v1/users/current/custom_rules/:rule_id` | Delete single rule |
| GET | `/api/v1/users/current/custom_rules_progress` | Background job progress |
| DELETE | `/api/v1/users/current/custom_rules_progress` | Abort background job |

#### Data Dumps
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users/current/data_dumps` | List exports |
| POST | `/api/v1/users/current/data_dumps` | Start new export |
| GET | `/api/v1/users/current/data_dumps/:dump/download` | Download completed export |

#### Share Tokens
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users/current/share_tokens` | List share tokens |
| POST | `/api/v1/users/current/share_tokens` | Create share token |
| DELETE | `/api/v1/users/current/share_tokens/:id` | Delete share token |

#### Imports
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/users/current/imports/wakatime` | Import WakaTime JSON or gzip dump |

#### AI Costs
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users/current/ai_costs` | List AI agent cost settings |
| PUT | `/api/v1/users/current/ai_costs` | Replace AI agent cost settings |

#### OAuth
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/oauth/apps` | List OAuth apps |
| POST | `/api/v1/oauth/apps` | Create OAuth app |
| DELETE | `/api/v1/oauth/apps/:id` | Delete OAuth app |
| GET | `/oauth/authorize` | Show consent screen |
| POST | `/oauth/authorize` | Approve or deny OAuth authorization |
| POST | `/oauth/token` | Exchange code/refresh for token |
| POST | `/oauth/revoke` | Revoke token |

#### Embed (shareable JSON, no auth required, JSONP supported)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/users/:user/share/:token/stats` | Embedded stats |
| GET | `/api/v1/users/:user/share/:token/summaries` | Embedded summaries |
| GET | `/api/v1/share/:token/stats` | Token-only embedded stats |
| GET | `/api/v1/share/:token/summaries` | Token-only embedded summaries |

---

## 6. Heartbeat Processing Pipeline

This is the core of the system. Must be correct and fast.

### Ingestion
1. POST `/heartbeats` or `/heartbeats.bulk`
2. Parse `User-Agent` header to extract editor name + OS
3. Apply custom rules (regex matching on entity, project, language, branch)
4. Validate: `entity` required, `time` required and within 1 year
5. Deduplicate: skip heartbeats with same (user_id, entity, time)
6. Write to PostgreSQL heartbeats table
7. Enqueue async job to invalidate relevant stats cache entries
8. Return 202 Accepted immediately

### Duration Computation
Durations are computed on-the-fly (not pre-stored) from heartbeats:
1. Fetch heartbeats for the requested day, ordered by time
2. Group by `slice_by` key (default: project)
3. Within each group, merge heartbeats within `timeout` minutes of each other into a single duration
4. Each duration has: start time, duration seconds, and the slice key value

### Stats Aggregation
Stats are expensive — compute async and cache:
1. Background worker computes stats for each user × range combination
2. Cache in `stats_cache` table as JSONB
3. Serve from cache; return `is_up_to_date: false` + 202 if cache is stale
4. Trigger cache refresh on: new heartbeat received, settings change, custom rules update
5. Redis TTL cache in front of DB for status bar endpoint (recompute max every 2 min)

### AI Metrics
Track AI coding activity from heartbeat fields:
- `ai_line_changes` — lines modified by AI agents
- `human_line_changes` — lines modified by human typing
- `ai_session` — session ID to group AI interactions
- `ai_input_tokens`, `ai_output_tokens` — token usage
- `ai_prompt_length` — chars typed to AI
- `ai_model`, `ai_agent`, `ai_agent_version`, `ai_agent_complexity` — model/agent identity for attribution and pricing
- Aggregate into: AI%, human%, cost estimates (use configurable cost-per-token table per model or agent; `ai_model` wins over `ai_agent`, which wins over `ai_subscription_plan`)

---

## 7. Frontend — Dashboard Pages

### Design System
- **Background:** `#0d0d0d` (near black)
- **Surface:** `#1a1a1a` (cards), `#242424` (sidebar)
- **Accent:** `#00b4d8` (WakaTime blue-ish; can be user-configurable)
- **Text:** `#e8e8e8` primary, `#888` muted
- **Language colors:** match WakaTime's exact palette (stored in `program_languages` table)
- **Border radius:** 8px cards, consistent
- **Font:** Inter or Geist

### Pages

#### `/dashboard` — Activity Overview
Clone of WakaTime's dashboard:
- Hero: total time this week (big number), current day, daily average, most active day
- **AI Coding panel:** AI% ring chart, AI lines, human lines, tokens, estimated cost, human review %, follow-up edits
- **Projects chart:** stacked area chart (7 days), with tooltip showing per-project breakdown on hover
- **Categories bar chart:** Coding, Debugging, Testing, etc.
- **Timeline charts:** hours per day (projects view + languages view), 24-hour x-axis
- **Agents donut:** AI agent breakdown (Codex, Claude, Cursor, etc.) by lines
- **Editors donut:** editor breakdown
- **Languages donut:** language breakdown
- **Operating Systems donut:** OS breakdown  
- **Machines donut:** machine breakdown
- **AI vs Human by Day:** bar chart, additions/deletions per day
- **Weekdays heatmap:** coding time by day of week
- **Projects grid:** cards per project with AI changes, human changes, prompts, sessions, tokens, spend

#### `/projects/:name` — Project Detail
- Total time, time range selector
- Language breakdown
- Branch activity
- Commit timeline (if GitHub repo linked)
- AI vs Human split

#### `/leaderboards` — Public Leaderboard
- Ranked list with GitHub avatars, language filter, country filter
- Current user's rank highlighted

#### `/goals` — Goals
- Goal cards showing progress bars (actual vs target per day/week)
- Create/edit goal modal

#### `/insights` — Insights
- Rich breakdowns: best day, weekday patterns, daily average trend, heatmaps
- The Insights UI includes a range activity heatmap backed by the selected stats range.

#### `/reports` — Custom Date Range Reports
- Date range picker, export to CSV/JSON

#### `/settings` — Settings
- API key management (create named keys, revoke)
- Timeout preference
- Writes-only toggle
- Timezone
- Plugin setup instructions (shows `api_url` and `api_key` for `~/.wakatime.cfg`)
- GitHub account info
- Data export (trigger dump, download)
- Danger zone: delete account + all data

#### `/share/:token` — Embedded/Public Stats
- Read-only view of a user's stats via share token
- JSONP endpoint for embedding
- Token-only frontend links resolve through `/api/v1/share/:token/stats`; user-scoped `/api/v1/users/:user/share/:token/*` endpoints remain available for embed clients that include a user reference.

---

## 8. AI Dashboard (Differentiation)

This is what sets Stint apart. WakaTime shows AI metrics but doesn't go deep enough.

### AI Activity Panel (expanded)
- **AI Coding %** — ring chart, big and prominent
- **Agent breakdown table:** per-agent lines, sessions, avg prompt length, estimated token cost
- **Cost tracker:** daily/weekly/monthly estimated spend per agent, with configurable cost-per-token rates
- **Prompt analytics:** average prompt length, median prompts per session, total prompts
- **Human review rate:** what % of AI lines you actually reviewed/modified
- **AI vs Human trend:** 30-day rolling chart

### Agent Cost Configuration
Settings page allows user to set cost-per-million-tokens for each agent:
```
Codex:  input $3/M, output $12/M  (defaults)
Claude: input $3/M, output $15/M
Cursor: input $0/M (flat sub)
etc.
```

### AI Heatmap
Calendar heatmap (GitHub contribution-style) but colored by AI% per day, not just total time. Days that are 100% AI glow differently.

---

## 9. Plugin Setup Flow

On first login, show an onboarding modal:
1. "Install wakatime-cli" — link to wakatime.com/help
2. Copy-paste config block:
```ini
[settings]
api_url = https://YOUR_DOMAIN/api/v1
api_key = waka_00000000-0000-4000-8000-000000000000
```
3. "Open your editor and start coding — activity appears within 2 minutes"

---

## 10. Background Jobs

Use Asynq with Redis. Define these job types:

| Job | Trigger | What it does |
|-----|---------|--------------|
| `stats:recompute` | New heartbeat, settings change | Recomputes stats_cache for affected ranges |
| `goals:evaluate` | Daily cron (midnight user TZ) | Marks goal periods pass/fail |
| `leaderboard:update` | Hourly cron | Recalculates public leaderboard |
| `data_dump:process` | On POST /data_dumps | Writes a stable JSON export snapshot, marks export metadata complete, and exposes an authenticated download URL |
| `wakatime_import:process` | On POST /imports/wakatime | Inserts uploaded WakaTime dump heartbeats and skips duplicates |
| `custom_rules:apply` | On PUT /custom_rules | Retroactively applies rules to past heartbeats |
| `heartbeats:purge` | Weekly cron | Deletes heartbeats older than user-configured retention |

---

## 11. Rate Limiting

- Heartbeat ingestion: 1000/min per user (generous for CLI usage)
- Read endpoints: 60/min per API key
- OAuth token creation: 10/user/hour
- Use Redis sliding window counters
- Return `Retry-After` header on 429

---

## 12. Configuration (Environment Variables)

```bash
# Server
PORT=8080
BASE_URL=https://api.example.com
WEB_BASE_URL=https://app.example.com

# Database
DATABASE_URL=postgres://user:pass@localhost:5432/stint

# Redis
REDIS_URL=redis://localhost:6379

# GitHub OAuth
GITHUB_CLIENT_ID=xxx
GITHUB_CLIENT_SECRET=xxx

# Session
SESSION_SECRET=64-random-bytes-hex

# Storage (for data dumps)
STORAGE_TYPE=local          # local for v1 self-hosted dumps
STORAGE_PATH=./data/dumps   # if local
S3_BUCKET=xxx               # reserved for future s3 support
S3_REGION=us-east-1
AWS_ACCESS_KEY_ID=xxx
AWS_SECRET_ACCESS_KEY=xxx

# Email (for data dump notifications)
SMTP_HOST=
SMTP_PORT=587
SMTP_USER=
SMTP_PASS=
EMAIL_FROM=noreply@example.com

# Feature flags
ENABLE_PUBLIC_LEADERBOARD=true
ENABLE_REGISTRATION=true     # set false to lock to invited users
MAX_USERS=0                  # 0 = unlimited
```

---

## 13. Repo Structure

```
stint/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── api/
│   │   ├── middleware/
│   │   │   ├── auth.go
│   │   │   ├── ratelimit.go
│   │   │   └── useragent.go
│   │   ├── handlers/
│   │   │   ├── heartbeats.go
│   │   │   ├── durations.go
│   │   │   ├── summaries.go
│   │   │   ├── stats.go
│   │   │   ├── insights.go
│   │   │   ├── goals.go
│   │   │   ├── leaderboards.go
│   │   │   ├── projects.go
│   │   │   ├── users.go
│   │   │   ├── oauth.go
│   │   │   └── ...
│   │   └── router.go
│   ├── db/
│   │   ├── migrations/
│   │   └── queries/       # sqlc input
│   ├── models/            # sqlc generated
│   ├── services/
│   │   ├── heartbeats.go  # ingestion, dedup, custom rules
│   │   ├── durations.go   # compute durations from heartbeats
│   │   ├── stats.go       # aggregate stats
│   │   ├── insights.go
│   │   └── ai.go          # AI metrics computation
│   ├── workers/
│   │   ├── stats_worker.go
│   │   ├── goals_worker.go
│   │   └── dump_worker.go
│   ├── auth/
│   │   ├── github.go
│   │   └── apikey.go
│   └── config/
│       └── config.go
├── web/                   # Next.js frontend
│   ├── app/
│   │   ├── dashboard/
│   │   ├── projects/
│   │   ├── insights/
│   │   ├── goals/
│   │   ├── leaderboards/
│   │   ├── settings/
│   │   └── share/
│   ├── components/
│   │   ├── charts/
│   │   │   ├── ActivityOverview.tsx
│   │   │   ├── AICodingPanel.tsx
│   │   │   ├── ProjectsChart.tsx
│   │   │   ├── DonutChart.tsx
│   │   │   ├── TimelineChart.tsx
│   │   │   └── AIHeatmap.tsx
│   │   ├── ui/            # shadcn components
│   │   └── layout/
│   └── lib/
│       ├── api.ts         # typed API client
│       └── hooks/
├── docker-compose.yml
├── Dockerfile
└── README.md
```

---

## 14. WakaTime Plugin Compatibility

### What "compatible" means
Any plugin sending to `https://api.wakatime.com/api/v1` can switch by setting:
```ini
api_url = https://YOUR_DOMAIN/api/v1
```

### User-Agent Parsing
WakaTime plugins send a User-Agent like:
```
wakatime/v1.102.1 (darwin-arm64) go1.22.0 vscode/1.89.0 vscode-wakatime/24.3.0
```
Parse this to extract: plugin name, plugin version, editor name, OS, architecture.

### Heartbeat Fields to Accept
Accept all current WakaTime heartbeat fields including AI fields (`ai_line_changes`, `ai_session`, `ai_input_tokens`, etc.) — these are already sent by wakatime-cli 1.90+. Stint also accepts model attribution fields (`ai_model`, `ai_model_name`, or `model`) so AI pricing can be calculated against the actual model when clients provide it.

### Wakatime-cli Config
Document exactly how to configure `~/.wakatime.cfg` in the setup flow:
```ini
[settings]
api_url = https://YOUR_DOMAIN/api/v1
api_key = waka_00000000-0000-4000-8000-000000000000
hide_file_names = false
timeout = 15
```

For Codex or other WakaTime CLI clients using multi-destination fanout:
```ini
[api_urls]
.* = https://YOUR_DOMAIN/api/v1|waka_00000000-0000-4000-8000-000000000000
```

---

## 15. Data Export / Import

### Export (Data Dumps)
Support both types WakaTime supports:
- `daily` — JSON array of daily summaries
- `heartbeats` — raw heartbeat JSON

Settings exposes the export flow: choose the dump type, trigger generation, poll queued progress, and download completed dump files.

### Import from WakaTime
On first login (or Settings > Import), allow importing a WakaTime data dump:
1. User downloads their dump from wakatime.com/settings
2. Upload the JSON file via Settings > Import
3. Background job processes and inserts into heartbeats/summaries tables
4. De-duplicate against existing data

---

## 16. Build & Deployment

### Development
```bash
# Start dependencies
docker compose up -d postgres redis

# Run backend
go run ./cmd/server

# Run frontend
cd web && npm run dev
```

### Production Docker Compose
```yaml
services:
  api:
    build: .
    environment:
      - DATABASE_URL
      - REDIS_URL
      - GITHUB_CLIENT_ID
      - GITHUB_CLIENT_SECRET
      - SESSION_SECRET
      - BASE_URL
      - WEB_BASE_URL
      - STORAGE_PATH=/data/dumps
    volumes: [dumpdata:/data/dumps]
    depends_on: [postgres, redis]
    ports: ["8080:8080"]

  worker:
    build: .
    command: ["worker"]
    environment:
      - DATABASE_URL
      - REDIS_URL
      - SESSION_SECRET
      - BASE_URL
      - WEB_BASE_URL
      - STORAGE_PATH=/data/dumps
    volumes: [dumpdata:/data/dumps]
    depends_on: [postgres, redis]

  web:
    build: ./web
    environment:
      - API_BASE_URL=http://api:8080
      - NEXT_PUBLIC_API_BASE_URL=
    depends_on: [api]
    ports: ["3001:3000"]

  postgres:
    image: postgres:15-alpine
    volumes: [pgdata:/var/lib/postgresql/data]
    environment:
      POSTGRES_DB: stint
      POSTGRES_USER: stint
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}

  redis:
    image: redis:7-alpine
    volumes: [redisdata:/data]

volumes:
  pgdata:
  redisdata:
  dumpdata:
```

### Deployment Notes
Deploy the API and worker containers on your preferred platform. Provide managed Postgres and Redis connection strings through environment variables, and run the same image with the `worker` command for background jobs.

---

## 17. What to Skip (Out of Scope)

These WakaTime features are explicitly not implemented:
- **Org dashboards / teams** — no orgs, no dashboards, no dashboard members
- **Org custom rules** — personal custom rules only
- **Twitter/tweeting goals** — removed
- **Billing / invoices / subscriptions** — it's free
- **Program language admin** — seed from static file, no admin UI
- **Editor plugin registry admin** — seed from static file
- **Stats aggregated** (cross-user aggregate endpoint) — skip
- **Commit tracking** — complex, skip for v1; add in v2 with GitHub webhooks
- **Multi-provider SSO** — GitHub only

---

## 18. Implementation Order (for Codex)

Phase 1 — Core pipeline (get plugins sending data):
1. Database schema + migrations
2. GitHub OAuth login
3. API key auth middleware
4. POST /heartbeats + /heartbeats.bulk
5. GET /users/current
6. Basic stats computation (last_7_days)
7. GET /users/current/stats/last_7_days
8. GET /users/current/summaries
9. GET /users/current/durations
10. Frontend: login page, dashboard skeleton, activity overview chart

Phase 2 — Full dashboard:
11. All stats ranges (last_30_days, last_6_months, last_year, all_time)
12. Insights endpoint
13. Projects list + project detail
14. Goals CRUD
15. Machine names
16. Status bar endpoint
17. Frontend: all dashboard panels, project pages, settings

Phase 3 — AI layer:
18. AI metrics aggregation in stats
19. AI Dashboard panel (cost tracking, agent breakdown)
20. AI Heatmap
21. Cost-per-token configuration in settings

Phase 4 — Social + export:
22. Public leaderboard
23. Private leaderboards CRUD
24. Data dumps (export + import)
25. Shareable embed tokens
26. External durations
27. Custom rules engine

Phase 5 — Polish:
28. OAuth 2.0 authorization server (for third-party apps)
29. Rate limiting
30. Container-platform-neutral deployment docs
31. Comprehensive README + plugin setup guide
32. OpenAPI spec (auto-generated, serves at `/api/v1/docs`)

---

## 19. Testing Strategy

- **Unit tests:** services layer (stats computation, duration merging, custom rules)
- **Integration tests:** API handlers against real Postgres (use testcontainers-go)
- **Plugin compatibility test:** run wakatime-cli against the local server and verify heartbeats arrive
- **Golden file tests:** snapshot the JSON shape of each API response and compare against WakaTime's docs
- Aim for 80%+ coverage on services and handlers

---

## 20. Non-Goals / Explicit Decisions

| Decision | Rationale |
|----------|-----------|
| GitHub-only SSO | Simplicity; devs all have GitHub; no password reset flows |
| Go backend | Fast, single binary, great for self-hosting; matches wakapi's proven approach |
| Next.js frontend | Best DX for a React dashboard; easy to self-host with `next start` |
| PostgreSQL only | No SQLite option — BRIN indexes and JSONB are required |
| No mobile app | Dashboard is responsive; mobile app is out of scope |
| Orgs dropped | The complexity-to-value ratio is poor for solo devs; orgs = $21/mo reason to use WakaTime |
| No per-user billing | This is the whole point |
