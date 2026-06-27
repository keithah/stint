package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/keithah/stint/internal/auth"
	"github.com/keithah/stint/internal/services"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestPostgresIntegrationRunsMigrationsAndStoresHeartbeat(t *testing.T) {
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "postgres:15-alpine",
			Env: map[string]string{
				"POSTGRES_DB":       "stint_test",
				"POSTGRES_USER":     "stint",
				"POSTGRES_PASSWORD": "stint",
			},
			ExposedPorts: []string{"5432/tcp"},
			WaitingFor: wait.ForAll(
				wait.ForListeningPort("5432/tcp"),
				wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			).WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Fatalf("terminate postgres container: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("postgres host: %v", err)
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatalf("postgres port: %v", err)
	}
	databaseURL := fmt.Sprintf("postgres://stint:stint@%s:%s/stint_test?sslmode=disable", host, port.Port())
	store, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(store.Close)
	if err := store.RunMigrations(ctx); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	user, err := store.UpsertGitHubUser(ctx, GitHubProfile{ID: 42, Username: "integration", Email: "integration@example.test"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	apiKey, rawKey, err := store.CreateAPIKeyWithScopes(ctx, user.ID, "integration key", nil)
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}
	if rawKey == "" || apiKey.Fingerprint == "" {
		t.Fatalf("expected API key material and fingerprint, got key=%q fingerprint=%q", rawKey, apiKey.Fingerprint)
	}
	legacyHash, err := auth.HashAPIKeyBcryptForTest(rawKey)
	if err != nil {
		t.Fatalf("hash legacy API key: %v", err)
	}
	if _, err := store.Pool.Exec(ctx, `UPDATE api_keys SET key_hash = $1 WHERE id = $2`, legacyHash, apiKey.ID); err != nil {
		t.Fatalf("write legacy API key hash: %v", err)
	}
	if _, err := store.AuthByAPIKey(ctx, rawKey); err != nil {
		t.Fatalf("auth legacy API key: %v", err)
	}
	var upgradedHash string
	if err := store.Pool.QueryRow(ctx, `SELECT key_hash FROM api_keys WHERE id = $1`, apiKey.ID).Scan(&upgradedHash); err != nil {
		t.Fatalf("read upgraded API key hash: %v", err)
	}
	if upgradedHash == legacyHash {
		t.Fatal("expected legacy API key hash to be upgraded")
	}
	if !auth.VerifyAPIKey(upgradedHash, rawKey) {
		t.Fatal("expected upgraded API key hash to verify")
	}

	heartbeatTime := float64(time.Date(2026, 6, 19, 12, 30, 0, 0, time.UTC).Unix())
	stored, err := store.InsertHeartbeat(ctx, user.ID, services.Heartbeat{
		Entity:          "/workspace/stint/main.go",
		Type:            "file",
		Time:            heartbeatTime,
		Project:         "stint",
		Language:        "Go",
		MachineName:     "workstation",
		Editor:          "vscode",
		OperatingSystem: "linux",
		AIModel:         "gpt-5.5-codex",
		AIProvider:      "openai",
		Metadata:        map[string]any{"request_id": "req_123"},
		RawPayload:      map[string]any{"model_name": "gpt-5.5-codex", "llm_provider": "openai"},
	})
	if err != nil {
		t.Fatalf("insert heartbeat: %v", err)
	}
	if stored.ID == "" {
		t.Fatal("expected stored heartbeat id")
	}

	heartbeats, err := store.HeartbeatsBetween(ctx, user.ID, heartbeatTime-1, heartbeatTime+1)
	if err != nil {
		t.Fatalf("list heartbeats: %v", err)
	}
	if len(heartbeats) != 1 {
		t.Fatalf("expected one heartbeat, got %d", len(heartbeats))
	}
	if heartbeats[0].Project != "stint" || heartbeats[0].Language != "Go" || heartbeats[0].Editor != "vscode" {
		t.Fatalf("unexpected heartbeat round trip: %#v", heartbeats[0])
	}
	if heartbeats[0].AIModel != "gpt-5.5-codex" || heartbeats[0].AIProvider != "openai" {
		t.Fatalf("unexpected AI telemetry round trip: %#v", heartbeats[0])
	}
	if heartbeats[0].Metadata["request_id"] != "req_123" || heartbeats[0].RawPayload["llm_provider"] != "openai" {
		t.Fatalf("unexpected JSON metadata round trip: %#v", heartbeats[0])
	}

	agents, err := store.ListUserAgents(ctx, user.ID)
	if err != nil {
		t.Fatalf("list user agents: %v", err)
	}
	if len(agents) != 1 || agents[0].AIModel != "gpt-5.5-codex" || agents[0].AIProvider != "openai" {
		t.Fatalf("expected user agent AI model/provider coverage, got %#v", agents)
	}
}
