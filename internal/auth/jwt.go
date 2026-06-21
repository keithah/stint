package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidJWT = errors.New("invalid session jwt")
	ErrExpiredJWT = errors.New("expired session jwt")
)

type sessionJWTClaims struct {
	Subject   string `json:"sub"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

func GenerateSessionJWT(userID, secret string, ttl time.Duration) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	now := time.Now()
	claims, err := json.Marshal(sessionJWTClaims{
		Subject:   userID,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
	})
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	return unsigned + "." + signJWT(unsigned, secret), nil
}

func VerifySessionJWT(token, secret string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", ErrInvalidJWT
	}
	unsigned := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(parts[2]), []byte(signJWT(unsigned, secret))) {
		return "", ErrInvalidJWT
	}
	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", ErrInvalidJWT
	}
	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if err := json.Unmarshal(headerRaw, &header); err != nil || header.Algorithm != "HS256" || header.Type != "JWT" {
		return "", ErrInvalidJWT
	}
	claimsRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ErrInvalidJWT
	}
	var claims sessionJWTClaims
	if err := json.Unmarshal(claimsRaw, &claims); err != nil || claims.Subject == "" {
		return "", ErrInvalidJWT
	}
	if time.Now().Unix() >= claims.ExpiresAt {
		return "", ErrExpiredJWT
	}
	return claims.Subject, nil
}

func signJWT(unsigned, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(unsigned))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
