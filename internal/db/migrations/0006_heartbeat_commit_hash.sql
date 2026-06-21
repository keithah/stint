ALTER TABLE heartbeats ADD COLUMN IF NOT EXISTS commit_hash text;

CREATE INDEX IF NOT EXISTS heartbeats_user_project_commit_time_idx ON heartbeats(user_id, project, commit_hash, time);
