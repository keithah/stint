package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/keithah/stint/internal/config"
	"github.com/labstack/echo/v4"
)

func TestOpenAPIDocsExposeRouteMethodsAndSecurity(t *testing.T) {
	server := &Server{Config: config.Config{BaseURL: "http://example.test"}}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs", nil)
	rec := httptest.NewRecorder()
	if err := server.openAPIDocs(e.NewContext(req, rec)); err != nil {
		t.Fatalf("openAPIDocs returned error: %v", err)
	}

	var doc struct {
		Components struct {
			Schemas         map[string]any            `json:"schemas"`
			SecuritySchemes map[string]map[string]any `json:"securitySchemes"`
		} `json:"components"`
		Paths map[string]map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("could not decode OpenAPI response: %v", err)
	}

	assertMethod := func(path, method string) {
		t.Helper()
		operations, ok := doc.Paths[path]
		if !ok {
			t.Fatalf("expected path %s to be documented", path)
		}
		operation, ok := operations[method]
		if !ok {
			t.Fatalf("expected %s %s to be documented, got methods %#v", method, path, operations)
		}
		if _, ok := operation["responses"]; !ok {
			t.Fatalf("expected %s %s to include responses", method, path)
		}
	}

	assertMethod("/api/v1/users/current/heartbeats", "post")
	assertMethod("/api/v1/users/current", "put")
	assertMethod("/api/v1/users/current", "delete")
	assertMethod("/api/v1/users/{user}", "get")
	assertMethod("/api/v1/users/{user}/stats", "get")
	assertMethod("/api/v1/users/{user}/stats/{range}", "get")
	assertMethod("/api/v1/users/{user}/summaries", "get")
	assertMethod("/api/v1/users/current/projects/{project}/commits", "get")
	assertMethod("/api/v1/users/current/projects/{project}/commits/{hash}", "get")
	assertMethod("/api/v1/users/current/file_experts", "post")
	assertMethod("/api/v1/users/current/statusbar/today", "get")
	assertMethod("/api/v1/users/current/leaderboards/{board}", "put")
	assertMethod("/api/v1/users/current/custom_rules/{rule_id}", "delete")
	assertMethod("/oauth/token", "post")

	secured := doc.Paths["/api/v1/users/current/heartbeats"]["post"]["security"]
	if secured == nil {
		t.Fatal("expected authenticated heartbeat route to include security metadata")
	}
	for _, name := range []string{"BearerAuth", "BasicAuth", "ApiKeyQuery", "SessionCookie"} {
		if _, ok := doc.Components.SecuritySchemes[name]; !ok {
			t.Fatalf("expected OpenAPI security scheme %s to be documented", name)
		}
	}
	if _, ok := doc.Components.SecuritySchemes["OAuthClientBasic"]; !ok {
		t.Fatal("expected OpenAPI security scheme OAuthClientBasic to be documented")
	}
	assertSecurityScheme(t, doc.Components.SecuritySchemes["BearerAuth"], "http", "bearer", "", "")
	assertSecurityScheme(t, doc.Components.SecuritySchemes["BasicAuth"], "http", "basic", "", "")
	assertSecurityScheme(t, doc.Components.SecuritySchemes["OAuthClientBasic"], "http", "basic", "", "")
	assertSecurityScheme(t, doc.Components.SecuritySchemes["ApiKeyQuery"], "apiKey", "", "query", "api_key")
	assertOperationSecurity(t, doc.Paths["/api/v1/users/current/heartbeats"]["post"], "BearerAuth")
	assertOperationSecurity(t, doc.Paths["/api/v1/users/current/heartbeats"]["post"], "BasicAuth")
	assertOperationSecurity(t, doc.Paths["/api/v1/users/current/heartbeats"]["post"], "ApiKeyQuery")
	assertOperationSecurity(t, doc.Paths["/oauth/token"]["post"], "OAuthClientBasic")
	assertOperationSecurity(t, doc.Paths["/oauth/revoke"]["post"], "OAuthClientBasic")

	for _, schema := range []string{
		"Error",
		"ServerMeta",
		"MetaResponse",
		"User",
		"UserResponse",
		"UserDeleteResponse",
		"PublicUser",
		"PublicUserResponse",
		"PublicStatsResponse",
		"PublicSummaryResponse",
		"DeletedCountResponse",
		"Heartbeat",
		"HeartbeatListResponse",
		"EditorMetadata",
		"EditorListResponse",
		"ProgramLanguage",
		"ProgramLanguageListResponse",
		"Stats",
		"StatusBarResponse",
		"WakaTimeStatusBarResponse",
		"AllTimeResponse",
		"InsightResponse",
		"DurationRow",
		"DurationResponse",
		"SummaryDay",
		"SummaryResponse",
		"MachineName",
		"MachineNameListResponse",
		"Project",
		"ProjectListResponse",
		"ProjectDetailResponse",
		"CommitSummary",
		"ProjectCommitListResponse",
		"ProjectCommitResponse",
		"Goal",
		"GoalProgress",
		"GoalResponse",
		"GoalListResponse",
		"WakaTimeGoalResponse",
		"APIKeyCreateRequest",
		"APIKey",
		"APIKeyCreateResponse",
		"APIKeyListResponse",
		"OAuthAppCreateRequest",
		"OAuthApp",
		"OAuthAppResponse",
		"OAuthAppListResponse",
		"UserUpdateRequest",
		"AccountDeleteRequest",
		"HeartbeatBulkDeleteRequest",
		"FileExpertsRequest",
		"GoalInput",
		"ExternalDuration",
		"ExternalDurationListResponse",
		"ExternalDurationResponse",
		"ExternalDurationBulkResponse",
		"ExternalDurationBulkRequest",
		"IDListRequest",
		"Leaderboard",
		"LeaderboardEntry",
		"PublicLeaderboardMeta",
		"PublicLeaderboardResponse",
		"LeaderboardResponse",
		"LeaderboardListResponse",
		"LeaderboardEntriesResponse",
		"LeaderboardInput",
		"LeaderboardMemberRequest",
		"LeaderboardMemberResponse",
		"DataDump",
		"DataDumpResponse",
		"DataDumpListResponse",
		"DataDumpDownloadResponse",
		"DataDumpRequest",
		"CustomRuleProgress",
		"CustomRuleProgressResponse",
		"CustomRuleListResponse",
		"CustomRulesRequest",
		"ShareToken",
		"ShareTokenResponse",
		"ShareTokenListResponse",
		"ShareTokenCreateRequest",
		"WakaTimeImportRequest",
		"WakaTimeImportResponse",
		"FileExpertsResponse",
		"OAuthTokenResponse",
		"OAuthRevokeResponse",
		"LogoutResponse",
		"AICostSettingsResponse",
		"AICostSettingsRequest",
	} {
		if _, ok := doc.Components.Schemas[schema]; !ok {
			t.Fatalf("expected OpenAPI components.schemas.%s to be documented", schema)
		}
	}
	assertRequestRef(t, doc.Paths["/api/v1/users/current/heartbeats"]["post"], "#/components/schemas/Heartbeat")
	assertRequestRef(t, doc.Paths["/api/v1/api_keys"]["post"], "#/components/schemas/APIKeyCreateRequest")
	assertRequestRef(t, doc.Paths["/api/v1/oauth/apps"]["post"], "#/components/schemas/OAuthAppCreateRequest")
	assertRequestRef(t, doc.Paths["/api/v1/users/current"]["put"], "#/components/schemas/UserUpdateRequest")
	assertRequestRef(t, doc.Paths["/api/v1/users/current"]["delete"], "#/components/schemas/AccountDeleteRequest")
	assertRequestRef(t, doc.Paths["/api/v1/users/current/heartbeats.bulk"]["delete"], "#/components/schemas/HeartbeatBulkDeleteRequest")
	assertRequestRef(t, doc.Paths["/api/v1/users/current/file_experts"]["post"], "#/components/schemas/FileExpertsRequest")
	assertRequestRef(t, doc.Paths["/api/v1/users/current/goals"]["post"], "#/components/schemas/GoalInput")
	assertRequestRef(t, doc.Paths["/api/v1/users/current/goals/{goal}"]["put"], "#/components/schemas/GoalInput")
	assertRequestRef(t, doc.Paths["/api/v1/users/current/external_durations"]["post"], "#/components/schemas/ExternalDuration")
	assertRequestRef(t, doc.Paths["/api/v1/users/current/external_durations.bulk"]["post"], "#/components/schemas/ExternalDurationBulkRequest")
	assertRequestRef(t, doc.Paths["/api/v1/users/current/external_durations.bulk"]["delete"], "#/components/schemas/IDListRequest")
	assertRequestRef(t, doc.Paths["/api/v1/users/current/leaderboards"]["post"], "#/components/schemas/LeaderboardInput")
	assertRequestRef(t, doc.Paths["/api/v1/users/current/leaderboards/{board}"]["put"], "#/components/schemas/LeaderboardInput")
	assertRequestRef(t, doc.Paths["/api/v1/users/current/leaderboards/{board}/members"]["post"], "#/components/schemas/LeaderboardMemberRequest")
	assertRequestRef(t, doc.Paths["/api/v1/users/current/data_dumps"]["post"], "#/components/schemas/DataDumpRequest")
	assertRequestRef(t, doc.Paths["/api/v1/users/current/custom_rules"]["put"], "#/components/schemas/CustomRulesRequest")
	assertRequestRef(t, doc.Paths["/api/v1/users/current/share_tokens"]["post"], "#/components/schemas/ShareTokenCreateRequest")
	assertRequestRef(t, doc.Paths["/api/v1/users/current/imports/wakatime"]["post"], "#/components/schemas/WakaTimeImportRequest")
	assertMultipartFileRequest(t, doc.Paths["/api/v1/users/current/imports/wakatime"]["post"], "file")
	assertRequestRef(t, doc.Paths["/api/v1/users/current/ai_costs"]["put"], "#/components/schemas/AICostSettingsRequest")
	assertResponseRef(t, doc.Paths["/api/v1/meta"]["get"], "200", "#/components/schemas/MetaResponse")
	assertResponseRef(t, doc.Paths["/api/v1/auth/me"]["get"], "200", "#/components/schemas/UserResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current"]["get"], "200", "#/components/schemas/UserResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current"]["put"], "200", "#/components/schemas/UserResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current"]["delete"], "200", "#/components/schemas/UserDeleteResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/{user}"]["get"], "200", "#/components/schemas/PublicUserResponse")
	assertResponseRef(t, doc.Paths["/api/v1/api_keys"]["get"], "200", "#/components/schemas/APIKeyListResponse")
	assertResponseRef(t, doc.Paths["/api/v1/api_keys"]["post"], "201", "#/components/schemas/APIKeyCreateResponse")
	assertResponseRef(t, doc.Paths["/api/v1/oauth/apps"]["get"], "200", "#/components/schemas/OAuthAppListResponse")
	assertResponseRef(t, doc.Paths["/api/v1/oauth/apps"]["post"], "201", "#/components/schemas/OAuthAppResponse")
	assertResponseRef(t, doc.Paths["/api/v1/leaders"]["get"], "200", "#/components/schemas/PublicLeaderboardResponse")
	assertResponseRef(t, doc.Paths["/api/v1/editors"]["get"], "200", "#/components/schemas/EditorListResponse")
	assertResponseRef(t, doc.Paths["/api/v1/program_languages"]["get"], "200", "#/components/schemas/ProgramLanguageListResponse")
	assertResponseRef(t, doc.Paths["/auth/logout"]["post"], "200", "#/components/schemas/LogoutResponse")
	assertResponseRef(t, doc.Paths["/oauth/token"]["post"], "200", "#/components/schemas/OAuthTokenResponse")
	assertResponseRef(t, doc.Paths["/oauth/revoke"]["post"], "200", "#/components/schemas/OAuthRevokeResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/{user}/stats"]["get"], "200", "#/components/schemas/PublicStatsResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/{user}/stats/{range}"]["get"], "200", "#/components/schemas/PublicStatsResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/{user}/summaries"]["get"], "200", "#/components/schemas/PublicSummaryResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/heartbeats"]["post"], "202", "#/components/schemas/HeartbeatResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/heartbeats"]["post"], "429", "#/components/schemas/Error")
	assertRetryAfterHeader(t, doc.Paths["/api/v1/users/current/heartbeats"]["post"], "429")
	assertResponseRef(t, doc.Paths["/oauth/token"]["post"], "429", "#/components/schemas/Error")
	assertRetryAfterHeader(t, doc.Paths["/oauth/token"]["post"], "429")
	assertResponseRef(t, doc.Paths["/api/v1/leaders"]["get"], "429", "#/components/schemas/Error")
	assertRetryAfterHeader(t, doc.Paths["/api/v1/leaders"]["get"], "429")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/heartbeats"]["get"], "200", "#/components/schemas/HeartbeatListResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/heartbeats.bulk"]["delete"], "200", "#/components/schemas/DeletedCountResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/file_experts"]["post"], "200", "#/components/schemas/FileExpertsResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/durations"]["get"], "200", "#/components/schemas/DurationResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/summaries"]["get"], "200", "#/components/schemas/SummaryResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/stats"]["get"], "200", "#/components/schemas/StatsRangesResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/stats/{range}"]["get"], "200", "#/components/schemas/StatsResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/stats/{range}"]["get"], "202", "#/components/schemas/StatsResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/status_bar/today"]["get"], "200", "#/components/schemas/StatusBarResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/statusbar/today"]["get"], "200", "#/components/schemas/WakaTimeStatusBarResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/all_time_since_today"]["get"], "200", "#/components/schemas/AllTimeResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/insights/{insight_type}/{range}"]["get"], "200", "#/components/schemas/InsightResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/machine_names"]["get"], "200", "#/components/schemas/MachineNameListResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/projects"]["get"], "200", "#/components/schemas/ProjectListResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/projects/{project}"]["get"], "200", "#/components/schemas/ProjectDetailResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/projects/{project}/commits"]["get"], "200", "#/components/schemas/ProjectCommitListResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/projects/{project}/commits/{hash}"]["get"], "200", "#/components/schemas/ProjectCommitResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/goals"]["get"], "200", "#/components/schemas/GoalListResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/goals"]["post"], "201", "#/components/schemas/GoalResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/goals/{goal}"]["get"], "200", "#/components/schemas/WakaTimeGoalResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/goals/{goal}"]["put"], "200", "#/components/schemas/GoalResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/external_durations"]["get"], "200", "#/components/schemas/ExternalDurationListResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/external_durations"]["post"], "201", "#/components/schemas/ExternalDurationResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/external_durations.bulk"]["post"], "202", "#/components/schemas/ExternalDurationBulkResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/external_durations.bulk"]["delete"], "200", "#/components/schemas/DeletedCountResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/leaderboards"]["get"], "200", "#/components/schemas/LeaderboardListResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/leaderboards"]["post"], "201", "#/components/schemas/LeaderboardResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/leaderboards/{board}"]["get"], "200", "#/components/schemas/LeaderboardEntriesResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/leaderboards/{board}"]["put"], "200", "#/components/schemas/LeaderboardResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/leaderboards/{board}/members"]["post"], "201", "#/components/schemas/LeaderboardMemberResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/data_dumps"]["get"], "200", "#/components/schemas/DataDumpListResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/data_dumps"]["post"], "201", "#/components/schemas/DataDumpResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/data_dumps"]["post"], "202", "#/components/schemas/DataDumpResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/data_dumps/{dump}/download"]["get"], "200", "#/components/schemas/DataDumpDownloadResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/custom_rules"]["get"], "200", "#/components/schemas/CustomRuleListResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/custom_rules"]["put"], "200", "#/components/schemas/CustomRuleListResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/custom_rules_progress"]["get"], "200", "#/components/schemas/CustomRuleProgressResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/custom_rules_progress"]["delete"], "200", "#/components/schemas/CustomRuleProgressResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/share_tokens"]["get"], "200", "#/components/schemas/ShareTokenListResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/share_tokens"]["post"], "201", "#/components/schemas/ShareTokenResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/imports/wakatime"]["post"], "202", "#/components/schemas/WakaTimeImportResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/ai_costs"]["get"], "200", "#/components/schemas/AICostSettingsResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/ai_costs"]["put"], "200", "#/components/schemas/AICostSettingsResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/{user}/share/{token}/stats"]["get"], "200", "#/components/schemas/PublicStatsResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/{user}/share/{token}/summaries"]["get"], "200", "#/components/schemas/PublicSummaryResponse")
	assertResponseRef(t, doc.Paths["/api/v1/share/{token}/stats"]["get"], "200", "#/components/schemas/PublicStatsResponse")
	assertResponseRef(t, doc.Paths["/api/v1/share/{token}/summaries"]["get"], "200", "#/components/schemas/PublicSummaryResponse")
	assertResponseRef(t, doc.Paths["/api/v1/users/current/heartbeats"]["post"], "401", "#/components/schemas/Error")
	assertQueryParameter(t, doc.Paths["/api/v1/leaders"]["get"], "language")
	assertQueryParameter(t, doc.Paths["/api/v1/leaders"]["get"], "country")
	assertQueryParameter(t, doc.Paths["/api/v1/users/{user}/share/{token}/stats"]["get"], "callback")
	assertQueryParameter(t, doc.Paths["/api/v1/users/{user}/share/{token}/summaries"]["get"], "callback")
	assertQueryParameter(t, doc.Paths["/api/v1/share/{token}/stats"]["get"], "callback")
	assertQueryParameter(t, doc.Paths["/api/v1/share/{token}/summaries"]["get"], "callback")
	assertQueryParameter(t, doc.Paths["/api/v1/users/current/heartbeats"]["get"], "date")
	assertQueryParameter(t, doc.Paths["/api/v1/users/current/durations"]["get"], "date")
	assertQueryParameter(t, doc.Paths["/api/v1/users/current/durations"]["get"], "slice_by")
	assertQueryParameter(t, doc.Paths["/api/v1/users/current/summaries"]["get"], "start")
	assertQueryParameter(t, doc.Paths["/api/v1/users/current/summaries"]["get"], "end")
	assertQueryParameter(t, doc.Paths["/api/v1/users/{user}/summaries"]["get"], "start")
	assertQueryParameter(t, doc.Paths["/api/v1/users/{user}/summaries"]["get"], "end")
	assertQueryParameter(t, doc.Paths["/api/v1/users/current/projects/{project}"]["get"], "range")
	assertQueryParameter(t, doc.Paths["/api/v1/users/current/projects/{project}/commits"]["get"], "branch")
	assertQueryParameter(t, doc.Paths["/api/v1/users/current/projects/{project}/commits"]["get"], "page")
	assertPathParameter(t, doc.Paths["/api/v1/users/{user}"]["get"], "user")
	assertPathParameter(t, doc.Paths["/api/v1/users/{user}/stats/{range}"]["get"], "user")
	assertPathParameter(t, doc.Paths["/api/v1/users/{user}/stats/{range}"]["get"], "range")
	assertPathParameter(t, doc.Paths["/api/v1/users/current/stats/{range}"]["get"], "range")
	assertPathParameter(t, doc.Paths["/api/v1/users/current/projects/{project}"]["get"], "project")
	assertPathParameter(t, doc.Paths["/api/v1/users/current/projects/{project}/commits/{hash}"]["get"], "project")
	assertPathParameter(t, doc.Paths["/api/v1/users/current/projects/{project}/commits/{hash}"]["get"], "hash")
	assertPathParameterEnum(t, doc.Paths["/api/v1/users/current/insights/{insight_type}/{range}"]["get"], "insight_type", supportedInsightTypes())
	assertPathParameter(t, doc.Paths["/api/v1/users/current/insights/{insight_type}/{range}"]["get"], "range")
	assertPathParameter(t, doc.Paths["/api/v1/users/current/goals/{goal}"]["put"], "goal")
	assertPathParameter(t, doc.Paths["/api/v1/users/current/leaderboards/{board}/members/{user}"]["delete"], "board")
	assertPathParameter(t, doc.Paths["/api/v1/users/current/leaderboards/{board}/members/{user}"]["delete"], "user")
	assertPathParameter(t, doc.Paths["/api/v1/users/{user}/share/{token}/stats"]["get"], "user")
	assertPathParameter(t, doc.Paths["/api/v1/users/{user}/share/{token}/stats"]["get"], "token")
	assertPathParameter(t, doc.Paths["/api/v1/share/{token}/summaries"]["get"], "token")
	assertPathParameterDescription(t, doc.Paths["/api/v1/share/{token}/summaries"]["get"], "token", "Share token.")
	for _, name := range []string{"response_type", "client_id", "redirect_uri", "scope", "state"} {
		assertQueryParameter(t, doc.Paths["/oauth/authorize"]["get"], name)
		assertFormParameter(t, doc.Paths["/oauth/authorize"]["post"], name)
	}
	assertFormParameter(t, doc.Paths["/oauth/authorize"]["post"], "decision")
	for _, name := range []string{"grant_type", "code", "redirect_uri", "refresh_token", "client_id", "client_secret"} {
		assertFormParameter(t, doc.Paths["/oauth/token"]["post"], name)
	}
	for _, name := range []string{"token", "client_id", "client_secret"} {
		assertFormParameter(t, doc.Paths["/oauth/revoke"]["post"], name)
	}
	assertStatsRangesResponseIsKeyedByRange(t, doc.Components.Schemas["StatsRangesResponse"])
	assertHeartbeatSchemaDocumentsCompatibilityFields(t, doc.Components.Schemas["Heartbeat"])
	assertObjectRequiredFields(t, doc.Components.Schemas["Heartbeat"], []string{"entity", "time"})
	assertDataDumpDownloadResponseIsRawArray(t, doc.Components.Schemas["DataDumpDownloadResponse"])
	assertStatsSchemaDocumentsDashboardFields(t, doc.Components.Schemas["Stats"])
	assertProjectSchemaDocumentsPublicMetadata(t, doc.Components.Schemas["Project"])
	assertCommitSummarySchemaDocumentsLinks(t, doc.Components.Schemas["CommitSummary"])
	assertAIMetricsSchemaDocumentsDashboardFields(t, doc.Components.Schemas["AIMetrics"])
	assertObjectStringEnum(t, doc.Components.Schemas["DataDumpRequest"], "type", []string{"heartbeats", "daily"})
	assertObjectStringEnum(t, doc.Components.Schemas["GoalInput"], "delta", []string{"day", "week"})
	assertObjectStringEnum(t, doc.Components.Schemas["CustomRule"], "action", []string{"change", "delete"})
	assertObjectStringEnum(t, doc.Components.Schemas["CustomRule"], "source", []string{"entity", "type", "category", "project", "branch", "language", "editor", "operating_system"})
	assertObjectStringEnum(t, doc.Components.Schemas["CustomRule"], "operation", []string{"equals", "contains", "starts_with", "ends_with", "regex", "matches"})
	assertObjectStringEnum(t, doc.Components.Schemas["CustomRuleDestination"], "destination", []string{"entity", "type", "category", "project", "branch", "language", "editor", "operating_system"})
	assertObjectStringMinLength(t, doc.Components.Schemas["APIKeyCreateRequest"], "name", float64(1))
	assertObjectStringMinLength(t, doc.Components.Schemas["ShareTokenCreateRequest"], "name", float64(1))
	assertObjectStringMinLength(t, doc.Components.Schemas["OAuthAppCreateRequest"], "name", float64(1))
	assertObjectStringMinLength(t, doc.Components.Schemas["LeaderboardInput"], "name", float64(1))
	assertObjectNumericMinimum(t, doc.Components.Schemas["AICostSetting"], "input_cost_per_million_cents", float64(0))
	assertObjectNumericMinimum(t, doc.Components.Schemas["AICostSetting"], "output_cost_per_million_cents", float64(0))
	assertObjectNumericMinimum(t, doc.Components.Schemas["GoalInput"], "seconds", float64(0))
	assertObjectNumericMinimum(t, doc.Components.Schemas["GoalInput"], "improve_by_percent", float64(0))
	assertObjectStringPattern(t, doc.Components.Schemas["LeaderboardInput"], "time_range", "^(last_7_days|last_30_days|last_6_months|last_year|all_time|[0-9]{4}|[0-9]{4}-[0-9]{2})$")
	assertObjectStringPattern(t, doc.Components.Schemas["UserUpdateRequest"], "country", "^[A-Za-z]{2}$")
	assertObjectStringPattern(t, doc.Components.Schemas["UserUpdateRequest"], "timezone", "^(UTC|[A-Za-z_]+(/[A-Za-z0-9_+\\-]+)+)$")
	assertObjectNumericMinimum(t, doc.Components.Schemas["UserUpdateRequest"], "timeout_minutes", float64(0))
	assertObjectNumericMinimum(t, doc.Components.Schemas["UserUpdateRequest"], "heartbeat_retention_days", float64(0))
	assertArrayMaxItems(t, doc.Components.Schemas["HeartbeatBulkRequest"], float64(25))
	assertArrayMaxItems(t, doc.Components.Schemas["ExternalDurationBulkRequest"], float64(1000))
	assertObjectStringArrayItems(t, doc.Components.Schemas["OAuthAppCreateRequest"], "redirect_uris", float64(1), "uri")
	assertObjectNumericExclusiveMinimum(t, doc.Components.Schemas["ExternalDuration"], "start_time", float64(0))
	assertObjectNumericExclusiveMinimum(t, doc.Components.Schemas["ExternalDuration"], "end_time", float64(0))
}

func assertSecurityScheme(t *testing.T, scheme map[string]any, wantType, wantScheme, wantIn, wantName string) {
	t.Helper()
	if got, _ := scheme["type"].(string); got != wantType {
		t.Fatalf("expected security scheme type %q, got %q in %#v", wantType, got, scheme)
	}
	if wantScheme != "" {
		if got, _ := scheme["scheme"].(string); got != wantScheme {
			t.Fatalf("expected security scheme %q, got %q in %#v", wantScheme, got, scheme)
		}
	}
	if wantIn != "" {
		if got, _ := scheme["in"].(string); got != wantIn {
			t.Fatalf("expected security scheme in %q, got %q in %#v", wantIn, got, scheme)
		}
	}
	if wantName != "" {
		if got, _ := scheme["name"].(string); got != wantName {
			t.Fatalf("expected security scheme name %q, got %q in %#v", wantName, got, scheme)
		}
	}
}

func assertOperationSecurity(t *testing.T, operation map[string]any, scheme string) {
	t.Helper()
	security, ok := operation["security"].([]any)
	if !ok {
		t.Fatalf("expected operation security metadata, got %#v", operation["security"])
	}
	for _, entry := range security {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := item[scheme]; ok {
			return
		}
	}
	t.Fatalf("expected operation security to include %s, got %#v", scheme, security)
}

func TestOpenAPIDocsCoverRegisteredPublicRoutes(t *testing.T) {
	router := NewRouter(config.Config{}, nil)
	docs := openAPIPaths()

	for _, route := range router.Routes() {
		if shouldSkipOpenAPIRoute(route.Method, route.Path) {
			continue
		}
		path := echoPathToOpenAPIPath(route.Path)
		operations, ok := docs[path].(map[string]any)
		if !ok {
			t.Fatalf("expected OpenAPI docs to include registered route %s %s as %s", route.Method, route.Path, path)
		}
		if _, ok := operations[strings.ToLower(route.Method)]; !ok {
			t.Fatalf("expected OpenAPI docs to include method %s for registered route %s", route.Method, path)
		}
	}
}

func shouldSkipOpenAPIRoute(method, path string) bool {
	if method == "echo_route_not_found" || strings.HasSuffix(path, "/*") {
		return true
	}
	return strings.HasPrefix(path, "/api/v1/dev/")
}

func echoPathToOpenAPIPath(path string) string {
	parts := strings.Split(path, "/")
	for index, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[index] = "{" + strings.TrimPrefix(part, ":") + "}"
		}
	}
	return strings.Join(parts, "/")
}

func assertRequestRef(t *testing.T, operation map[string]any, want string) {
	t.Helper()
	requestBody, ok := operation["requestBody"].(map[string]any)
	if !ok {
		t.Fatalf("expected operation to include requestBody")
	}
	content, ok := requestBody["content"].(map[string]any)
	if !ok {
		t.Fatalf("expected requestBody to include content")
	}
	jsonContent, ok := content["application/json"].(map[string]any)
	if !ok {
		t.Fatalf("expected requestBody to include application/json content")
	}
	schema, ok := jsonContent["schema"].(map[string]any)
	if !ok {
		t.Fatalf("expected requestBody application/json content to include schema")
	}
	if got, _ := schema["$ref"].(string); got != want {
		t.Fatalf("expected request schema ref %q, got %q", want, got)
	}
}

func assertResponseRef(t *testing.T, operation map[string]any, status, want string) {
	t.Helper()
	responses, ok := operation["responses"].(map[string]any)
	if !ok {
		t.Fatalf("expected operation to include responses")
	}
	response, ok := responses[status].(map[string]any)
	if !ok {
		t.Fatalf("expected response %s to be documented", status)
	}
	content, ok := response["content"].(map[string]any)
	if !ok {
		t.Fatalf("expected response %s to include content", status)
	}
	jsonContent, ok := content["application/json"].(map[string]any)
	if !ok {
		t.Fatalf("expected response %s to include application/json content", status)
	}
	schema, ok := jsonContent["schema"].(map[string]any)
	if !ok {
		t.Fatalf("expected response %s application/json content to include schema", status)
	}
	if got, _ := schema["$ref"].(string); got != want {
		t.Fatalf("expected response %s schema ref %q, got %q", status, want, got)
	}
}

func assertRetryAfterHeader(t *testing.T, operation map[string]any, status string) {
	t.Helper()
	responses, ok := operation["responses"].(map[string]any)
	if !ok {
		t.Fatalf("expected operation to include responses")
	}
	response, ok := responses[status].(map[string]any)
	if !ok {
		t.Fatalf("expected response %s to be documented", status)
	}
	headers, ok := response["headers"].(map[string]any)
	if !ok {
		t.Fatalf("expected response %s to include headers", status)
	}
	header, ok := headers["Retry-After"].(map[string]any)
	if !ok {
		t.Fatalf("expected response %s to document Retry-After", status)
	}
	schema, ok := header["schema"].(map[string]any)
	if !ok {
		t.Fatalf("expected Retry-After header to include schema")
	}
	if got, _ := schema["type"].(string); got != "integer" {
		t.Fatalf("expected Retry-After schema type integer, got %q", got)
	}
}

func assertResponseStatus(t *testing.T, operation map[string]any, status string) {
	t.Helper()
	responses, ok := operation["responses"].(map[string]any)
	if !ok {
		t.Fatalf("expected operation to include responses")
	}
	if _, ok := responses[status]; !ok {
		t.Fatalf("expected response %s to be documented", status)
	}
}

func assertQueryParameter(t *testing.T, operation map[string]any, name string) {
	t.Helper()
	parameters, ok := operation["parameters"].([]any)
	if !ok {
		t.Fatalf("expected operation to include parameters")
	}
	for _, parameter := range parameters {
		item, ok := parameter.(map[string]any)
		if !ok {
			continue
		}
		if item["name"] == name && item["in"] == "query" {
			return
		}
	}
	t.Fatalf("expected query parameter %q in %#v", name, parameters)
}

func assertPathParameter(t *testing.T, operation map[string]any, name string) {
	t.Helper()
	parameters, ok := operation["parameters"].([]any)
	if !ok {
		t.Fatalf("expected operation to include parameters")
	}
	for _, parameter := range parameters {
		item, ok := parameter.(map[string]any)
		if !ok {
			continue
		}
		if item["name"] == name && item["in"] == "path" && item["required"] == true {
			return
		}
	}
	t.Fatalf("expected required path parameter %q in %#v", name, parameters)
}

func assertPathParameterDescription(t *testing.T, operation map[string]any, name, description string) {
	t.Helper()
	parameters, ok := operation["parameters"].([]any)
	if !ok {
		t.Fatalf("expected operation to include parameters")
	}
	for _, parameter := range parameters {
		item, ok := parameter.(map[string]any)
		if !ok {
			continue
		}
		if item["name"] == name && item["in"] == "path" {
			if item["description"] != description {
				t.Fatalf("expected path parameter %q description %q, got %q", name, description, item["description"])
			}
			return
		}
	}
	t.Fatalf("expected path parameter %q in %#v", name, parameters)
}

func assertPathParameterEnum(t *testing.T, operation map[string]any, name string, want []string) {
	t.Helper()
	parameters, ok := operation["parameters"].([]any)
	if !ok {
		t.Fatalf("expected operation to include parameters")
	}
	for _, parameter := range parameters {
		item, ok := parameter.(map[string]any)
		if !ok || item["name"] != name || item["in"] != "path" {
			continue
		}
		schema, ok := item["schema"].(map[string]any)
		if !ok {
			t.Fatalf("expected path parameter %q schema object, got %#v", name, item["schema"])
		}
		rawEnum, ok := schema["enum"].([]any)
		if !ok {
			t.Fatalf("expected path parameter %q enum, got %#v", name, schema["enum"])
		}
		if len(rawEnum) != len(want) {
			t.Fatalf("expected path parameter %q enum %#v, got %#v", name, want, rawEnum)
		}
		for index, expected := range want {
			if rawEnum[index] != expected {
				t.Fatalf("expected path parameter %q enum %#v, got %#v", name, want, rawEnum)
			}
		}
		return
	}
	t.Fatalf("expected path parameter %q in %#v", name, parameters)
}

func assertFormParameter(t *testing.T, operation map[string]any, name string) {
	t.Helper()
	requestBody, ok := operation["requestBody"].(map[string]any)
	if !ok {
		t.Fatalf("expected operation to include requestBody")
	}
	content, ok := requestBody["content"].(map[string]any)
	if !ok {
		t.Fatalf("expected requestBody to include content")
	}
	formContent, ok := content["application/x-www-form-urlencoded"].(map[string]any)
	if !ok {
		t.Fatalf("expected requestBody to include application/x-www-form-urlencoded content")
	}
	schema, ok := formContent["schema"].(map[string]any)
	if !ok {
		t.Fatalf("expected form requestBody content to include schema")
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected form schema to include properties")
	}
	if _, ok := properties[name]; !ok {
		t.Fatalf("expected form parameter %q in %#v", name, properties)
	}
}

func assertMultipartFileRequest(t *testing.T, operation map[string]any, name string) {
	t.Helper()
	requestBody, ok := operation["requestBody"].(map[string]any)
	if !ok {
		t.Fatalf("expected operation to include requestBody")
	}
	content, ok := requestBody["content"].(map[string]any)
	if !ok {
		t.Fatalf("expected requestBody to include content")
	}
	multipartContent, ok := content["multipart/form-data"].(map[string]any)
	if !ok {
		t.Fatalf("expected requestBody to include multipart/form-data content")
	}
	schema, ok := multipartContent["schema"].(map[string]any)
	if !ok {
		t.Fatalf("expected multipart requestBody content to include schema")
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected multipart schema to include properties")
	}
	file, ok := properties[name].(map[string]any)
	if !ok {
		t.Fatalf("expected multipart file field %q in %#v", name, properties)
	}
	if file["type"] != "string" || file["format"] != "binary" {
		t.Fatalf("expected multipart field %q to be string/binary, got %#v", name, file)
	}
}

func assertObjectStringEnum(t *testing.T, schema any, property string, want []string) {
	t.Helper()
	body, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("expected schema object, got %#v", schema)
	}
	properties, ok := body["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected schema properties, got %#v", body)
	}
	field, ok := properties[property].(map[string]any)
	if !ok {
		t.Fatalf("expected property %q schema, got %#v", property, properties[property])
	}
	if got, _ := field["type"].(string); got != "string" {
		t.Fatalf("expected %q to be string, got %q", property, got)
	}
	rawEnum, ok := field["enum"].([]any)
	if !ok {
		t.Fatalf("expected %q to include string enum, got %#v", property, field["enum"])
	}
	if len(rawEnum) != len(want) {
		t.Fatalf("expected enum %#v, got %#v", want, rawEnum)
	}
	for i := range want {
		if rawEnum[i] != want[i] {
			t.Fatalf("expected enum %#v, got %#v", want, rawEnum)
		}
	}
}

func assertObjectNumericMinimum(t *testing.T, schema any, property string, want float64) {
	t.Helper()
	body, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("expected schema object, got %#v", schema)
	}
	properties, ok := body["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected schema properties, got %#v", body)
	}
	field, ok := properties[property].(map[string]any)
	if !ok {
		t.Fatalf("expected property %q schema, got %#v", property, properties[property])
	}
	got, ok := field["minimum"].(float64)
	if !ok || got != want {
		t.Fatalf("expected %q minimum %v, got %#v", property, want, field["minimum"])
	}
}

func assertObjectNumericExclusiveMinimum(t *testing.T, schema any, property string, want float64) {
	t.Helper()
	body, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("expected schema object, got %#v", schema)
	}
	properties, ok := body["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected schema properties, got %#v", body)
	}
	field, ok := properties[property].(map[string]any)
	if !ok {
		t.Fatalf("expected property %q schema, got %#v", property, properties[property])
	}
	got, ok := field["exclusiveMinimum"].(float64)
	if !ok || got != want {
		t.Fatalf("expected %q exclusiveMinimum %v, got %#v", property, want, field["exclusiveMinimum"])
	}
}

func assertObjectStringPattern(t *testing.T, schema any, property, want string) {
	t.Helper()
	body, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("expected schema object, got %#v", schema)
	}
	properties, ok := body["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected schema properties, got %#v", body)
	}
	field, ok := properties[property].(map[string]any)
	if !ok {
		t.Fatalf("expected property %q schema, got %#v", property, properties[property])
	}
	if got, _ := field["type"].(string); got != "string" {
		t.Fatalf("expected %q to be string, got %q", property, got)
	}
	if got, _ := field["pattern"].(string); got != want {
		t.Fatalf("expected %q pattern %q, got %q", property, want, got)
	}
}

func assertObjectRequiredFields(t *testing.T, schema any, want []string) {
	t.Helper()
	object, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("expected object schema, got %#v", schema)
	}
	required, ok := schemaStringList(object["required"])
	if !ok {
		t.Fatalf("expected required fields %#v, got %#v", want, object["required"])
	}
	if len(required) != len(want) {
		t.Fatalf("expected required fields %#v, got %#v", want, required)
	}
	for index := range want {
		if required[index] != want[index] {
			t.Fatalf("required field %d: expected %q, got %q in %#v", index, want[index], required[index], required)
		}
	}
}

func schemaStringList(value any) ([]string, bool) {
	switch typed := value.(type) {
	case []string:
		return typed, true
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, text)
		}
		return out, true
	default:
		return nil, false
	}
}

func assertObjectStringMinLength(t *testing.T, schema any, property string, want float64) {
	t.Helper()
	body, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("expected schema object, got %#v", schema)
	}
	properties, ok := body["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected schema properties, got %#v", body)
	}
	field, ok := properties[property].(map[string]any)
	if !ok {
		t.Fatalf("expected property %q schema, got %#v", property, properties[property])
	}
	if got, _ := field["type"].(string); got != "string" {
		t.Fatalf("expected %q to be string, got %q", property, got)
	}
	if got, _ := field["minLength"].(float64); got != want {
		t.Fatalf("expected %q minLength %v, got %#v", property, want, field["minLength"])
	}
}

func assertObjectStringArrayItems(t *testing.T, schema any, property string, minItems float64, itemFormat string) {
	t.Helper()
	body, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("expected schema object, got %#v", schema)
	}
	properties, ok := body["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected schema properties, got %#v", body)
	}
	field, ok := properties[property].(map[string]any)
	if !ok {
		t.Fatalf("expected property %q schema, got %#v", property, properties[property])
	}
	if got, _ := field["type"].(string); got != "array" {
		t.Fatalf("expected %q to be array, got %q", property, got)
	}
	if got, _ := field["minItems"].(float64); got != minItems {
		t.Fatalf("expected %q minItems %v, got %#v", property, minItems, field["minItems"])
	}
	items, ok := field["items"].(map[string]any)
	if !ok {
		t.Fatalf("expected %q items schema, got %#v", property, field["items"])
	}
	if got, _ := items["type"].(string); got != "string" {
		t.Fatalf("expected %q items to be string, got %q", property, got)
	}
	if got, _ := items["format"].(string); got != itemFormat {
		t.Fatalf("expected %q items format %q, got %q", property, itemFormat, got)
	}
}

func assertArrayMaxItems(t *testing.T, schema any, want float64) {
	t.Helper()
	body, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("expected array schema object, got %#v", schema)
	}
	if got, _ := body["type"].(string); got != "array" {
		t.Fatalf("expected array schema, got type %q", got)
	}
	if got, _ := body["maxItems"].(float64); got != want {
		t.Fatalf("expected maxItems %v, got %#v", want, body["maxItems"])
	}
}

func assertDataDumpDownloadResponseIsRawArray(t *testing.T, schema any) {
	t.Helper()
	body, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("expected data dump download schema object, got %#v", schema)
	}
	if got, _ := body["type"].(string); got != "array" {
		t.Fatalf("expected data dump download response to be a raw array, got type %q", got)
	}
	items, ok := body["items"].(map[string]any)
	if !ok {
		t.Fatalf("expected data dump download array items schema, got %#v", body["items"])
	}
	if _, ok := items["oneOf"]; !ok {
		t.Fatalf("expected data dump download items to document heartbeat or summary rows, got %#v", items)
	}
}

func assertStatsRangesResponseIsKeyedByRange(t *testing.T, schema any) {
	t.Helper()
	body, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("expected StatsRangesResponse schema object, got %#v", schema)
	}
	properties, ok := body["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected StatsRangesResponse properties, got %#v", body["properties"])
	}
	data, ok := properties["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected StatsRangesResponse.data schema object, got %#v", properties["data"])
	}
	if got, _ := data["type"].(string); got != "object" {
		t.Fatalf("expected StatsRangesResponse.data to be an object keyed by range, got %q", got)
	}
	additional, ok := data["additionalProperties"].(map[string]any)
	if !ok {
		t.Fatalf("expected StatsRangesResponse.data additionalProperties ref, got %#v", data["additionalProperties"])
	}
	if got, _ := additional["$ref"].(string); got != "#/components/schemas/Stats" {
		t.Fatalf("expected StatsRangesResponse.data values to be Stats refs, got %q", got)
	}
}

func assertHeartbeatSchemaDocumentsCompatibilityFields(t *testing.T, schema any) {
	t.Helper()
	heartbeat, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("expected Heartbeat schema to be an object, got %#v", schema)
	}
	properties, ok := heartbeat["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected Heartbeat schema to include properties")
	}
	for _, field := range []string{"alternate_project", "dependencies", "lineno", "cursorpos", "plugin", "plugin_version", "editor_version", "architecture", "model_name", "llm_model", "ai_provider", "provider", "llm_provider", "metadata", "raw_payload"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("expected Heartbeat schema to document %q", field)
		}
	}
	dependencies, ok := properties["dependencies"].(map[string]any)
	if !ok {
		t.Fatalf("expected dependencies schema to be an object")
	}
	if _, ok := dependencies["oneOf"]; !ok {
		t.Fatalf("expected dependencies schema to document string and array compatibility")
	}
}

func assertStatsSchemaDocumentsDashboardFields(t *testing.T, schema any) {
	t.Helper()
	stats, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("expected Stats schema to be an object, got %#v", schema)
	}
	properties, ok := stats["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected Stats schema to include properties")
	}
	for _, field := range []string{"hourly", "project_ai"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("expected Stats schema to document %q", field)
		}
	}
	aiRef, ok := properties["ai"].(map[string]any)
	if !ok {
		t.Fatalf("expected Stats.ai to be a schema ref")
	}
	if got, _ := aiRef["$ref"].(string); got != "#/components/schemas/AIMetrics" {
		t.Fatalf("expected Stats.ai to reference AIMetrics, got %q", got)
	}
}

func assertProjectSchemaDocumentsPublicMetadata(t *testing.T, schema any) {
	t.Helper()
	project, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("expected Project schema to be an object, got %#v", schema)
	}
	properties, ok := project["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected Project schema to include properties")
	}
	for _, field := range []string{"id", "name", "color", "has_public_url", "badge", "first_heartbeat_at", "last_heartbeat_at", "created_at"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("expected Project schema to document %q", field)
		}
	}
	publicURL, ok := properties["has_public_url"].(map[string]any)
	if !ok {
		t.Fatalf("expected Project.has_public_url schema to be an object")
	}
	if got, _ := publicURL["type"].(string); got != "boolean" {
		t.Fatalf("expected Project.has_public_url to be boolean, got %q", got)
	}
}

func assertCommitSummarySchemaDocumentsLinks(t *testing.T, schema any) {
	t.Helper()
	commit, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("expected CommitSummary schema to be an object, got %#v", schema)
	}
	properties, ok := commit["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected CommitSummary schema to include properties")
	}
	for _, field := range []string{"id", "hash", "truncated_hash", "branch", "ref", "total_seconds", "human_readable_total", "human_readable_total_with_seconds", "created_at", "author_date", "committer_date", "html_url", "url"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("expected CommitSummary schema to document %q", field)
		}
	}
}

func assertAIMetricsSchemaDocumentsDashboardFields(t *testing.T, schema any) {
	t.Helper()
	metrics, ok := schema.(map[string]any)
	if !ok {
		t.Fatalf("expected AIMetrics schema to be an object, got %#v", schema)
	}
	properties, ok := metrics["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected AIMetrics schema to include properties")
	}
	for _, field := range []string{"session_count", "estimated_cost_cents", "agents", "days", "costs"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("expected AIMetrics schema to document %q", field)
		}
	}
}
