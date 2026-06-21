CREATE TABLE IF NOT EXISTS goal_evaluations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  goal_id uuid NOT NULL REFERENCES goals(id) ON DELETE CASCADE,
  period_start timestamptz NOT NULL,
  period_end timestamptz NOT NULL,
  actual_seconds int NOT NULL,
  target_seconds int NOT NULL,
  percent int NOT NULL,
  is_complete boolean NOT NULL,
  evaluated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(goal_id, period_start, period_end)
);

CREATE INDEX IF NOT EXISTS goal_evaluations_user_period_idx ON goal_evaluations(user_id, period_start DESC);
