package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/config"
	"github.com/keithah/stint/internal/db"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestPostgresHandlerIntegrationStoresWakaTimeHeartbeat(t *testing.T) {
	ctx := context.Background()
	store := openTestPostgresStore(t, ctx)

	user, err := store.UpsertGitHubUser(ctx, db.GitHubProfile{ID: 1001, Username: "handler-integration", Email: "handler@example.test"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	_, rawKey, err := store.CreateAPIKeyWithScopes(ctx, user.ID, "handler integration", nil)
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}

	router := NewRouter(config.Config{
		BaseURL:       "http://api.example.test",
		WebBaseURL:    "http://web.example.test",
		SessionSecret: "test-session-secret-with-enough-bytes",
	}, store)

	heartbeatTime := float64(time.Now().UTC().Add(-time.Minute).Unix())
	body, err := json.Marshal(map[string]any{
		"entity":       "/workspace/stint/main.go",
		"type":         "file",
		"category":     "coding",
		"time":         heartbeatTime,
		"project":      "stint",
		"language":     "Go",
		"machine_name": "handler-machine",
		"model_name":   "gpt-5.5-codex",
		"llm_provider": "openai",
		"metadata": map[string]any{
			"request_id": "req_handler",
		},
	})
	if err != nil {
		t.Fatalf("marshal heartbeat: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/current/heartbeats", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", basicAPIKey(rawKey))
	req.Header.Set("User-Agent", "wakatime/v1.102.1 (linux-amd64) go1.22.0 vscode/1.89.0 vscode-wakatime/24.3.0")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected heartbeat create status 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var createBody struct {
		Data struct {
			ID              string `json:"id"`
			Editor          string `json:"editor"`
			EditorVersion   string `json:"editor_version"`
			OperatingSystem string `json:"operating_system"`
			AIModel         string `json:"ai_model"`
			AIProvider      string `json:"ai_provider"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &createBody); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createBody.Data.ID == "" || createBody.Data.Editor != "vscode" || createBody.Data.EditorVersion != "1.89.0" || createBody.Data.OperatingSystem != "linux" {
		t.Fatalf("unexpected heartbeat metadata: %#v", createBody.Data)
	}
	if createBody.Data.AIModel != "gpt-5.5-codex" || createBody.Data.AIProvider != "openai" {
		t.Fatalf("unexpected heartbeat AI metadata: %#v", createBody.Data)
	}

	date := time.Unix(int64(heartbeatTime), 0).UTC().Format("2006-01-02")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/users/current/heartbeats?date="+date, nil)
	req.Header.Set("Authorization", basicAPIKey(rawKey))
	rec = httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected heartbeat list status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var listBody struct {
		Data []struct {
			Entity  string `json:"entity"`
			Project string `json:"project"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listBody.Data) != 1 || listBody.Data[0].Entity != "/workspace/stint/main.go" || listBody.Data[0].Project != "stint" {
		t.Fatalf("unexpected persisted heartbeat list: %#v", listBody.Data)
	}

	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/durations?date="+date, http.StatusOK, "stint")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/summaries?start="+date+"&end="+date, http.StatusOK, "stint")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/stats/last_7_days", http.StatusOK, "stint")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/status_bar/today", http.StatusOK, "stint")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/statusbar/today", http.StatusOK, "grand_total")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/all_time_since_today", http.StatusOK, "stint")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/projects", http.StatusOK, "stint")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/projects/stint?range=all_time", http.StatusOK, "stint")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/projects/stint/commits", http.StatusOK, "status")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/machine_names", http.StatusOK, "handler-machine")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/user_agents", http.StatusOK, "vscode")
	assertAuthenticatedPOST(t, router, rawKey, "/api/v1/users/current/file_experts", map[string]any{
		"entity":  "/workspace/stint/main.go",
		"project": "stint",
	}, http.StatusOK, "handler-integration")
}

func TestPostgresHandlerIntegrationManagementEndpoints(t *testing.T) {
	ctx := context.Background()
	store := openTestPostgresStore(t, ctx)

	user, err := store.UpsertGitHubUser(ctx, db.GitHubProfile{ID: 2001, Username: "management-owner", Email: "owner@example.test"})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	member, err := store.UpsertGitHubUser(ctx, db.GitHubProfile{ID: 2002, Username: "management-member", Email: "member@example.test"})
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	_, rawKey, err := store.CreateAPIKeyWithScopes(ctx, user.ID, "management integration", nil)
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}

	router := NewRouter(config.Config{
		BaseURL:       "http://api.example.test",
		WebBaseURL:    "http://web.example.test",
		SessionSecret: "test-session-secret-with-enough-bytes",
		StorageType:   "local",
		StoragePath:   t.TempDir(),
	}, store)

	assertAuthenticatedGET(t, router, rawKey, "/healthz", http.StatusOK, `"ok":true`)
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/editors", http.StatusOK, "vscode")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/program_languages", http.StatusOK, "TypeScript")
	assertAuthenticatedPUT(t, router, rawKey, "/api/v1/users/current", map[string]any{
		"timezone":                 "UTC",
		"timeout_minutes":          20,
		"writes_only":              false,
		"has_public_profile":       true,
		"country":                  "US",
		"heartbeat_retention_days": 365,
	}, http.StatusOK, "management-owner")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/management-owner", http.StatusOK, "management-owner")

	apiKeyID := responseDataKeyID(t, assertAuthenticatedPOSTBody(t, router, rawKey, "/api/v1/api_keys", map[string]any{"name": "secondary"}, http.StatusCreated, "api_key"))
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/api_keys", http.StatusOK, "secondary")
	assertAuthenticatedDELETE(t, router, rawKey, "/api/v1/api_keys/"+apiKeyID, http.StatusNoContent)

	goalID := responseDataID(t, assertAuthenticatedPOSTBody(t, router, rawKey, "/api/v1/users/current/goals", map[string]any{
		"title":            "Management goal",
		"delta":            "day",
		"seconds":          60,
		"projects":         []string{"stint"},
		"languages":        []string{"Go"},
		"editors":          []string{"vscode"},
		"ignore_days":      []string{"sunday"},
		"is_enabled":       true,
		"is_inverse":       false,
		"is_snoozed":       false,
		"ignore_zero_days": false,
	}, http.StatusCreated, "Management goal"))
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/goals", http.StatusOK, "Management goal")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/goals/"+goalID, http.StatusOK, "management-owner")
	assertAuthenticatedPUT(t, router, rawKey, "/api/v1/users/current/goals/"+goalID, map[string]any{
		"title":            "Management goal updated",
		"delta":            "day",
		"seconds":          120,
		"projects":         []string{"stint"},
		"languages":        []string{"Go"},
		"editors":          []string{"vscode"},
		"ignore_days":      []string{"sunday"},
		"is_enabled":       true,
		"is_inverse":       false,
		"is_snoozed":       false,
		"ignore_zero_days": false,
	}, http.StatusOK, "Management goal updated")

	now := time.Now().UTC()
	externalID := responseDataID(t, assertAuthenticatedPOSTBody(t, router, rawKey, "/api/v1/users/current/external_durations", map[string]any{
		"external_id": "ext-1",
		"provider":    "manual",
		"entity":      "Planning",
		"type":        "app",
		"category":    "coding",
		"start_time":  float64(now.Add(-10 * time.Minute).Unix()),
		"end_time":    float64(now.Add(-5 * time.Minute).Unix()),
		"project":     "stint",
		"language":    "Go",
	}, http.StatusCreated, "Planning"))
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/external_durations", http.StatusOK, "Planning")
	assertAuthenticatedPOST(t, router, rawKey, "/api/v1/users/current/external_durations.bulk", []map[string]any{{
		"external_id": "ext-2",
		"provider":    "manual",
		"entity":      "Review",
		"type":        "app",
		"category":    "coding",
		"start_time":  float64(now.Add(-4 * time.Minute).Unix()),
		"end_time":    float64(now.Add(-2 * time.Minute).Unix()),
		"project":     "stint",
	}}, http.StatusAccepted, "Review")
	assertAuthenticatedDELETEWithBody(t, router, rawKey, "/api/v1/users/current/external_durations.bulk", map[string]any{"ids": []string{externalID}}, http.StatusOK, "deleted")

	boardID := responseDataID(t, assertAuthenticatedPOSTBody(t, router, rawKey, "/api/v1/users/current/leaderboards", map[string]any{"name": "Management board", "time_range": "last_7_days"}, http.StatusCreated, "Management board"))
	assertAuthenticatedPOST(t, router, rawKey, "/api/v1/users/current/leaderboards/"+boardID+"/members", map[string]any{"username": member.GitHubUsername}, http.StatusCreated, "management-member")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/leaderboards", http.StatusOK, "Management board")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/leaderboards/"+boardID, http.StatusOK, "management-member")
	assertAuthenticatedPUT(t, router, rawKey, "/api/v1/users/current/leaderboards/"+boardID, map[string]any{"name": "Management board updated", "time_range": "last_30_days"}, http.StatusOK, "Management board updated")
	assertAuthenticatedDELETE(t, router, rawKey, "/api/v1/users/current/leaderboards/"+boardID+"/members/"+member.ID.String(), http.StatusNoContent)

	shareID := responseDataID(t, assertAuthenticatedPOSTBody(t, router, rawKey, "/api/v1/users/current/share_tokens", map[string]any{"name": "public share"}, http.StatusCreated, "public share"))
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/share_tokens", http.StatusOK, "public share")
	assertAuthenticatedDELETE(t, router, rawKey, "/api/v1/users/current/share_tokens/"+shareID, http.StatusNoContent)

	assertAuthenticatedPUT(t, router, rawKey, "/api/v1/users/current/ai_costs", []map[string]any{{
		"agent":                         "Codex",
		"input_cost_per_million_cents":  300,
		"output_cost_per_million_cents": 1200,
	}}, http.StatusOK, "Codex")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/ai_costs", http.StatusOK, "Codex")

	ruleID := responseFirstDataID(t, assertAuthenticatedPUTBody(t, router, rawKey, "/api/v1/users/current/custom_rules", []map[string]any{{
		"action":       "change",
		"source":       "entity",
		"operation":    "contains",
		"source_value": "management",
		"priority":     1,
		"destinations": []map[string]any{{"destination": "project", "destination_value": "managed"}},
	}}, http.StatusOK, "managed"))
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/custom_rules", http.StatusOK, "managed")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/custom_rules_progress", http.StatusOK, "Completed")
	assertAuthenticatedDELETE(t, router, rawKey, "/api/v1/users/current/custom_rules/"+ruleID, http.StatusNoContent)
	assertAuthenticatedDELETEResponse(t, router, rawKey, "/api/v1/users/current/custom_rules_progress", http.StatusOK, "Aborted")

	assertAuthenticatedPOST(t, router, rawKey, "/api/v1/users/current/imports/wakatime", map[string]any{
		"data": []map[string]any{{
			"entity":       "/tmp/imported-management.go",
			"type":         "file",
			"category":     "coding",
			"time":         float64(now.Unix()),
			"project":      "imported-management",
			"language":     "Go",
			"machine_name": "management-machine",
		}},
	}, http.StatusAccepted, "inserted")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/projects/imported-management?range=all_time", http.StatusOK, "imported-management")

	dumpID := responseDataID(t, assertAuthenticatedPOSTBody(t, router, rawKey, "/api/v1/users/current/data_dumps", map[string]any{"type": "heartbeats"}, http.StatusCreated, "heartbeats"))
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/data_dumps", http.StatusOK, dumpID)
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/data_dumps/"+dumpID+"/download", http.StatusOK, "imported-management")

	assertAuthenticatedDELETE(t, router, rawKey, "/api/v1/users/current/goals/"+goalID, http.StatusNoContent)
	assertAuthenticatedDELETE(t, router, rawKey, "/api/v1/users/current/leaderboards/"+boardID, http.StatusNoContent)
}

func TestPostgresHandlerIntegrationPublicBulkAndOAuthAppEndpoints(t *testing.T) {
	ctx := context.Background()
	store := openTestPostgresStore(t, ctx)

	user, err := store.UpsertGitHubUser(ctx, db.GitHubProfile{ID: 3001, Username: "public-owner", Email: "public@example.test"})
	if err != nil {
		t.Fatalf("create public owner: %v", err)
	}
	if _, err := store.UpdateUser(ctx, user.ID, db.UserSettingsInput{Timezone: "UTC", TimeoutMinutes: 15, HasPublicProfile: true, Country: "US", PublicShowTotalTime: true, PublicShowProjects: true, PublicProjectVisibility: "all", PublicShowLanguages: true, PublicShowSummaries: true}); err != nil {
		t.Fatalf("make user public: %v", err)
	}
	_, rawKey, err := store.CreateAPIKeyWithScopes(ctx, user.ID, "public integration", nil)
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}

	router := NewRouter(config.Config{
		BaseURL:                 "http://api.example.test",
		WebBaseURL:              "http://web.example.test",
		SessionSecret:           "test-session-secret-with-enough-bytes",
		EnablePublicLeaderboard: true,
	}, store)

	now := time.Now().UTC()
	bulkBody := assertAuthenticatedPOSTBody(t, router, rawKey, "/api/v1/users/current/heartbeats.bulk", []map[string]any{
		{
			"entity":       "/workspace/public/a.go",
			"type":         "file",
			"category":     "coding",
			"time":         float64(now.Add(-4 * time.Minute).Unix()),
			"project":      "public-project",
			"language":     "Go",
			"machine_name": "public-machine",
			"branch":       "main",
			"commit_hash":  "abcdef1234567890",
		},
		{
			"entity":       "/workspace/public/b.ts",
			"type":         "file",
			"category":     "coding",
			"time":         float64(now.Add(-2 * time.Minute).Unix()),
			"project":      "public-project",
			"language":     "TypeScript",
			"machine_name": "public-machine",
			"branch":       "main",
			"commit_hash":  "abcdef1234567890",
		},
	}, http.StatusAccepted, "responses")
	if !bytes.Contains(bulkBody, []byte("201")) {
		t.Fatalf("expected bulk heartbeat tuple statuses, got %s", bulkBody)
	}

	date := now.Format("2006-01-02")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/stats", http.StatusOK, "public-project")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/stats/last_30_days", http.StatusOK, "TypeScript")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/insights/languages/last_7_days", http.StatusOK, "TypeScript")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/insights/daily_average/last_7_days", http.StatusOK, "seconds")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/projects/public-project/commits", http.StatusOK, "public-project")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/projects/public-project/commits/abcdef1", http.StatusOK, "abcdef1234567890")

	shareBody := assertAuthenticatedPOSTBody(t, router, rawKey, "/api/v1/users/current/share_tokens", map[string]any{"name": "public read"}, http.StatusCreated, "stintshare_")
	shareToken := responseDataToken(t, shareBody)
	assertPublicGET(t, router, "/api/v1/users/public-owner/stats/last_7_days", http.StatusOK, "public-project")
	assertPublicGET(t, router, "/api/v1/users/public-owner/summaries?start="+date+"&end="+date, http.StatusOK, "public-project")
	assertPublicGET(t, router, "/api/v1/users/public-owner/share/"+shareToken+"/stats?range=last_7_days&callback=stint", http.StatusOK, "stint(")
	assertPublicGET(t, router, "/api/v1/users/public-owner/share/"+shareToken+"/summaries?start="+date+"&end="+date, http.StatusOK, "public-project")
	assertPublicGET(t, router, "/api/v1/share/"+shareToken+"/stats?range=last_7_days", http.StatusOK, "public-project")
	assertPublicGET(t, router, "/api/v1/share/"+shareToken+"/summaries?start="+date+"&end="+date, http.StatusOK, "public-project")
	assertPublicGET(t, router, "/api/v1/leaders?language=Go&country=US", http.StatusOK, `"language":"Go"`)

	appID := responseDataID(t, assertAuthenticatedPOSTBody(t, router, rawKey, "/api/v1/oauth/apps", map[string]any{
		"name":          "Local OAuth app",
		"redirect_uris": []string{"http://localhost:9999/callback"},
		"scopes":        []string{"read_stats", "read_summaries"},
	}, http.StatusCreated, "stintc_"))
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/oauth/apps", http.StatusOK, "Local OAuth app")
	assertAuthenticatedDELETE(t, router, rawKey, "/api/v1/oauth/apps/"+appID, http.StatusNoContent)

	heartbeats := assertAuthenticatedGETBody(t, router, rawKey, "/api/v1/users/current/heartbeats?date="+date, http.StatusOK, "public-project")
	heartbeatID := responseFirstDataID(t, heartbeats)
	assertAuthenticatedDELETEWithBody(t, router, rawKey, "/api/v1/users/current/heartbeats.bulk", map[string]any{
		"date": date,
		"ids":  []string{heartbeatID},
	}, http.StatusOK, "deleted")
}

func TestPostgresHandlerIntegrationDevSeedOAuthAndAccountLifecycle(t *testing.T) {
	ctx := context.Background()
	store := openTestPostgresStore(t, ctx)

	router := NewRouter(config.Config{
		BaseURL:                 "http://api.example.test",
		WebBaseURL:              "http://web.example.test",
		SessionSecret:           "test-session-secret-with-enough-bytes",
		DevSeedEnabled:          true,
		EnableRegistration:      true,
		EnablePublicLeaderboard: true,
	}, store)

	seedBody, seedCookies := assertPublicPOSTBody(t, router, "/api/v1/dev/seed-key?github_id=4001&username=oauth-owner", nil, http.StatusCreated, "access_token")
	userID := responseDataUserID(t, seedBody)
	sessionJWT := responseDataAccessToken(t, seedBody)
	assertBearerGET(t, router, sessionJWT, "/api/v1/auth/me", http.StatusOK, "oauth-owner")

	app, err := store.CreateOAuthApp(ctx, responseUUID(t, userID), "Integration OAuth App", []string{"http://localhost:9999/callback"}, []string{"read_stats", "read_summaries"})
	if err != nil {
		t.Fatalf("create OAuth app: %v", err)
	}
	authQuery := url.Values{
		"response_type": {"code"},
		"client_id":     {app.ClientID},
		"redirect_uri":  {"http://localhost:9999/callback"},
		"scope":         {"read_stats read_summaries"},
		"state":         {"state-code"},
	}
	assertSessionGET(t, router, seedCookies, "/oauth/authorize?"+authQuery.Encode(), http.StatusOK, "Integration OAuth App")

	codeLocation := assertSessionFormPOSTLocation(t, router, seedCookies, "/oauth/authorize", url.Values{
		"response_type": {"code"},
		"client_id":     {app.ClientID},
		"redirect_uri":  {"http://localhost:9999/callback"},
		"scope":         {"read_stats read_summaries"},
		"state":         {"state-code"},
		"decision":      {"allow"},
	}, http.StatusFound)
	code := locationQueryValue(t, codeLocation, "code")
	if state := locationQueryValue(t, codeLocation, "state"); state != "state-code" {
		t.Fatalf("expected OAuth state round trip, got %q in %q", state, codeLocation)
	}

	tokenBody := assertOAuthTokenPOST(t, router, app.ClientID, app.ClientSecret, url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {"http://localhost:9999/callback"},
	}, http.StatusOK, "refresh_token")
	oauthAccess := responseAccessToken(t, tokenBody)
	refreshToken := responseRefreshToken(t, tokenBody)
	assertBearerGET(t, router, oauthAccess, "/api/v1/users/current/stats/last_7_days", http.StatusOK, "total_seconds")

	refreshedBody := assertOAuthTokenPOST(t, router, app.ClientID, app.ClientSecret, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}, http.StatusOK, "access_token")
	refreshedAccess := responseAccessToken(t, refreshedBody)

	assertOAuthFormPOST(t, router, "/oauth/revoke", app.ClientID, app.ClientSecret, url.Values{
		"token": {refreshedAccess},
	}, http.StatusOK, "revoked")

	implicitLocation := assertSessionFormPOSTLocation(t, router, seedCookies, "/oauth/authorize", url.Values{
		"response_type": {"token"},
		"client_id":     {app.ClientID},
		"redirect_uri":  {"http://localhost:9999/callback"},
		"scope":         {"read_stats"},
		"state":         {"state-token"},
		"decision":      {"allow"},
	}, http.StatusFound)
	if token := locationFragmentValue(t, implicitLocation, "access_token"); !strings.HasPrefix(token, "waka_tok_") {
		t.Fatalf("expected implicit access token, got location %q", implicitLocation)
	}
	if refresh := locationFragmentValue(t, implicitLocation, "refresh_token"); refresh != "" {
		t.Fatalf("implicit flow must not return refresh token, got %q", refresh)
	}

	assertPublicPOST(t, router, "/api/v1/dev/jobs/heartbeats-purge?retention_days=0", nil, http.StatusOK, "deleted")
	assertPublicPOST(t, router, "/api/v1/dev/jobs/leaderboard-update?range=last_7_days", nil, http.StatusOK, "entries")
	assertPublicPOST(t, router, "/api/v1/dev/jobs/goals-evaluate", nil, http.StatusOK, "evaluated")
	assertPublicPOST(t, router, "/auth/logout", nil, http.StatusOK, `"ok":true`)
	assertBearerDELETEWithBody(t, router, sessionJWT, "/api/v1/users/current", map[string]any{"confirmation": "DELETE"}, http.StatusOK, "deleted")
}

func TestPostgresHandlerEmptyListResponsesUseArrays(t *testing.T) {
	ctx := context.Background()
	store := openTestPostgresStore(t, ctx)

	user, err := store.UpsertGitHubUser(ctx, db.GitHubProfile{ID: 2101, Username: "empty-list-owner", Email: "empty-list@example.test"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	_, rawKey, err := store.CreateAPIKeyWithScopes(ctx, user.ID, "empty list contract", nil)
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}

	router := NewRouter(config.Config{
		BaseURL:       "http://api.example.test",
		WebBaseURL:    "http://web.example.test",
		SessionSecret: "test-session-secret-with-enough-bytes",
	}, store)

	nonEmptyBody := assertAuthenticatedGETBody(t, router, rawKey, "/api/v1/api_keys", http.StatusOK, "")
	var apiKeys struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(nonEmptyBody, &apiKeys); err != nil {
		t.Fatalf("decode /api/v1/api_keys response %s: %v", nonEmptyBody, err)
	}
	if len(apiKeys.Data) == 0 {
		t.Fatalf("GET /api/v1/api_keys: expected authentication key in array, got %s", nonEmptyBody)
	}

	for _, path := range []string{
		"/api/v1/oauth/apps",
		"/api/v1/users/current/share_tokens",
		"/api/v1/users/current/data_dumps",
		"/api/v1/users/current/ai_costs",
		"/api/v1/users/current/custom_rules",
		"/api/v1/users/current/projects",
		"/api/v1/users/current/leaderboards",
	} {
		body := assertAuthenticatedGETBody(t, router, rawKey, path, http.StatusOK, "")
		var decoded struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("decode %s response %s: %v", path, body, err)
		}
		if string(decoded.Data) != "[]" {
			t.Fatalf("GET %s: expected empty data array, got %s in %s", path, decoded.Data, body)
		}
	}
}

func TestPostgresHandlerIntegrationValidationAndNotFoundPaths(t *testing.T) {
	ctx := context.Background()
	store := openTestPostgresStore(t, ctx)

	user, err := store.UpsertGitHubUser(ctx, db.GitHubProfile{ID: 5001, Username: "validation-owner", Email: "validation@example.test"})
	if err != nil {
		t.Fatalf("create validation user: %v", err)
	}
	_, rawKey, err := store.CreateAPIKeyWithScopes(ctx, user.ID, "validation integration", nil)
	if err != nil {
		t.Fatalf("create API key: %v", err)
	}

	router := NewRouter(config.Config{
		BaseURL:            "http://api.example.test",
		WebBaseURL:         "http://web.example.test",
		SessionSecret:      "test-session-secret-with-enough-bytes",
		DevSeedEnabled:     true,
		EnableRegistration: true,
	}, store)

	assertPublicPOST(t, router, "/api/v1/dev/seed-key?github_id=bad", nil, http.StatusBadRequest, "positive integer")
	assertPublicPOST(t, router, "/api/v1/dev/jobs/heartbeats-purge?retention_days=bad", nil, http.StatusBadRequest, "integer")
	assertPublicPOST(t, router, "/api/v1/dev/jobs/leaderboard-update?range=not_a_range", nil, http.StatusBadRequest, "unsupported")
	assertPublicPOST(t, router, "/api/v1/dev/jobs/goals-evaluate?now_unix=bad", nil, http.StatusBadRequest, "positive unix timestamp")

	disabledRouter := NewRouter(config.Config{
		BaseURL:       "http://api.example.test",
		WebBaseURL:    "http://web.example.test",
		SessionSecret: "test-session-secret-with-enough-bytes",
	}, store)
	assertPublicPOST(t, disabledRouter, "/api/v1/dev/seed-key", nil, http.StatusNotFound, "disabled")
	assertPublicPOST(t, disabledRouter, "/api/v1/dev/jobs/goals-evaluate", nil, http.StatusNotFound, "disabled")

	assertPublicGET(t, router, "/api/v1/users/missing-public", http.StatusNotFound, "public user not found")
	assertPublicGET(t, router, "/api/v1/users/missing-public/stats/last_7_days", http.StatusNotFound, "public user not found")
	assertPublicGET(t, router, "/api/v1/share/stintshare_missing/stats", http.StatusNotFound, "share token not found")
	assertPublicGET(t, router, "/api/v1/share/stintshare_missing/summaries", http.StatusNotFound, "share token not found")

	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/heartbeats?date=bad-date", http.StatusBadRequest, "YYYY-MM-DD")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/durations?date=bad-date", http.StatusBadRequest, "YYYY-MM-DD")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/summaries?start=2026-06-19&end=2026-06-18", http.StatusBadRequest, "on or after")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/stats/not_a_range", http.StatusBadRequest, "unsupported")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/insights/not_real/last_7_days", http.StatusBadRequest, "unsupported insight type")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/projects/missing-project", http.StatusNotFound, "project not found")
	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/projects/missing-project/commits", http.StatusNotFound, "project not found")

	assertAuthenticatedPOST(t, router, rawKey, "/api/v1/users/current/heartbeats.bulk", make([]map[string]any, 26), http.StatusBadRequest, "limit is 25")
	assertAuthenticatedPOST(t, router, rawKey, "/api/v1/users/current/heartbeats.bulk", []map[string]any{{"type": "file", "time": float64(time.Now().Unix())}}, http.StatusAccepted, "entity is required")
	assertAuthenticatedPOST(t, router, rawKey, "/api/v1/users/current/heartbeats", map[string]any{"type": "file"}, http.StatusBadRequest, "entity is required")
	assertAuthenticatedPOST(t, router, rawKey, "/api/v1/users/current/file_experts", map[string]any{}, http.StatusBadRequest, "entity is required")

	assertAuthenticatedPOST(t, router, rawKey, "/api/v1/oauth/apps", map[string]any{"name": "", "redirect_uris": []string{}, "scopes": []string{"bad_scope"}}, http.StatusBadRequest, "OAuth app")
	assertAuthenticatedDELETE(t, router, rawKey, "/api/v1/oauth/apps/not-a-uuid", http.StatusBadRequest)
	assertAuthenticatedDELETE(t, router, rawKey, "/api/v1/oauth/apps/00000000-0000-4000-8000-000000000000", http.StatusNotFound)
	assertAuthenticatedPOST(t, router, rawKey, "/api/v1/api_keys", map[string]any{"name": "bad", "scopes": []string{"bad_scope"}}, http.StatusBadRequest, "invalid")
	assertAuthenticatedDELETE(t, router, rawKey, "/api/v1/api_keys/not-a-uuid", http.StatusBadRequest)
	assertAuthenticatedDELETE(t, router, rawKey, "/api/v1/api_keys/00000000-0000-4000-8000-000000000000", http.StatusNotFound)

	assertAuthenticatedGET(t, router, rawKey, "/api/v1/users/current/goals/not-a-uuid", http.StatusBadRequest, "invalid goal id")
	assertAuthenticatedDELETE(t, router, rawKey, "/api/v1/users/current/goals/not-a-uuid", http.StatusBadRequest)
	assertAuthenticatedDELETE(t, router, rawKey, "/api/v1/users/current/goals/00000000-0000-4000-8000-000000000000", http.StatusNotFound)
	assertAuthenticatedDELETE(t, router, rawKey, "/api/v1/users/current/leaderboards/not-a-uuid", http.StatusBadRequest)
	assertAuthenticatedDELETE(t, router, rawKey, "/api/v1/users/current/leaderboards/00000000-0000-4000-8000-000000000000", http.StatusNotFound)
	assertAuthenticatedDELETE(t, router, rawKey, "/api/v1/users/current/custom_rules/not-a-uuid", http.StatusBadRequest)
	assertAuthenticatedDELETE(t, router, rawKey, "/api/v1/users/current/custom_rules/00000000-0000-4000-8000-000000000000", http.StatusNotFound)
	assertAuthenticatedDELETE(t, router, rawKey, "/api/v1/users/current/share_tokens/not-a-uuid", http.StatusBadRequest)
	assertAuthenticatedDELETE(t, router, rawKey, "/api/v1/users/current/share_tokens/00000000-0000-4000-8000-000000000000", http.StatusNotFound)
	assertAuthenticatedDELETEWithBody(t, router, rawKey, "/api/v1/users/current/heartbeats.bulk", map[string]any{"date": "bad", "ids": []string{}}, http.StatusBadRequest, "YYYY-MM-DD")
}

func openTestPostgresStore(t *testing.T, ctx context.Context) *db.Store {
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
	store, err := db.Open(ctx, fmt.Sprintf("postgres://stint:stint@%s:%s/stint_test?sslmode=disable", host, port.Port()))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(store.Close)
	if err := store.RunMigrations(ctx); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return store
}

func basicAPIKey(key string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(key+":"))
}

func basicClientCredentials(clientID, secret string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(clientID+":"+secret))
}

func assertAuthenticatedGET(t *testing.T, router http.Handler, apiKey, path string, wantStatus int, wantBody string) {
	t.Helper()

	_ = assertAuthenticatedGETBody(t, router, apiKey, path, wantStatus, wantBody)
}

func assertAuthenticatedGETBody(t *testing.T, router http.Handler, apiKey, path string, wantStatus int, wantBody string) []byte {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", basicAPIKey(apiKey))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("GET %s: expected status %d, got %d: %s", path, wantStatus, rec.Code, rec.Body.String())
	}
	if wantBody != "" && !bytes.Contains(rec.Body.Bytes(), []byte(wantBody)) {
		t.Fatalf("GET %s: expected body to contain %q, got %s", path, wantBody, rec.Body.String())
	}
	return rec.Body.Bytes()
}

func assertPublicGET(t *testing.T, router http.Handler, path string, wantStatus int, wantBody string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("GET %s: expected status %d, got %d: %s", path, wantStatus, rec.Code, rec.Body.String())
	}
	if wantBody != "" && !bytes.Contains(rec.Body.Bytes(), []byte(wantBody)) {
		t.Fatalf("GET %s: expected body to contain %q, got %s", path, wantBody, rec.Body.String())
	}
}

func assertBearerGET(t *testing.T, router http.Handler, token, path string, wantStatus int, wantBody string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("GET %s: expected status %d, got %d: %s", path, wantStatus, rec.Code, rec.Body.String())
	}
	if wantBody != "" && !bytes.Contains(rec.Body.Bytes(), []byte(wantBody)) {
		t.Fatalf("GET %s: expected body to contain %q, got %s", path, wantBody, rec.Body.String())
	}
}

func assertSessionGET(t *testing.T, router http.Handler, cookies []*http.Cookie, path string, wantStatus int, wantBody string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("GET %s: expected status %d, got %d: %s", path, wantStatus, rec.Code, rec.Body.String())
	}
	if wantBody != "" && !bytes.Contains(rec.Body.Bytes(), []byte(wantBody)) {
		t.Fatalf("GET %s: expected body to contain %q, got %s", path, wantBody, rec.Body.String())
	}
}

func assertPublicPOST(t *testing.T, router http.Handler, path string, payload any, wantStatus int, wantBody string) {
	t.Helper()

	_, _ = assertPublicPOSTBody(t, router, path, payload, wantStatus, wantBody)
}

func assertPublicPOSTBody(t *testing.T, router http.Handler, path string, payload any, wantStatus int, wantBody string) ([]byte, []*http.Cookie) {
	t.Helper()

	var bodyReader *bytes.Reader
	if payload == nil {
		bodyReader = bytes.NewReader(nil)
	} else {
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal POST %s payload: %v", path, err)
		}
		bodyReader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(http.MethodPost, path, bodyReader)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("POST %s: expected status %d, got %d: %s", path, wantStatus, rec.Code, rec.Body.String())
	}
	if wantBody != "" && !bytes.Contains(rec.Body.Bytes(), []byte(wantBody)) {
		t.Fatalf("POST %s: expected body to contain %q, got %s", path, wantBody, rec.Body.String())
	}
	return rec.Body.Bytes(), rec.Result().Cookies()
}

func assertAuthenticatedPOST(t *testing.T, router http.Handler, apiKey, path string, payload any, wantStatus int, wantBody string) {
	t.Helper()

	_ = assertAuthenticatedPOSTBody(t, router, apiKey, path, payload, wantStatus, wantBody)
}

func assertAuthenticatedPOSTBody(t *testing.T, router http.Handler, apiKey, path string, payload any, wantStatus int, wantBody string) []byte {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal POST %s payload: %v", path, err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", basicAPIKey(apiKey))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("POST %s: expected status %d, got %d: %s", path, wantStatus, rec.Code, rec.Body.String())
	}
	if wantBody != "" && !bytes.Contains(rec.Body.Bytes(), []byte(wantBody)) {
		t.Fatalf("POST %s: expected body to contain %q, got %s", path, wantBody, rec.Body.String())
	}
	return rec.Body.Bytes()
}

func assertAuthenticatedPUT(t *testing.T, router http.Handler, apiKey, path string, payload any, wantStatus int, wantBody string) {
	t.Helper()

	_ = assertAuthenticatedPUTBody(t, router, apiKey, path, payload, wantStatus, wantBody)
}

func assertAuthenticatedPUTBody(t *testing.T, router http.Handler, apiKey, path string, payload any, wantStatus int, wantBody string) []byte {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal PUT %s payload: %v", path, err)
	}
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", basicAPIKey(apiKey))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("PUT %s: expected status %d, got %d: %s", path, wantStatus, rec.Code, rec.Body.String())
	}
	if wantBody != "" && !bytes.Contains(rec.Body.Bytes(), []byte(wantBody)) {
		t.Fatalf("PUT %s: expected body to contain %q, got %s", path, wantBody, rec.Body.String())
	}
	return rec.Body.Bytes()
}

func assertAuthenticatedDELETE(t *testing.T, router http.Handler, apiKey, path string, wantStatus int) {
	t.Helper()

	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set("Authorization", basicAPIKey(apiKey))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("DELETE %s: expected status %d, got %d: %s", path, wantStatus, rec.Code, rec.Body.String())
	}
}

func assertAuthenticatedDELETEResponse(t *testing.T, router http.Handler, apiKey, path string, wantStatus int, wantBody string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set("Authorization", basicAPIKey(apiKey))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("DELETE %s: expected status %d, got %d: %s", path, wantStatus, rec.Code, rec.Body.String())
	}
	if wantBody != "" && !bytes.Contains(rec.Body.Bytes(), []byte(wantBody)) {
		t.Fatalf("DELETE %s: expected body to contain %q, got %s", path, wantBody, rec.Body.String())
	}
}

func assertAuthenticatedDELETEWithBody(t *testing.T, router http.Handler, apiKey, path string, payload any, wantStatus int, wantBody string) {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal DELETE %s payload: %v", path, err)
	}
	req := httptest.NewRequest(http.MethodDelete, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", basicAPIKey(apiKey))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("DELETE %s: expected status %d, got %d: %s", path, wantStatus, rec.Code, rec.Body.String())
	}
	if wantBody != "" && !bytes.Contains(rec.Body.Bytes(), []byte(wantBody)) {
		t.Fatalf("DELETE %s: expected body to contain %q, got %s", path, wantBody, rec.Body.String())
	}
}

func assertBearerDELETEWithBody(t *testing.T, router http.Handler, token, path string, payload any, wantStatus int, wantBody string) {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal DELETE %s payload: %v", path, err)
	}
	req := httptest.NewRequest(http.MethodDelete, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("DELETE %s: expected status %d, got %d: %s", path, wantStatus, rec.Code, rec.Body.String())
	}
	if wantBody != "" && !bytes.Contains(rec.Body.Bytes(), []byte(wantBody)) {
		t.Fatalf("DELETE %s: expected body to contain %q, got %s", path, wantBody, rec.Body.String())
	}
}

func assertSessionFormPOSTLocation(t *testing.T, router http.Handler, cookies []*http.Cookie, path string, values url.Values, wantStatus int) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("POST %s: expected status %d, got %d: %s", path, wantStatus, rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if location == "" {
		t.Fatalf("POST %s: expected Location header", path)
	}
	return location
}

func assertOAuthTokenPOST(t *testing.T, router http.Handler, clientID, secret string, values url.Values, wantStatus int, wantBody string) []byte {
	t.Helper()

	return assertOAuthFormPOST(t, router, "/oauth/token", clientID, secret, values, wantStatus, wantBody)
}

func assertOAuthFormPOST(t *testing.T, router http.Handler, path, clientID, secret string, values url.Values, wantStatus int, wantBody string) []byte {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", basicClientCredentials(clientID, secret))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("POST %s: expected status %d, got %d: %s", path, wantStatus, rec.Code, rec.Body.String())
	}
	if wantBody != "" && !bytes.Contains(rec.Body.Bytes(), []byte(wantBody)) {
		t.Fatalf("POST %s: expected body to contain %q, got %s", path, wantBody, rec.Body.String())
	}
	return rec.Body.Bytes()
}

func responseDataID(t *testing.T, raw []byte) string {
	t.Helper()

	var body struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response id from %s: %v", raw, err)
	}
	if body.Data.ID == "" {
		t.Fatalf("response did not include data.id: %s", raw)
	}
	return body.Data.ID
}

func responseDataUserID(t *testing.T, raw []byte) string {
	t.Helper()

	var body struct {
		Data struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response user id from %s: %v", raw, err)
	}
	if body.Data.User.ID == "" {
		t.Fatalf("response did not include data.user.id: %s", raw)
	}
	return body.Data.User.ID
}

func responseDataAccessToken(t *testing.T, raw []byte) string {
	t.Helper()

	var body struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response access token from %s: %v", raw, err)
	}
	if body.Data.AccessToken == "" {
		t.Fatalf("response did not include data.access_token: %s", raw)
	}
	return body.Data.AccessToken
}

func responseAccessToken(t *testing.T, raw []byte) string {
	t.Helper()

	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode access token from %s: %v", raw, err)
	}
	if body.AccessToken == "" {
		t.Fatalf("response did not include access_token: %s", raw)
	}
	return body.AccessToken
}

func responseRefreshToken(t *testing.T, raw []byte) string {
	t.Helper()

	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode refresh token from %s: %v", raw, err)
	}
	if body.RefreshToken == "" {
		t.Fatalf("response did not include refresh_token: %s", raw)
	}
	return body.RefreshToken
}

func responseUUID(t *testing.T, value string) uuid.UUID {
	t.Helper()

	id, err := uuid.Parse(value)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", value, err)
	}
	return id
}

func responseDataKeyID(t *testing.T, raw []byte) string {
	t.Helper()

	var body struct {
		Data struct {
			Key struct {
				ID string `json:"id"`
			} `json:"key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response key id from %s: %v", raw, err)
	}
	if body.Data.Key.ID == "" {
		t.Fatalf("response did not include data.key.id: %s", raw)
	}
	return body.Data.Key.ID
}

func responseDataToken(t *testing.T, raw []byte) string {
	t.Helper()

	var body struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response token from %s: %v", raw, err)
	}
	if body.Data.Token == "" {
		t.Fatalf("response did not include data.token: %s", raw)
	}
	return body.Data.Token
}

func responseFirstDataID(t *testing.T, raw []byte) string {
	t.Helper()

	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode first response id from %s: %v", raw, err)
	}
	if len(body.Data) == 0 || body.Data[0].ID == "" {
		t.Fatalf("response did not include data[0].id: %s", raw)
	}
	return body.Data[0].ID
}

func locationQueryValue(t *testing.T, rawLocation, key string) string {
	t.Helper()

	parsed, err := url.Parse(rawLocation)
	if err != nil {
		t.Fatalf("parse location %q: %v", rawLocation, err)
	}
	value := parsed.Query().Get(key)
	if value == "" {
		t.Fatalf("location %q did not include query value %q", rawLocation, key)
	}
	return value
}

func locationFragmentValue(t *testing.T, rawLocation, key string) string {
	t.Helper()

	parsed, err := url.Parse(rawLocation)
	if err != nil {
		t.Fatalf("parse location %q: %v", rawLocation, err)
	}
	values, err := url.ParseQuery(parsed.Fragment)
	if err != nil {
		t.Fatalf("parse location fragment %q: %v", parsed.Fragment, err)
	}
	return values.Get(key)
}
