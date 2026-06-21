DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'data_dumps_type_check'
      AND conrelid = 'data_dumps'::regclass
  ) THEN
    ALTER TABLE data_dumps
      ADD CONSTRAINT data_dumps_type_check
      CHECK (type IN ('daily', 'heartbeats'));
  END IF;
END $$;
