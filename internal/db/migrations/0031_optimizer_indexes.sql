CREATE INDEX IF NOT EXISTS users_github_username_lower_created_idx
  ON users (lower(github_username), created_at);

CREATE INDEX IF NOT EXISTS custom_rule_destinations_rule_id_id_idx
  ON custom_rule_destinations (rule_id, id);

CREATE INDEX IF NOT EXISTS projects_user_last_heartbeat_name_idx
  ON projects (user_id, last_heartbeat_at DESC NULLS LAST, name);

CREATE INDEX IF NOT EXISTS machine_names_user_last_seen_name_idx
  ON machine_names (user_id, last_seen_at DESC NULLS LAST, name);

CREATE INDEX IF NOT EXISTS api_keys_user_created_idx
  ON api_keys (user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS oauth_apps_user_created_idx
  ON oauth_apps (user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS leaderboard_members_user_id_idx
  ON leaderboard_members (user_id, leaderboard_id);
