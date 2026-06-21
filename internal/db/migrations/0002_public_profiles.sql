ALTER TABLE users ADD COLUMN IF NOT EXISTS has_public_profile boolean NOT NULL DEFAULT false;
