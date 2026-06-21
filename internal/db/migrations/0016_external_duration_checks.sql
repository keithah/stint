DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'external_durations_required_text_check'
      AND conrelid = 'external_durations'::regclass
  ) THEN
    ALTER TABLE external_durations
      ADD CONSTRAINT external_durations_required_text_check
      CHECK (
        btrim(external_id) <> ''
        AND btrim(provider) <> ''
        AND btrim(entity) <> ''
        AND btrim(type) <> ''
      );
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'external_durations_time_check'
      AND conrelid = 'external_durations'::regclass
  ) THEN
    ALTER TABLE external_durations
      ADD CONSTRAINT external_durations_time_check
      CHECK (start_time > 0 AND end_time > start_time);
  END IF;
END $$;
