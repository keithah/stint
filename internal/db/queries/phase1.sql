-- name: GetUser :one
SELECT id, github_id, github_username, email, full_name, avatar_url, timezone, timeout_minutes, writes_only, is_hireable, has_public_profile, country, heartbeat_retention_days,
  public_username, public_display_name, public_github_link_enabled, public_show_total_time, public_show_projects, public_project_visibility,
  public_show_languages, public_show_editors, public_show_machines, public_show_operating_systems, public_show_categories, public_show_ai,
  public_show_summaries, public_profile
FROM users
WHERE id = $1;

-- name: ListHeartbeatsForRange :many
SELECT id, user_id, entity, type, category, time, project, branch, language, machine_name_id,
  machine_name, plugin, plugin_version, editor, editor_version, operating_system, architecture,
  dependencies, lines, line_number, cursor_pos, is_write, ai_line_changes, human_line_changes,
  ai_session, ai_input_tokens, ai_output_tokens, ai_prompt_length, ai_subscription_plan, created_at,
  commit_hash
FROM heartbeats
WHERE user_id = $1 AND time >= $2 AND time < $3
ORDER BY time ASC;
