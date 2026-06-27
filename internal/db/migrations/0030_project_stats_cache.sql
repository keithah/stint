CREATE TABLE IF NOT EXISTS project_stats_cache (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  project text NOT NULL,
  range text NOT NULL,
  data jsonb NOT NULL,
  is_up_to_date boolean NOT NULL DEFAULT true,
  percent_calculated integer NOT NULL DEFAULT 100,
  computed_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, project, range)
);

CREATE INDEX IF NOT EXISTS project_stats_cache_user_range_idx ON project_stats_cache(user_id, range);
