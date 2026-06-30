CREATE TABLE IF NOT EXISTS wakatime_imports (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  heartbeats jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  processed_at timestamptz
);

CREATE INDEX IF NOT EXISTS wakatime_imports_user_created_idx ON wakatime_imports(user_id, created_at DESC);
