package auth

import (
	"crypto/rand"
	"encoding/hex"
)

const OAuthBearerPrefix = "waka_tok_"

func GenerateOAuthClientSecret() (string, string, string, error) {
	return generateSecret("stints_", 32)
}

func GenerateOAuthBearerToken() (string, string, string, error) {
	return generateSecret(OAuthBearerPrefix, 31)
}

func GenerateOAuthRefreshToken() (string, string, string, error) {
	return generateSecret("stintr_", 32)
}

func GenerateOAuthCode() (string, string, string, error) {
	return generateSecret("stintc_", 24)
}

func VerifyOAuthSecret(hash, value string) bool {
	return VerifyAPIKey(hash, value)
}

func VerifyOAuthSecretDetailed(hash, value string) VerifyResult {
	return VerifyAPIKeyDetailed(hash, value)
}

func generateSecret(prefix string, n int) (string, string, string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", "", err
	}
	value := prefix + hex.EncodeToString(bytes)
	hash, err := HashAPIKey(value)
	if err != nil {
		return "", "", "", err
	}
	return value, KeyFingerprint(value), hash, nil
}
