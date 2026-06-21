ALTER TABLE heartbeats ADD COLUMN IF NOT EXISTS ai_model text;
ALTER TABLE heartbeats ADD COLUMN IF NOT EXISTS ai_agent text;
ALTER TABLE heartbeats ADD COLUMN IF NOT EXISTS ai_agent_version text;
ALTER TABLE heartbeats ADD COLUMN IF NOT EXISTS ai_agent_complexity text;
