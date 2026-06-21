DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'leaderboards_time_range_check'
      AND conrelid = 'leaderboards'::regclass
  ) THEN
    ALTER TABLE leaderboards
      ADD CONSTRAINT leaderboards_time_range_check
      CHECK (time_range ~ '^(last_7_days|last_30_days|last_6_months|last_year|all_time|[0-9]{4}|[0-9]{4}-[0-9]{2})$');
  END IF;
END $$;
