ALTER TABLE heartbeats ADD COLUMN IF NOT EXISTS plugin text;
ALTER TABLE heartbeats ADD COLUMN IF NOT EXISTS plugin_version text;
ALTER TABLE heartbeats ADD COLUMN IF NOT EXISTS editor_version text;
ALTER TABLE heartbeats ADD COLUMN IF NOT EXISTS architecture text;
