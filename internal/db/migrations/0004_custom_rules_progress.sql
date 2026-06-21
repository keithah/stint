CREATE TABLE IF NOT EXISTS custom_rules_progress (
  user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  status text NOT NULL,
  percent_complete int NOT NULL DEFAULT 0,
  total int NOT NULL DEFAULT 0,
  changed int NOT NULL DEFAULT 0,
  deleted int NOT NULL DEFAULT 0,
  error text,
  started_at timestamptz,
  completed_at timestamptz,
  modified_at timestamptz NOT NULL DEFAULT now()
);
