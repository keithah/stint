package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestValidateCustomPricing(t *testing.T) {
	if err := ValidateCustomPricing(CustomPricing{Model: "  ", InputPerMillionUSD: 1}); err == nil {
		t.Fatal("expected error for blank model")
	}
	if err := ValidateCustomPricing(CustomPricing{Model: "m", InputPerMillionUSD: -1}); err == nil {
		t.Fatal("expected error for negative price")
	}
	if err := ValidateCustomPricing(CustomPricing{Model: "opencode/big-pickle", InputPerMillionUSD: 1.5, OutputPerMillionUSD: 6}); err != nil {
		t.Fatalf("expected valid pricing, got %v", err)
	}
}

func TestCustomPricingStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := openCustomPricingTestStore(t, ctx)

	user, err := store.UpsertGitHubUser(ctx, GitHubProfile{ID: 7007, Username: "custom-pricing", Email: "cp@example.test"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Empty to start.
	prices, err := store.ListCustomPricing(ctx, user.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(prices) != 0 {
		t.Fatalf("expected no prices, got %d", len(prices))
	}

	// Insert.
	if err := store.UpsertCustomPricing(ctx, user.ID, CustomPricing{
		Model:                   "opencode/big-pickle",
		InputPerMillionUSD:      1.5,
		OutputPerMillionUSD:     6,
		CacheWritePerMillionUSD: 1.875,
		CacheReadPerMillionUSD:  0.15,
	}); err != nil {
		t.Fatalf("upsert insert: %v", err)
	}

	prices, err = store.ListCustomPricing(ctx, user.ID)
	if err != nil {
		t.Fatalf("list after insert: %v", err)
	}
	if len(prices) != 1 || prices[0].Model != "opencode/big-pickle" || prices[0].InputPerMillionUSD != 1.5 {
		t.Fatalf("unexpected prices after insert: %+v", prices)
	}

	// Update (same model id) keeps a single row.
	if err := store.UpsertCustomPricing(ctx, user.ID, CustomPricing{
		Model:               "opencode/big-pickle",
		InputPerMillionUSD:  2,
		OutputPerMillionUSD: 8,
	}); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	prices, err = store.ListCustomPricing(ctx, user.ID)
	if err != nil {
		t.Fatalf("list after update: %v", err)
	}
	if len(prices) != 1 || prices[0].InputPerMillionUSD != 2 || prices[0].OutputPerMillionUSD != 8 {
		t.Fatalf("expected single updated row, got %+v", prices)
	}

	// Delete.
	if err := store.DeleteCustomPricing(ctx, user.ID, "opencode/big-pickle"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	prices, err = store.ListCustomPricing(ctx, user.ID)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(prices) != 0 {
		t.Fatalf("expected no prices after delete, got %d", len(prices))
	}
}

func openCustomPricingTestStore(t *testing.T, ctx context.Context) *Store {
	t.Helper()
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
	store, err := Open(ctx, fmt.Sprintf("postgres://stint:stint@%s:%s/stint_test?sslmode=disable", host, port.Port()))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(store.Close)
	if err := store.RunMigrations(ctx); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return store
}
