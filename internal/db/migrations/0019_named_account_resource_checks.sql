DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'api_keys_name_check'
      AND conrelid = 'api_keys'::regclass
  ) THEN
    ALTER TABLE api_keys
      ADD CONSTRAINT api_keys_name_check
      CHECK (btrim(name) <> '');
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'share_tokens_name_check'
      AND conrelid = 'share_tokens'::regclass
  ) THEN
    ALTER TABLE share_tokens
      ADD CONSTRAINT share_tokens_name_check
      CHECK (btrim(name) <> '');
  END IF;
END $$;
