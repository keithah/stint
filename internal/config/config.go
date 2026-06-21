package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port                    string
	BaseURL                 string
	WebBaseURL              string
	DatabaseURL             string
	RedisURL                string
	GitHubClientID          string
	GitHubClientSecret      string
	SessionSecret           string
	StorageType             string
	StoragePath             string
	S3Bucket                string
	S3Region                string
	AWSAccessKeyID          string
	AWSSecretAccessKey      string
	SMTPHost                string
	SMTPPort                int
	SMTPUser                string
	SMTPPass                string
	EmailFrom               string
	EnablePublicLeaderboard bool
	EnableRegistration      bool
	MaxUsers                int
	DevSeedEnabled          bool
	HeartbeatRetentionDays  int
}

var insecureSessionSecrets = map[string]struct{}{
	"":                             {},
	"dev-session-secret-change-me": {},
	"change-me-in-production":      {},
}

func Load() Config {
	port := getenv("PORT", "8080")
	baseURL := getenv("BASE_URL", "http://localhost:"+port)
	return Config{
		Port:                    port,
		BaseURL:                 baseURL,
		WebBaseURL:              getenv("WEB_BASE_URL", "http://localhost:3000"),
		DatabaseURL:             getenv("DATABASE_URL", "postgres://stint:stint@localhost:5432/stint?sslmode=disable"),
		RedisURL:                getenv("REDIS_URL", "redis://localhost:6379"),
		GitHubClientID:          os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret:      os.Getenv("GITHUB_CLIENT_SECRET"),
		SessionSecret:           getenv("SESSION_SECRET", "dev-session-secret-change-me"),
		StorageType:             getenv("STORAGE_TYPE", "local"),
		StoragePath:             getenv("STORAGE_PATH", "./data/dumps"),
		S3Bucket:                os.Getenv("S3_BUCKET"),
		S3Region:                getenv("S3_REGION", "us-east-1"),
		AWSAccessKeyID:          os.Getenv("AWS_ACCESS_KEY_ID"),
		AWSSecretAccessKey:      os.Getenv("AWS_SECRET_ACCESS_KEY"),
		SMTPHost:                os.Getenv("SMTP_HOST"),
		SMTPPort:                getenvInt("SMTP_PORT", 587),
		SMTPUser:                os.Getenv("SMTP_USER"),
		SMTPPass:                os.Getenv("SMTP_PASS"),
		EmailFrom:               getenv("EMAIL_FROM", "noreply@example.com"),
		EnablePublicLeaderboard: getenvBool("ENABLE_PUBLIC_LEADERBOARD", true),
		EnableRegistration:      getenvBool("ENABLE_REGISTRATION", true),
		MaxUsers:                getenvInt("MAX_USERS", 0),
		DevSeedEnabled:          getenvBool("DEV_SEED_ENABLED", defaultDevSeedEnabled(baseURL)),
		HeartbeatRetentionDays:  getenvInt("HEARTBEAT_RETENTION_DAYS", 0),
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func (c Config) Validate() error {
	storageType := strings.ToLower(strings.TrimSpace(c.StorageType))
	if storageType == "" {
		storageType = "local"
	}
	if storageType != "local" {
		return fmt.Errorf("STORAGE_TYPE %q is not supported yet; use STORAGE_TYPE=local", c.StorageType)
	}
	if isLocalBaseURL(c.BaseURL) {
		return nil
	}
	if sessionSecretIsInsecure(c.SessionSecret) {
		return fmt.Errorf("SESSION_SECRET must be set to a strong random value for public BASE_URL %q", c.BaseURL)
	}
	if !c.DevSeedEnabled && (strings.TrimSpace(c.GitHubClientID) == "" || strings.TrimSpace(c.GitHubClientSecret) == "") {
		return fmt.Errorf("GITHUB_CLIENT_ID and GITHUB_CLIENT_SECRET must be set for public BASE_URL %q", c.BaseURL)
	}
	return nil
}

func sessionSecretIsInsecure(secret string) bool {
	trimmed := strings.TrimSpace(secret)
	if _, ok := insecureSessionSecrets[trimmed]; ok {
		return true
	}
	return len(trimmed) < 32
}

func defaultDevSeedEnabled(baseURL string) bool {
	return isLocalBaseURL(baseURL)
}

func isLocalBaseURL(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	switch parsed.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
