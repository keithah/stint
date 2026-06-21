DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'oauth_apps_name_check'
      AND conrelid = 'oauth_apps'::regclass
  ) THEN
    ALTER TABLE oauth_apps
      ADD CONSTRAINT oauth_apps_name_check
      CHECK (btrim(name) <> '');
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'leaderboards_name_check'
      AND conrelid = 'leaderboards'::regclass
  ) THEN
    ALTER TABLE leaderboards
      ADD CONSTRAINT leaderboards_name_check
      CHECK (btrim(name) <> '');
  END IF;
END $$;
