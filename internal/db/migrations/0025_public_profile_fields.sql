-- Personal-info profile fields and the owner-selected profile layout live in a
-- single jsonb blob so optional fields and an evolving visibility model
-- (public/private now, org/team later) extend without further migrations.
ALTER TABLE users ADD COLUMN IF NOT EXISTS public_profile jsonb NOT NULL DEFAULT '{}'::jsonb;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'users_public_profile_object_check'
      AND conrelid = 'users'::regclass
  ) THEN
    ALTER TABLE users
      ADD CONSTRAINT users_public_profile_object_check
      CHECK (jsonb_typeof(public_profile) = 'object');
  END IF;
END $$;
