ALTER TABLE wakatime_imports
  ADD COLUMN IF NOT EXISTS total_count integer NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS wakatime_import_chunks (
  import_id uuid NOT NULL REFERENCES wakatime_imports(id) ON DELETE CASCADE,
  chunk_index integer NOT NULL,
  heartbeats jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (import_id, chunk_index)
);
