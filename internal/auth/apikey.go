package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const APIKeyPrefix = "waka_"
const tokenHashSHA256Prefix = "sha256$"

func ExtractAPIKey(r *http.Request) (string, bool) {
	if key := strings.TrimSpace(r.URL.Query().Get("api_key")); key != "" {
		return key, IsAPIKey(key)
	}

	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return "", false
	}
	if strings.HasPrefix(strings.ToLower(header), "bearer ") {
		key := strings.TrimSpace(header[len("Bearer "):])
		return key, IsAPIKey(key)
	}
	if strings.HasPrefix(strings.ToLower(header), "basic ") {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(header[len("Basic "):]))
		if err != nil {
			return "", false
		}
		username, _, _ := strings.Cut(string(raw), ":")
		key := strings.TrimSpace(username)
		return key, IsAPIKey(key)
	}
	return "", false
}

func ExtractBearerToken(r *http.Request) (string, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return "", false
	}
	token := strings.TrimSpace(header[len("Bearer "):])
	return token, token != ""
}

func GenerateAPIKey() (string, string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", "", err
	}
	key := APIKeyPrefix + id.String()
	return key, KeyFingerprint(key), nil
}

func IsAPIKey(key string) bool {
	if strings.HasPrefix(key, APIKeyPrefix) {
		return true
	}
	_, err := uuid.Parse(key)
	return err == nil
}

func KeyFingerprint(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:8])
}

func HashAPIKey(key string) (string, error) {
	sum := sha256.Sum256([]byte(key))
	return tokenHashSHA256Prefix + hex.EncodeToString(sum[:]), nil
}

type VerifyResult struct {
	Valid        bool
	NeedsUpgrade bool
}

func VerifyAPIKeyDetailed(hash, key string) VerifyResult {
	if strings.HasPrefix(hash, tokenHashSHA256Prefix) {
		want := strings.TrimPrefix(hash, tokenHashSHA256Prefix)
		sum := sha256.Sum256([]byte(key))
		got := hex.EncodeToString(sum[:])
		return VerifyResult{Valid: subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1}
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(key)) == nil {
		return VerifyResult{Valid: true, NeedsUpgrade: true}
	}
	return VerifyResult{}
}

func HashAPIKeyBcryptForTest(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("key is required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(key), bcrypt.DefaultCost)
	return string(hash), err
}

func VerifyAPIKey(hash, key string) bool {
	return VerifyAPIKeyDetailed(hash, key).Valid
}
