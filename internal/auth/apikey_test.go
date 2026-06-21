package auth

import (
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestExtractAPIKeyFromBasicAuth(t *testing.T) {
	key := "waka_1234567890abcdef"
	req := httptest.NewRequest("GET", "/api/v1/users/current", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(key+":")))

	got, ok := ExtractAPIKey(req)
	if !ok {
		t.Fatal("expected API key to be extracted")
	}
	if got != key {
		t.Fatalf("expected %q, got %q", key, got)
	}
}

func TestExtractAPIKeyFromBasicAuthAcceptsWakaTimeUUID(t *testing.T) {
	key := "00000000-0000-4000-8000-000000000000"
	req := httptest.NewRequest("GET", "/api/v1/users/current", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(key+":")))

	got, ok := ExtractAPIKey(req)
	if !ok {
		t.Fatal("expected WakaTime UUID API key to be extracted")
	}
	if got != key {
		t.Fatalf("expected %q, got %q", key, got)
	}
}

func TestExtractAPIKeyFromBasicAuthAcceptsWakaTimeUUIDWithoutPasswordSeparator(t *testing.T) {
	key := "00000000-0000-4000-8000-000000000000"
	req := httptest.NewRequest("GET", "/api/v1/users/current", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(key)))

	got, ok := ExtractAPIKey(req)
	if !ok {
		t.Fatal("expected WakaTime UUID API key without password separator to be extracted")
	}
	if got != key {
		t.Fatalf("expected %q, got %q", key, got)
	}
}

func TestExtractAPIKeyFromBearerAuth(t *testing.T) {
	key := "waka_bearertoken"
	req := httptest.NewRequest("GET", "/api/v1/users/current", nil)
	req.Header.Set("Authorization", "Bearer "+key)

	got, ok := ExtractAPIKey(req)
	if !ok {
		t.Fatal("expected API key to be extracted")
	}
	if got != key {
		t.Fatalf("expected %q, got %q", key, got)
	}
}

func TestExtractAPIKeyFromBearerAcceptsWakaTimeUUID(t *testing.T) {
	key := "00000000-0000-4000-8000-000000000000"
	req := httptest.NewRequest("GET", "/api/v1/users/current", nil)
	req.Header.Set("Authorization", "Bearer "+key)

	got, ok := ExtractAPIKey(req)
	if !ok {
		t.Fatal("expected WakaTime UUID bearer key to be extracted")
	}
	if got != key {
		t.Fatalf("expected %q, got %q", key, got)
	}
}

func TestExtractAPIKeyAcceptsOAuthBearerPrefix(t *testing.T) {
	key := "waka_tok_bearertoken"
	req := httptest.NewRequest("GET", "/api/v1/users/current", nil)
	req.Header.Set("Authorization", "Bearer "+key)

	got, ok := ExtractAPIKey(req)
	if !ok {
		t.Fatal("expected OAuth bearer token to be extracted")
	}
	if got != key {
		t.Fatalf("expected %q, got %q", key, got)
	}
}

func TestGenerateAPIKeyProducesWakaTimeFanoutCompatibleKey(t *testing.T) {
	key, fingerprint, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey returned error: %v", err)
	}
	if !strings.HasPrefix(key, APIKeyPrefix) {
		t.Fatalf("expected generated key to use WakaTime fanout-compatible prefix %q, got %q", APIKeyPrefix, key)
	}
	id := strings.TrimPrefix(key, APIKeyPrefix)
	if _, err := uuid.Parse(id); err != nil {
		t.Fatalf("expected generated key suffix to be a UUID, got %q: %v", id, err)
	}
	if !IsAPIKey(key) {
		t.Fatalf("expected generated key %q to be accepted as an API key", key)
	}
	if fingerprint == "" || fingerprint != KeyFingerprint(key) {
		t.Fatalf("unexpected fingerprint %q for key %q", fingerprint, key)
	}
}

func TestExtractAPIKeyFromQuery(t *testing.T) {
	key := "waka_querytoken"
	req := httptest.NewRequest("GET", "/api/v1/users/current?api_key="+key, nil)

	got, ok := ExtractAPIKey(req)
	if !ok {
		t.Fatal("expected API key to be extracted")
	}
	if got != key {
		t.Fatalf("expected %q, got %q", key, got)
	}
}

func TestExtractAPIKeyRejectsMalformedBasicAuth(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/users/current", nil)
	req.Header.Set("Authorization", "Basic not-base64")

	_, ok := ExtractAPIKey(req)
	if ok {
		t.Fatal("expected malformed Basic auth to be rejected")
	}
}
