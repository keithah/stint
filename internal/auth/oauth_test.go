package auth

import "testing"

func TestGenerateOAuthClientSecretReturnsPlainSecretAndHash(t *testing.T) {
	secret, fingerprint, hash, err := GenerateOAuthClientSecret()
	if err != nil {
		t.Fatalf("GenerateOAuthClientSecret returned error: %v", err)
	}
	if secret == "" || fingerprint == "" || hash == "" {
		t.Fatal("expected secret, fingerprint, and hash")
	}
	if !VerifyOAuthSecret(hash, secret) {
		t.Fatal("expected generated secret to verify against hash")
	}
}

func TestGenerateOAuthBearerTokenUsesOAuthTokenPrefix(t *testing.T) {
	token, fingerprint, hash, err := GenerateOAuthBearerToken()
	if err != nil {
		t.Fatalf("GenerateOAuthBearerToken returned error: %v", err)
	}
	if len(token) <= len(OAuthBearerPrefix) || token[:len(OAuthBearerPrefix)] != OAuthBearerPrefix {
		t.Fatalf("expected token to use %q prefix, got %q", OAuthBearerPrefix, token)
	}
	if fingerprint != KeyFingerprint(token) {
		t.Fatal("expected bearer token fingerprint to match KeyFingerprint")
	}
	if !VerifyOAuthSecret(hash, token) {
		t.Fatal("expected generated bearer token to verify against hash")
	}
}

func TestGenerateOAuthCodeIsNotBearerToken(t *testing.T) {
	code, fingerprint, hash, err := GenerateOAuthCode()
	if err != nil {
		t.Fatalf("GenerateOAuthCode returned error: %v", err)
	}
	if len(code) >= len(APIKeyPrefix) && code[:len(APIKeyPrefix)] == APIKeyPrefix {
		t.Fatalf("authorization code must not look like an API token: %q", code)
	}
	if fingerprint != KeyFingerprint(code) {
		t.Fatal("expected code fingerprint to match KeyFingerprint")
	}
	if !VerifyOAuthSecret(hash, code) {
		t.Fatal("expected generated code to verify against hash")
	}
}
