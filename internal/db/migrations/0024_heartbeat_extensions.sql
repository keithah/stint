ALTER TABLE heartbeats ADD COLUMN IF NOT EXISTS ai_provider text;
ALTER TABLE heartbeats ADD COLUMN IF NOT EXISTS metadata jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE heartbeats ADD COLUMN IF NOT EXISTS raw_payload jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS heartbeats_user_ai_provider_time_idx ON heartbeats(user_id, ai_provider, time);
