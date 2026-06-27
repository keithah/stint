package db

import (
	"context"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/auth"
)

func (s *Store) upgradeAPIKeyHash(ctx context.Context, keyID uuid.UUID, key string) error {
	return s.updateTokenHash(ctx, `UPDATE api_keys SET key_hash = $1 WHERE id = $2`, keyID, key)
}

func (s *Store) upgradeOAuthAppSecretHash(ctx context.Context, appID uuid.UUID, secret string) error {
	return s.updateTokenHash(ctx, `UPDATE oauth_apps SET client_secret_hash = $1 WHERE id = $2`, appID, secret)
}

func (s *Store) upgradeOAuthAuthorizationCodeHash(ctx context.Context, codeID uuid.UUID, code string) error {
	return s.updateTokenHash(ctx, `UPDATE oauth_authorization_codes SET code_hash = $1 WHERE id = $2`, codeID, code)
}

func (s *Store) upgradeOAuthAccessTokenHash(ctx context.Context, tokenID uuid.UUID, token string) error {
	return s.updateTokenHash(ctx, `UPDATE oauth_tokens SET access_token_hash = $1 WHERE id = $2`, tokenID, token)
}

func (s *Store) upgradeOAuthRefreshTokenHash(ctx context.Context, tokenID uuid.UUID, token string) error {
	return s.updateTokenHash(ctx, `UPDATE oauth_tokens SET refresh_token_hash = $1 WHERE id = $2`, tokenID, token)
}

func (s *Store) upgradeShareTokenHash(ctx context.Context, tokenID uuid.UUID, token string) error {
	return s.updateTokenHash(ctx, `UPDATE share_tokens SET token_hash = $1 WHERE id = $2`, tokenID, token)
}

func (s *Store) updateTokenHash(ctx context.Context, query string, id uuid.UUID, token string) error {
	hash, err := auth.HashAPIKey(token)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, query, hash, id)
	return err
}

func (s *Store) verifyTokenHash(ctx context.Context, hash, token string, upgrade func(context.Context, string) error) bool {
	verified := auth.VerifyAPIKeyDetailed(hash, token)
	if !verified.Valid {
		return false
	}
	if verified.NeedsUpgrade && upgrade != nil {
		_ = upgrade(ctx, token)
	}
	return true
}
