CREATE INDEX IF NOT EXISTS external_durations_user_project_time_idx
  ON external_durations(user_id, project, start_time, end_time);
