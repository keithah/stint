-- Cached upstream price snapshots, refreshed weekly by the pricing job. Global
-- (not per-user): the LiteLLM table and OpenRouter catalog are shared. The raw
-- payload is stored so every process can rebuild its pricing engine from the
-- latest fetch without a redeploy; the embedded bundle remains the fallback when
-- a source has never been fetched. status/error/fetched_at drive the settings
-- "last checked" display.
CREATE TABLE IF NOT EXISTS pricing_snapshots (
  source       text PRIMARY KEY,
  url          text NOT NULL,
  payload      text NOT NULL DEFAULT '',
  model_count  int NOT NULL DEFAULT 0,
  status       text NOT NULL DEFAULT 'ok',
  error        text NOT NULL DEFAULT '',
  fetched_at   timestamptz NOT NULL DEFAULT now()
);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'pricing_snapshots_source_check'
  ) THEN
    ALTER TABLE pricing_snapshots
      ADD CONSTRAINT pricing_snapshots_source_check CHECK (source IN ('litellm', 'openrouter'));
  END IF;
END$$;
