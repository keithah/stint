package auth

import (
	"strings"
	"testing"
	"time"
)

func TestSessionJWTRoundTrip(t *testing.T) {
	token, err := GenerateSessionJWT("user-123", "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateSessionJWT returned error: %v", err)
	}
	if strings.Count(token, ".") != 2 {
		t.Fatalf("expected compact JWT, got %q", token)
	}

	userID, err := VerifySessionJWT(token, "secret")
	if err != nil {
		t.Fatalf("VerifySessionJWT returned error: %v", err)
	}
	if userID != "user-123" {
		t.Fatalf("expected user-123, got %q", userID)
	}
}

func TestSessionJWTRejectsTampering(t *testing.T) {
	token, err := GenerateSessionJWT("user-123", "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateSessionJWT returned error: %v", err)
	}
	if token[len(token)-1] == 'a' {
		token = token[:len(token)-1] + "b"
	} else {
		token = token[:len(token)-1] + "a"
	}

	if _, err := VerifySessionJWT(token, "secret"); err == nil {
		t.Fatal("expected tampered token to be rejected")
	}
}

func TestSessionJWTRejectsExpiredToken(t *testing.T) {
	token, err := GenerateSessionJWT("user-123", "secret", -time.Hour)
	if err != nil {
		t.Fatalf("GenerateSessionJWT returned error: %v", err)
	}

	if _, err := VerifySessionJWT(token, "secret"); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}
