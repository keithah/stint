package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoadReadsWebBaseURL(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("BASE_URL", "https://api.example.test")
	t.Setenv("WEB_BASE_URL", "https://app.example.test")

	cfg := Load()

	if cfg.Port != "9090" {
		t.Fatalf("expected port from env, got %q", cfg.Port)
	}
	if cfg.BaseURL != "https://api.example.test" {
		t.Fatalf("expected base URL from env, got %q", cfg.BaseURL)
	}
	if cfg.WebBaseURL != "https://app.example.test" {
		t.Fatalf("expected web base URL from env, got %q", cfg.WebBaseURL)
	}
}

func TestLoadDefaultsWebBaseURLForLocalFrontend(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("BASE_URL", "")
	t.Setenv("WEB_BASE_URL", "")
	t.Setenv("DEV_SEED_ENABLED", "")

	cfg := Load()

	if cfg.BaseURL != "http://localhost:9090" {
		t.Fatalf("expected API base URL to use PORT default, got %q", cfg.BaseURL)
	}
	if cfg.WebBaseURL != "http://localhost:3000" {
		t.Fatalf("expected local web base URL default, got %q", cfg.WebBaseURL)
	}
	if !cfg.DevSeedEnabled {
		t.Fatal("expected dev seed to default enabled for localhost")
	}
}

func TestSpecDocumentsSplitAPIAndWebRuntime(t *testing.T) {
	content, err := os.ReadFile("../../docs/SPEC.md")
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	spec := string(content)
	for _, want := range []string{
		"PORT=8080",
		"WEB_BASE_URL=https://app.example.com",
		"api:",
		"worker:",
		"web:",
		"ports: [\"8080:8080\"]",
		"ports: [\"3001:3000\"]",
	} {
		if !strings.Contains(spec, want) {
			t.Fatalf("expected spec deployment docs to include %q", want)
		}
	}
	for _, stale := range []string{
		"PORT=3000",
		"ports: [\"3000:3000\"]",
		"command: [\"./stint\", \"worker\"]",
	} {
		if strings.Contains(spec, stale) {
			t.Fatalf("expected spec deployment docs not to include stale example %q", stale)
		}
	}
}

func TestLoadDefaultsDevSeedOffForPublicBaseURL(t *testing.T) {
	t.Setenv("BASE_URL", "https://api.example.test")
	t.Setenv("DEV_SEED_ENABLED", "")

	cfg := Load()

	if cfg.DevSeedEnabled {
		t.Fatal("expected dev seed to default disabled for public base URL")
	}
}

func TestLoadExplicitDevSeedOverridesPublicDefault(t *testing.T) {
	t.Setenv("BASE_URL", "https://api.example.test")
	t.Setenv("DEV_SEED_ENABLED", "true")

	cfg := Load()

	if !cfg.DevSeedEnabled {
		t.Fatal("expected explicit dev seed setting to override public base URL default")
	}
}

func TestValidateAllowsLocalDevSessionSecret(t *testing.T) {
	cfg := Config{BaseURL: "http://localhost:8080", SessionSecret: "dev-session-secret-change-me"}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected localhost dev secret to validate, got %v", err)
	}
}

func TestValidateRejectsDefaultSessionSecretForPublicBaseURL(t *testing.T) {
	cfg := Config{BaseURL: "https://api.example.test", SessionSecret: "dev-session-secret-change-me"}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected public base URL with default session secret to fail validation")
	}
	if !strings.Contains(err.Error(), "SESSION_SECRET") {
		t.Fatalf("expected SESSION_SECRET error, got %v", err)
	}
}

func TestValidateRejectsPlaceholderSessionSecretForPublicBaseURL(t *testing.T) {
	cfg := Config{BaseURL: "https://api.example.test", SessionSecret: "change-me-in-production"}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected public base URL with placeholder session secret to fail validation")
	}
}

func TestValidateAllowsStrongSessionSecretForPublicBaseURL(t *testing.T) {
	cfg := Config{
		BaseURL:            "https://api.example.test",
		SessionSecret:      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		GitHubClientID:     "client-id",
		GitHubClientSecret: "client-secret",
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected explicit production session secret to validate, got %v", err)
	}
}

func TestValidateRejectsMissingGitHubOAuthForPublicBaseURL(t *testing.T) {
	cfg := Config{
		BaseURL:       "https://api.example.test",
		SessionSecret: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected public base URL without GitHub OAuth config to fail validation")
	}
	if !strings.Contains(err.Error(), "GITHUB_CLIENT_ID") {
		t.Fatalf("expected GitHub OAuth error, got %v", err)
	}
}

func TestValidateAllowsExplicitDevSeedWithoutGitHubOAuthForPublicBaseURL(t *testing.T) {
	cfg := Config{
		BaseURL:        "https://api.example.test",
		SessionSecret:  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		DevSeedEnabled: true,
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected explicit dev seed public test config to validate, got %v", err)
	}
}

func TestValidateAllowsGitHubOAuthForPublicBaseURL(t *testing.T) {
	cfg := Config{
		BaseURL:            "https://api.example.test",
		SessionSecret:      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		GitHubClientID:     "client-id",
		GitHubClientSecret: "client-secret",
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected public GitHub OAuth config to validate, got %v", err)
	}
}

func TestValidateRejectsUnsupportedStorageType(t *testing.T) {
	cfg := Config{
		BaseURL:            "https://api.example.test",
		SessionSecret:      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		GitHubClientID:     "client-id",
		GitHubClientSecret: "client-secret",
		StorageType:        "s3",
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected unsupported storage type to fail validation")
	}
	if !strings.Contains(err.Error(), "STORAGE_TYPE") {
		t.Fatalf("expected STORAGE_TYPE error, got %v", err)
	}
}

func TestValidateAllowsLocalStorageType(t *testing.T) {
	cfg := Config{
		BaseURL:            "https://api.example.test",
		SessionSecret:      "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		GitHubClientID:     "client-id",
		GitHubClientSecret: "client-secret",
		StorageType:        "local",
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected local storage type to validate, got %v", err)
	}
}

func TestLoadReadsSelfHostFeatureConfig(t *testing.T) {
	t.Setenv("STORAGE_TYPE", "s3")
	t.Setenv("STORAGE_PATH", "/var/lib/stint/dumps")
	t.Setenv("S3_BUCKET", "stint-dumps")
	t.Setenv("S3_REGION", "us-west-2")
	t.Setenv("AWS_ACCESS_KEY_ID", "access")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	t.Setenv("SMTP_HOST", "smtp.example.test")
	t.Setenv("SMTP_PORT", "2525")
	t.Setenv("SMTP_USER", "mailer")
	t.Setenv("SMTP_PASS", "smtp-secret")
	t.Setenv("EMAIL_FROM", "stint@example.test")
	t.Setenv("ENABLE_PUBLIC_LEADERBOARD", "false")
	t.Setenv("ENABLE_REGISTRATION", "false")
	t.Setenv("MAX_USERS", "25")

	cfg := Load()

	if cfg.StorageType != "s3" || cfg.StoragePath != "/var/lib/stint/dumps" {
		t.Fatalf("expected storage config from env, got type=%q path=%q", cfg.StorageType, cfg.StoragePath)
	}
	if cfg.S3Bucket != "stint-dumps" || cfg.S3Region != "us-west-2" || cfg.AWSAccessKeyID != "access" || cfg.AWSSecretAccessKey != "secret" {
		t.Fatalf("expected S3 config from env, got %#v", cfg)
	}
	if cfg.SMTPHost != "smtp.example.test" || cfg.SMTPPort != 2525 || cfg.SMTPUser != "mailer" || cfg.SMTPPass != "smtp-secret" || cfg.EmailFrom != "stint@example.test" {
		t.Fatalf("expected SMTP config from env, got %#v", cfg)
	}
	if cfg.EnablePublicLeaderboard {
		t.Fatal("expected public leaderboard feature flag to be false")
	}
	if cfg.EnableRegistration {
		t.Fatal("expected registration feature flag to be false")
	}
	if cfg.MaxUsers != 25 {
		t.Fatalf("expected max users from env, got %d", cfg.MaxUsers)
	}
}

func TestLoadDefaultsSelfHostFeatureConfig(t *testing.T) {
	cfg := Load()

	if cfg.StorageType != "local" {
		t.Fatalf("expected local storage default, got %q", cfg.StorageType)
	}
	if cfg.StoragePath != "./data/dumps" {
		t.Fatalf("expected default storage path, got %q", cfg.StoragePath)
	}
	if cfg.S3Region != "us-east-1" {
		t.Fatalf("expected default S3 region, got %q", cfg.S3Region)
	}
	if cfg.SMTPPort != 587 {
		t.Fatalf("expected default SMTP port, got %d", cfg.SMTPPort)
	}
	if cfg.EmailFrom != "noreply@example.com" {
		t.Fatalf("expected default email sender, got %q", cfg.EmailFrom)
	}
	if !cfg.EnablePublicLeaderboard {
		t.Fatal("expected public leaderboard to default enabled")
	}
	if !cfg.EnableRegistration {
		t.Fatal("expected registration to default enabled")
	}
	if cfg.MaxUsers != 0 {
		t.Fatalf("expected unlimited users by default, got %d", cfg.MaxUsers)
	}
}
