CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  github_id bigint UNIQUE NOT NULL,
  github_username text NOT NULL,
  email text,
  full_name text,
  avatar_url text,
  country text,
  timezone text NOT NULL DEFAULT 'UTC',
  timeout_minutes int NOT NULL DEFAULT 15,
  writes_only boolean NOT NULL DEFAULT false,
  is_hireable boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  modified_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS api_keys (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name text NOT NULL,
  key_hash text NOT NULL,
  key_fingerprint text NOT NULL,
  scopes text[] NOT NULL DEFAULT '{}',
  last_used_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS api_keys_user_id_idx ON api_keys(user_id);
CREATE INDEX IF NOT EXISTS api_keys_key_fingerprint_idx ON api_keys(key_fingerprint);

CREATE TABLE IF NOT EXISTS sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash text NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions(user_id);

CREATE TABLE IF NOT EXISTS machine_names (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name text NOT NULL,
  value text,
  ip inet,
  timezone text,
  last_seen_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(user_id, name)
);

CREATE TABLE IF NOT EXISTS projects (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name text NOT NULL,
  color text,
  has_public_url boolean NOT NULL DEFAULT false,
  badge text,
  first_heartbeat_at timestamptz,
  last_heartbeat_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(user_id, name)
);

CREATE TABLE IF NOT EXISTS heartbeats (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  entity text NOT NULL,
  type text NOT NULL,
  category text,
  time double precision NOT NULL,
  project text,
  branch text,
  language text,
  machine_name_id uuid REFERENCES machine_names(id),
  machine_name text,
  editor text,
  operating_system text,
  dependencies text,
  lines int,
  line_number int,
  cursor_pos int,
  is_write boolean NOT NULL DEFAULT false,
  ai_line_changes int,
  human_line_changes int,
  ai_session text,
  ai_input_tokens int,
  ai_output_tokens int,
  ai_prompt_length int,
  ai_subscription_plan text,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(user_id, entity, time)
);

CREATE INDEX IF NOT EXISTS heartbeats_user_time_idx ON heartbeats(user_id, time);
CREATE INDEX IF NOT EXISTS heartbeats_user_project_time_idx ON heartbeats(user_id, project, time);
CREATE INDEX IF NOT EXISTS heartbeats_user_language_time_idx ON heartbeats(user_id, language, time);
CREATE INDEX IF NOT EXISTS heartbeats_time_brin_idx ON heartbeats USING brin(time);

CREATE TABLE IF NOT EXISTS stats_cache (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  range text NOT NULL,
  data jsonb NOT NULL,
  is_up_to_date boolean NOT NULL DEFAULT false,
  percent_calculated int NOT NULL DEFAULT 0,
  computed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(user_id, range)
);

CREATE INDEX IF NOT EXISTS stats_cache_user_range_idx ON stats_cache(user_id, range);

CREATE TABLE IF NOT EXISTS goals (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title text NOT NULL,
  custom_title text,
  delta text NOT NULL DEFAULT 'day',
  seconds int NOT NULL,
  languages text[] NOT NULL DEFAULT '{}',
  editors text[] NOT NULL DEFAULT '{}',
  projects text[] NOT NULL DEFAULT '{}',
  ignore_days text[] NOT NULL DEFAULT '{}',
  ignore_zero_days boolean NOT NULL DEFAULT false,
  improve_by_percent double precision,
  is_enabled boolean NOT NULL DEFAULT true,
  is_inverse boolean NOT NULL DEFAULT false,
  is_snoozed boolean NOT NULL DEFAULT false,
  snooze_until timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  modified_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS goals_user_id_idx ON goals(user_id);

CREATE TABLE IF NOT EXISTS leaderboards (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name text NOT NULL,
  time_range text NOT NULL DEFAULT 'last_7_days',
  created_at timestamptz NOT NULL DEFAULT now(),
  modified_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS leaderboard_members (
  leaderboard_id uuid NOT NULL REFERENCES leaderboards(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role text NOT NULL DEFAULT 'member',
  PRIMARY KEY (leaderboard_id, user_id)
);

CREATE TABLE IF NOT EXISTS external_durations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  external_id text NOT NULL,
  provider text NOT NULL,
  entity text NOT NULL,
  type text NOT NULL,
  category text,
  start_time double precision NOT NULL,
  end_time double precision NOT NULL,
  project text,
  branch text,
  language text,
  meta text,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(user_id, provider, external_id)
);

CREATE INDEX IF NOT EXISTS external_durations_user_time_idx ON external_durations(user_id, start_time, end_time);

CREATE TABLE IF NOT EXISTS data_dumps (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  type text NOT NULL,
  status text NOT NULL DEFAULT 'Pending',
  percent_complete double precision NOT NULL DEFAULT 0,
  download_url text,
  is_processing boolean NOT NULL DEFAULT true,
  is_stuck boolean NOT NULL DEFAULT false,
  has_failed boolean NOT NULL DEFAULT false,
  expires_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS data_dumps_user_id_idx ON data_dumps(user_id);

CREATE TABLE IF NOT EXISTS custom_rules (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  action text NOT NULL,
  source text NOT NULL,
  operation text NOT NULL,
  source_value text NOT NULL,
  priority int NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  modified_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS custom_rules_user_id_idx ON custom_rules(user_id, priority);

CREATE TABLE IF NOT EXISTS custom_rule_destinations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  rule_id uuid NOT NULL REFERENCES custom_rules(id) ON DELETE CASCADE,
  destination text NOT NULL,
  destination_value text NOT NULL
);

CREATE TABLE IF NOT EXISTS oauth_apps (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name text NOT NULL,
  client_id text NOT NULL UNIQUE,
  client_secret_hash text NOT NULL,
  client_secret_fingerprint text NOT NULL,
  redirect_uris text[] NOT NULL DEFAULT '{}',
  scopes text[] NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  modified_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS oauth_apps_user_id_idx ON oauth_apps(user_id);
CREATE INDEX IF NOT EXISTS oauth_apps_client_id_idx ON oauth_apps(client_id);

CREATE TABLE IF NOT EXISTS oauth_authorization_codes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id uuid NOT NULL REFERENCES oauth_apps(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  code_hash text NOT NULL,
  code_fingerprint text NOT NULL UNIQUE,
  redirect_uri text NOT NULL,
  scopes text[] NOT NULL DEFAULT '{}',
  expires_at timestamptz NOT NULL,
  used_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS oauth_authorization_codes_app_id_idx ON oauth_authorization_codes(app_id);
CREATE INDEX IF NOT EXISTS oauth_authorization_codes_user_id_idx ON oauth_authorization_codes(user_id);

CREATE TABLE IF NOT EXISTS oauth_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  app_id uuid NOT NULL REFERENCES oauth_apps(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  access_token_hash text NOT NULL,
  access_token_fingerprint text NOT NULL,
  refresh_token_hash text NOT NULL,
  refresh_token_fingerprint text NOT NULL,
  scopes text[] NOT NULL DEFAULT '{}',
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  last_used_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS oauth_tokens_user_id_idx ON oauth_tokens(user_id);
CREATE INDEX IF NOT EXISTS oauth_tokens_access_token_fingerprint_idx ON oauth_tokens(access_token_fingerprint);
CREATE INDEX IF NOT EXISTS oauth_tokens_refresh_token_fingerprint_idx ON oauth_tokens(refresh_token_fingerprint);

CREATE TABLE IF NOT EXISTS share_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name text NOT NULL,
  token_hash text NOT NULL,
  token_fingerprint text NOT NULL,
  last_used_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(user_id, token_fingerprint)
);

CREATE INDEX IF NOT EXISTS share_tokens_user_id_idx ON share_tokens(user_id);
CREATE INDEX IF NOT EXISTS share_tokens_token_fingerprint_idx ON share_tokens(token_fingerprint);

CREATE TABLE IF NOT EXISTS ai_cost_settings (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  agent text NOT NULL,
  input_cost_per_million_cents int NOT NULL DEFAULT 3,
  output_cost_per_million_cents int NOT NULL DEFAULT 12,
  modified_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, agent)
);
