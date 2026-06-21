ALTER TABLE users ADD COLUMN IF NOT EXISTS public_username text;
ALTER TABLE users ADD COLUMN IF NOT EXISTS public_display_name text;
ALTER TABLE users ADD COLUMN IF NOT EXISTS public_github_link_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN IF NOT EXISTS public_show_total_time boolean NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN IF NOT EXISTS public_show_projects boolean NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN IF NOT EXISTS public_project_visibility text NOT NULL DEFAULT 'public_repos';
ALTER TABLE users ADD COLUMN IF NOT EXISTS public_show_languages boolean NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN IF NOT EXISTS public_show_editors boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN IF NOT EXISTS public_show_machines boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN IF NOT EXISTS public_show_operating_systems boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN IF NOT EXISTS public_show_categories boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN IF NOT EXISTS public_show_ai boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN IF NOT EXISTS public_show_summaries boolean NOT NULL DEFAULT true;

CREATE UNIQUE INDEX IF NOT EXISTS users_public_username_lower_idx
  ON users (lower(public_username))
  WHERE public_username IS NOT NULL AND public_username <> '';

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'users_public_username_check'
      AND conrelid = 'users'::regclass
  ) THEN
    ALTER TABLE users
      ADD CONSTRAINT users_public_username_check
      CHECK (public_username IS NULL OR public_username = '' OR public_username ~ '^[A-Za-z0-9][A-Za-z0-9_-]{1,37}[A-Za-z0-9]$');
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'users_public_project_visibility_check'
      AND conrelid = 'users'::regclass
  ) THEN
    ALTER TABLE users
      ADD CONSTRAINT users_public_project_visibility_check
      CHECK (public_project_visibility IN ('none', 'public_repos', 'all'));
  END IF;
END $$;
