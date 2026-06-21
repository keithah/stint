package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/services"
	"github.com/labstack/echo/v4"
)

func TestRequireScopeRejectsMissingOAuthScope(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/current/heartbeats", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", db.User{ID: uuid.New()})
	c.Set("auth", authContext{Kind: "oauth", Scopes: []string{"read_stats"}})

	called := false
	err := requireScope("write_heartbeats")(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})(c)
	if err != nil {
		t.Fatalf("requireScope returned error: %v", err)
	}
	if called {
		t.Fatal("expected handler not to be called")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestRequireScopeAllowsSessionWithoutExplicitScopes(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/current/heartbeats", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", db.User{ID: uuid.New()})
	c.Set("auth", authContext{Kind: "session"})

	called := false
	err := requireScope("write_heartbeats")(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})(c)
	if err != nil {
		t.Fatalf("requireScope returned error: %v", err)
	}
	if !called {
		t.Fatal("expected handler to be called")
	}
}

func TestRequireScopeRejectsAPIKeyMissingScope(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/current/heartbeats", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", db.User{ID: uuid.New()})
	c.Set("auth", authContext{Kind: "api_key", Scopes: []string{"read_stats"}})

	called := false
	err := requireScope("write_heartbeats")(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})(c)
	if err != nil {
		t.Fatalf("requireScope returned error: %v", err)
	}
	if called {
		t.Fatal("expected handler not to be called")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestRequireLocalAccountAccessRejectsScopedAPIKey(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api_keys", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("auth", authContext{Kind: "api_key", Scopes: []string{scopeReadStats}})

	called := false
	err := requireLocalAccountAccess(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})(c)
	if err != nil {
		t.Fatalf("requireLocalAccountAccess returned error: %v", err)
	}
	if called {
		t.Fatal("expected scoped API key not to reach local account handler")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestRequireLocalAccountAccessRejectsOAuthToken(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api_keys", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("auth", authContext{Kind: "oauth", Scopes: allAuthScopes()})

	called := false
	err := requireLocalAccountAccess(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})(c)
	if err != nil {
		t.Fatalf("requireLocalAccountAccess returned error: %v", err)
	}
	if called {
		t.Fatal("expected OAuth token not to reach local account handler")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestRequireLocalAccountAccessAllowsSessionJWTAndFullAPIKey(t *testing.T) {
	for _, authInfo := range []authContext{
		{Kind: "session"},
		{Kind: "jwt", Scopes: allAuthScopes()},
		{Kind: "api_key", Scopes: db.DefaultAPIKeyScopes()},
	} {
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/api_keys", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("auth", authInfo)

		called := false
		err := requireLocalAccountAccess(func(c echo.Context) error {
			called = true
			return c.NoContent(http.StatusOK)
		})(c)
		if err != nil {
			t.Fatalf("%s: requireLocalAccountAccess returned error: %v", authInfo.Kind, err)
		}
		if !called {
			t.Fatalf("%s: expected local account handler to be called", authInfo.Kind)
		}
	}
}

func TestAuthContextNarrowScopeDoesNotSatisfyBroadScope(t *testing.T) {
	authInfo := authContext{Kind: "oauth", Scopes: []string{"read_stats.projects"}}

	if authInfo.HasScope("read_stats") {
		t.Fatal("expected read_stats.projects not to grant full read_stats")
	}
}

func TestAuthContextBroadScopeSatisfiesGranularScope(t *testing.T) {
	authInfo := authContext{Kind: "oauth", Scopes: []string{"read_stats"}}

	if !authInfo.HasScope("read_stats.projects") {
		t.Fatal("expected read_stats to grant read_stats.projects")
	}
}

func TestScopeForInsightTypeUsesGranularStatsScopes(t *testing.T) {
	tests := map[string]string{
		"best_day":          scopeReadStatsBestDay,
		"categories":        scopeReadStatsCategories,
		"dependencies":      scopeReadStatsDependencies,
		"editors":           scopeReadStatsEditors,
		"languages":         scopeReadStatsLanguages,
		"machines":          scopeReadStatsMachines,
		"operating_systems": scopeReadStatsOperatingSystems,
		"projects":          scopeReadStatsProjects,
	}

	for insightType, expected := range tests {
		if got := scopeForInsightType(insightType); got != expected {
			t.Fatalf("%s: expected %q, got %q", insightType, expected, got)
		}
	}
}

func TestScopeForInsightTypeFallsBackToFullStats(t *testing.T) {
	for _, insightType := range []string{"stats", "days", "hours", "weekdays", "daily_average", "daily_average_trend", "ai_days", "ai_agents", "unknown"} {
		if got := scopeForInsightType(insightType); got != scopeReadStats {
			t.Fatalf("%s: expected %q, got %q", insightType, scopeReadStats, got)
		}
	}
}

func TestRequireInsightScopeAllowsMatchingGranularScope(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/current/insights/projects/last_30_days", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("insight_type", "range")
	c.SetParamValues("projects", "last_30_days")
	c.Set("user", db.User{ID: uuid.New()})
	c.Set("auth", authContext{Kind: "oauth", Scopes: []string{scopeReadStatsProjects}})

	called := false
	err := requireInsightScope(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})(c)
	if err != nil {
		t.Fatalf("requireInsightScope returned error: %v", err)
	}
	if !called {
		t.Fatal("expected handler to be called")
	}
}

func TestRequireInsightScopeRejectsGranularScopeForFullStats(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/current/insights/stats/last_30_days", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("insight_type", "range")
	c.SetParamValues("stats", "last_30_days")
	c.Set("user", db.User{ID: uuid.New()})
	c.Set("auth", authContext{Kind: "oauth", Scopes: []string{scopeReadStatsProjects}})

	called := false
	err := requireInsightScope(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})(c)
	if err != nil {
		t.Fatalf("requireInsightScope returned error: %v", err)
	}
	if called {
		t.Fatal("expected handler not to be called")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestSummaryScopeForSliceByUsesGranularSummaryScopes(t *testing.T) {
	tests := map[string]string{
		"":                 scopeReadSummariesProjects,
		"project":          scopeReadSummariesProjects,
		"language":         scopeReadSummariesLanguages,
		"editor":           scopeReadSummariesEditors,
		"machine":          scopeReadSummariesMachines,
		"operating_system": scopeReadSummariesOperatingSystems,
		"category":         scopeReadSummariesCategories,
		"dependencies":     scopeReadSummariesDependencies,
	}

	for sliceBy, expected := range tests {
		if got := summaryScopeForSliceBy(sliceBy); got != expected {
			t.Fatalf("%s: expected %q, got %q", sliceBy, expected, got)
		}
	}
}

func TestRequireSummarySliceScopeAllowsMatchingGranularScope(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/current/durations?slice_by=language", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("auth", authContext{Kind: "oauth", Scopes: []string{scopeReadSummariesLanguages}})

	called := false
	err := requireSummarySliceScope(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})(c)
	if err != nil {
		t.Fatalf("requireSummarySliceScope returned error: %v", err)
	}
	if !called {
		t.Fatal("expected handler to be called")
	}
}

func TestRequireSummarySliceScopeRejectsMismatchedGranularScope(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/current/durations?slice_by=language", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("auth", authContext{Kind: "oauth", Scopes: []string{scopeReadSummariesProjects}})

	called := false
	err := requireSummarySliceScope(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})(c)
	if err != nil {
		t.Fatalf("requireSummarySliceScope returned error: %v", err)
	}
	if called {
		t.Fatal("expected handler not to be called")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestSummaryFieldsForAuthReflectGranularScopes(t *testing.T) {
	fields := summaryFieldsForAuth(authContext{Kind: "oauth", Scopes: []string{scopeReadSummariesProjects}})
	if !fields.Projects || fields.Languages || fields.Categories || fields.Dependencies || fields.Editors || fields.Machines || fields.OperatingSystems {
		t.Fatalf("expected project-only summary fields, got %#v", fields)
	}

	fields = summaryFieldsForAuth(authContext{Kind: "oauth", Scopes: []string{scopeReadSummariesLanguages}})
	if fields.Projects || !fields.Languages || fields.Categories || fields.Dependencies || fields.Editors || fields.Machines || fields.OperatingSystems {
		t.Fatalf("expected language-only summary fields, got %#v", fields)
	}

	fields = summaryFieldsForAuth(authContext{Kind: "oauth", Scopes: []string{scopeReadSummariesCategories, scopeReadSummariesDependencies, scopeReadSummariesEditors, scopeReadSummariesMachines, scopeReadSummariesOperatingSystems}})
	if fields.Projects || fields.Languages || !fields.Categories || !fields.Dependencies || !fields.Editors || !fields.Machines || !fields.OperatingSystems {
		t.Fatalf("expected requested granular summary fields, got %#v", fields)
	}

	fields = summaryFieldsForAuth(authContext{Kind: "oauth", Scopes: []string{scopeReadSummaries}})
	if !fields.Projects || !fields.Languages || !fields.Categories || !fields.Dependencies || !fields.Editors || !fields.Machines || !fields.OperatingSystems {
		t.Fatalf("expected broad summary scope to include all fields, got %#v", fields)
	}
}

func TestOAuthAppBroadScopeAllowsGranularRequest(t *testing.T) {
	if !oauthAppAllowsScope([]string{"read_stats"}, "read_stats.projects") {
		t.Fatal("expected app read_stats grant to allow read_stats.projects request")
	}
}

func TestOAuthAppGranularScopeDoesNotAllowBroadRequest(t *testing.T) {
	if oauthAppAllowsScope([]string{"read_stats.projects"}, "read_stats") {
		t.Fatal("expected app read_stats.projects grant not to allow read_stats request")
	}
}

func TestOAuthTokenFragmentOmitsRefreshToken(t *testing.T) {
	userID := uuid.New()
	fragment := oauthTokenFragment(db.OAuthTokenResult{
		User:         db.User{ID: userID},
		AccessToken:  "waka_tok_example",
		RefreshToken: "stintr_should_not_be_returned",
		ExpiresAt:    time.Now().Add(12 * time.Hour),
		ExpiresIn:    43200,
		Scopes:       []string{"read_stats.projects"},
	}, "state-123")

	if fragment["access_token"] != "waka_tok_example" {
		t.Fatalf("unexpected access token %q", fragment["access_token"])
	}
	if _, ok := fragment["refresh_token"]; ok {
		t.Fatal("implicit OAuth fragment must not include refresh_token")
	}
	if fragment["token_type"] != "Bearer" {
		t.Fatalf("unexpected token type %q", fragment["token_type"])
	}
	if fragment["expires_in"] != "43200" {
		t.Fatalf("unexpected expires_in %q", fragment["expires_in"])
	}
	if fragment["scope"] != "read_stats.projects" {
		t.Fatalf("unexpected scope %q", fragment["scope"])
	}
	if fragment["state"] != "state-123" {
		t.Fatalf("unexpected state %q", fragment["state"])
	}
	if fragment["uid"] != userID.String() {
		t.Fatalf("unexpected uid %q", fragment["uid"])
	}
}

func TestAllAuthScopesIncludesPrivateResourceScopes(t *testing.T) {
	scopes := map[string]bool{}
	for _, scope := range allAuthScopes() {
		scopes[scope] = true
	}
	for _, scope := range []string{scopeReadGoals, scopeReadPrivateLeaderboards, scopeWritePrivateLeaderboards} {
		if !scopes[scope] {
			t.Fatalf("expected %s to be advertised as an OAuth scope", scope)
		}
	}
}

func TestCurrentUserRedactsEmailWithoutEmailScope(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/current", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", db.User{ID: uuid.New(), GitHubUsername: "scoped", Email: "scoped@example.test", Timezone: "UTC", TimeoutMinutes: 15})
	c.Set("auth", authContext{Kind: "oauth", Scopes: []string{scopeReadStats}})

	if err := (&Server{}).currentUser(c); err != nil {
		t.Fatalf("currentUser returned error: %v", err)
	}

	data := decodeCurrentUserData(t, rec.Body.Bytes())
	if _, ok := data["email"]; ok {
		t.Fatal("expected email to be redacted without email scope")
	}
}

func TestCurrentUserIncludesEmailWithEmailScope(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/current", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", db.User{ID: uuid.New(), GitHubUsername: "scoped", Email: "scoped@example.test", Timezone: "UTC", TimeoutMinutes: 15})
	c.Set("auth", authContext{Kind: "api_key", Scopes: []string{"email"}})

	if err := (&Server{}).currentUser(c); err != nil {
		t.Fatalf("currentUser returned error: %v", err)
	}

	data := decodeCurrentUserData(t, rec.Body.Bytes())
	if data["email"] != "scoped@example.test" {
		t.Fatalf("expected email with email scope, got %#v", data["email"])
	}
}

func TestCurrentUserIncludesEmailForSession(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/current", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user", db.User{ID: uuid.New(), GitHubUsername: "session", Email: "session@example.test", Timezone: "UTC", TimeoutMinutes: 15})
	c.Set("auth", authContext{Kind: "session"})

	if err := (&Server{}).currentUser(c); err != nil {
		t.Fatalf("currentUser returned error: %v", err)
	}

	data := decodeCurrentUserData(t, rec.Body.Bytes())
	if data["email"] != "session@example.test" {
		t.Fatalf("expected session email, got %#v", data["email"])
	}
}

func decodeCurrentUserData(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("failed to decode current user response: %v", err)
	}
	return envelope.Data
}

func TestLeaderboardLanguageSecondsMatchesCaseInsensitiveLanguage(t *testing.T) {
	stats := services.Stats{Languages: []services.SliceTotal{
		{Name: "Go", TotalSeconds: 120},
		{Name: "TypeScript", TotalSeconds: 60},
	}}

	seconds, ok := leaderboardLanguageSeconds(stats, "go")

	if !ok {
		t.Fatal("expected language to match")
	}
	if seconds != 120 {
		t.Fatalf("expected 120 seconds, got %d", seconds)
	}
}

func TestLeaderboardCountryMatchesCaseInsensitiveCountry(t *testing.T) {
	user := db.User{Country: "US"}

	if !leaderboardCountryMatches(user, "us") {
		t.Fatal("expected country to match case-insensitively")
	}
	if leaderboardCountryMatches(user, "ca") {
		t.Fatal("did not expect different country to match")
	}
	if !leaderboardCountryMatches(user, "") {
		t.Fatal("expected empty country filter to match all users")
	}
}

func TestLeaderboardCacheKeySeparatesLanguageAndCountryFilters(t *testing.T) {
	if got := leaderboardCacheKey("last_7_days", "", ""); got != "last_7_days" {
		t.Fatalf("unexpected unfiltered cache key %q", got)
	}
	if got := leaderboardCacheKey("last_7_days", "Go", ""); got != "last_7_days:language:go" {
		t.Fatalf("unexpected language cache key %q", got)
	}
	if got := leaderboardCacheKey("last_7_days", "", "us"); got != "last_7_days:country:us" {
		t.Fatalf("unexpected country cache key %q", got)
	}
	if got := leaderboardCacheKey("last_7_days", "Go", "us"); got != "last_7_days:language:go:country:us" {
		t.Fatalf("unexpected language and country cache key %q", got)
	}
}
