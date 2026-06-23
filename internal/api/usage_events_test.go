package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/keithah/stint/internal/config"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/usage"
)

type usageSummaryTotal struct {
	CostUSD           float64 `json:"cost_usd"`
	MarginalUSD       float64 `json:"marginal_usd"`
	EventCount        int     `json:"event_count"`
	InputTokens       int     `json:"input_tokens"`
	OutputTokens      int     `json:"output_tokens"`
	CacheCreateTokens int     `json:"cache_create_tokens"`
	CacheReadTokens   int     `json:"cache_read_tokens"`
	ReasoningTokens   int     `json:"reasoning_tokens"`
}

type usageSummaryBucket struct {
	Name        string  `json:"name"`
	CostUSD     float64 `json:"cost_usd"`
	MarginalUSD float64 `json:"marginal_usd"`
	Tokens      int     `json:"tokens"`
	EventCount  int     `json:"event_count"`
}

type usageSummaryResponse struct {
	Data struct {
		Range     string               `json:"range"`
		CostMode  string               `json:"cost_mode"`
		Total     usageSummaryTotal    `json:"total"`
		ByAgent   []usageSummaryBucket `json:"by_agent"`
		ByModel   []usageSummaryBucket `json:"by_model"`
		ByProject []usageSummaryBucket `json:"by_project"`
		ByDay     []struct {
			Date    string  `json:"date"`
			CostUSD float64 `json:"cost_usd"`
			Tokens  int     `json:"tokens"`
		} `json:"by_day"`
		UnpricedModels []string `json:"unpriced_models"`
	} `json:"data"`
}

func TestUsageEventsIngestIsIdempotentAndSummaryAggregates(t *testing.T) {
	ctx := context.Background()
	store := openTestPostgresStore(t, ctx)

	user, err := store.UpsertGitHubUser(ctx, db.GitHubProfile{ID: 2002, Username: "usage-events", Email: "usage@example.test"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	_, rawKey, err := store.CreateAPIKeyWithScopes(ctx, user.ID, "usage events integration", nil)
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}

	router := NewRouter(config.Config{
		BaseURL:       "http://api.example.test",
		WebBaseURL:    "http://web.example.test",
		SessionSecret: "test-session-secret-with-enough-bytes",
	}, store)

	ts := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	events := []usage.Event{
		{
			MessageID:    "msg-1",
			RequestID:    "req-1",
			Agent:        "claude-code",
			SessionID:    "sess-1",
			Project:      "stint",
			Model:        "claude-sonnet-4-5",
			InputTokens:  1000,
			OutputTokens: 500,
			Timestamp:    ts,
			BillingType:  usage.BillingAPI,
		},
		{
			MessageID:    "msg-2",
			RequestID:    "req-2",
			Agent:        "claude-code",
			SessionID:    "sess-1",
			Project:      "stint",
			Model:        "claude-sonnet-4-5",
			InputTokens:  2000,
			OutputTokens: 1000,
			Timestamp:    ts,
			BillingType:  usage.BillingAPI,
		},
	}

	// First ingest inserts both events.
	first := postUsageEvents(t, router, rawKey, events)
	if first.Received != 2 || first.Inserted != 2 || first.Duplicates != 0 {
		t.Fatalf("first ingest: expected received=2 inserted=2 duplicates=0, got %+v", first)
	}

	summaryBefore := getUsageSummary(t, router, rawKey)

	// Re-ingesting the exact same batch must be a no-op: all duplicates.
	second := postUsageEvents(t, router, rawKey, events)
	if second.Received != 2 || second.Inserted != 0 || second.Duplicates != 2 {
		t.Fatalf("second ingest: expected received=2 inserted=0 duplicates=2, got %+v", second)
	}

	summaryAfter := getUsageSummary(t, router, rawKey)

	// Idempotency: totals unchanged after the duplicate ingest.
	if summaryBefore.Data.Total != summaryAfter.Data.Total {
		t.Fatalf("summary changed after duplicate ingest: before=%+v after=%+v", summaryBefore.Data.Total, summaryAfter.Data.Total)
	}

	total := summaryAfter.Data.Total
	if total.EventCount != 2 {
		t.Fatalf("expected event_count 2, got %d", total.EventCount)
	}
	if total.InputTokens != 3000 {
		t.Fatalf("expected input_tokens 3000, got %d", total.InputTokens)
	}
	if total.OutputTokens != 1500 {
		t.Fatalf("expected output_tokens 1500, got %d", total.OutputTokens)
	}
	if total.CostUSD <= 0 {
		t.Fatalf("expected positive cost for a known-priced model, got %f", total.CostUSD)
	}
	if len(summaryAfter.Data.UnpricedModels) != 0 {
		t.Fatalf("expected no unpriced models, got %v", summaryAfter.Data.UnpricedModels)
	}

	// Breakdown by agent should attribute the full cost to claude-code.
	if len(summaryAfter.Data.ByAgent) != 1 || summaryAfter.Data.ByAgent[0].Name != "claude-code" {
		t.Fatalf("unexpected by_agent breakdown: %+v", summaryAfter.Data.ByAgent)
	}
	if summaryAfter.Data.ByAgent[0].EventCount != 2 || summaryAfter.Data.ByAgent[0].Tokens != 4500 {
		t.Fatalf("unexpected by_agent totals: %+v", summaryAfter.Data.ByAgent[0])
	}
	if summaryAfter.Data.ByModel[0].Name != "claude-sonnet-4-5" {
		t.Fatalf("unexpected by_model breakdown: %+v", summaryAfter.Data.ByModel)
	}

	// Export endpoint returns the two stored events.
	body := assertAuthenticatedGETBody(t, router, rawKey, "/api/v1/users/current/usage_events", http.StatusOK, "claude-sonnet-4-5")
	var export struct {
		Data []usage.Event `json:"data"`
	}
	if err := json.Unmarshal(body, &export); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if len(export.Data) != 2 {
		t.Fatalf("expected 2 exported events, got %d", len(export.Data))
	}
}

func postUsageEvents(t *testing.T, router http.Handler, rawKey string, events []usage.Event) db.UsageIngestResult {
	t.Helper()
	payload, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal events: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/current/usage_events.bulk", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", basicAPIKey(rawKey))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ingest usage events: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data db.UsageIngestResult `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode ingest response: %v", err)
	}
	return resp.Data
}

func getUsageSummary(t *testing.T, router http.Handler, rawKey string) usageSummaryResponse {
	t.Helper()
	body := assertAuthenticatedGETBody(t, router, rawKey, "/api/v1/users/current/usage_events/summary?range=last_30_days&cost_mode=calculate", http.StatusOK, "total")
	var resp usageSummaryResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	return resp
}
