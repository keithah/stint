ALTER TABLE oauth_tokens
  ALTER COLUMN refresh_token_hash DROP NOT NULL,
  ALTER COLUMN refresh_token_fingerprint DROP NOT NULL;
