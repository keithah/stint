ALTER TABLE users ADD COLUMN IF NOT EXISTS heartbeat_retention_days int NOT NULL DEFAULT 0;
