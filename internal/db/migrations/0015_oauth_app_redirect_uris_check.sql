DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'oauth_apps_redirect_uris_check'
      AND conrelid = 'oauth_apps'::regclass
  ) THEN
    ALTER TABLE oauth_apps
      ADD CONSTRAINT oauth_apps_redirect_uris_check
      CHECK (cardinality(redirect_uris) > 0);
  END IF;
END $$;
