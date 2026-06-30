package api

import (
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/services"
)

func TestOptimizerRouteWiring(t *testing.T) {
	source, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatalf("read router.go: %v", err)
	}
	text := string(source)
	assertContains(t, text, `e.POST("/oauth/revoke", server.oauthRevoke, server.rateLimitOAuthToken(oauthTokenCreationRateLimit, time.Hour))`)
	assertContains(t, text, `current.PUT("", server.updateCurrentUser, requireLocalAccountAccess, writeLimit("user-settings"))`)
	assertContains(t, text, `current.DELETE("", server.deleteCurrentUser, requireLocalAccountAccess, writeLimit("user-account"))`)
	assertContains(t, text, `setPublicMetadataCache(c)`)
	assertContains(t, text, `context.WithTimeout(c.Request().Context(), githubOAuthRequestTimeout)`)
	assertContains(t, text, `e.IPExtractor = echo.ExtractIPDirect()`)
	assertContains(t, text, `middleware.GzipWithConfig(middleware.GzipConfig{Level: gzip.BestSpeed})`)
	assertContains(t, text, `e.GET("/auth/github/login", server.githubLogin, server.rateLimitIP("github-oauth-login", 20, time.Minute))`)
	assertContains(t, text, `e.GET("/auth/github/callback", server.githubCallback, server.rateLimitIP("github-oauth-callback", 20, time.Minute))`)
	assertContains(t, text, `api.GET("/docs", server.openAPIDocs, server.rateLimitIP("openapi-docs", 30, time.Minute))`)
	assertContains(t, text, `s.openAPIDocsOnce.Do`)
	assertContains(t, text, `setPublicMetadataCache(c)`)
	assertContains(t, text, `api.POST("/dev/jobs/heartbeats-purge", server.devHeartbeatsPurge, server.rateLimitIP("dev-jobs", 30, time.Minute))`)
	assertContains(t, text, `s.Store.UpsertExternalDurations(c.Request().Context(), user.ID, validInputs)`)
	assertContains(t, text, `current.GET("/events", server.currentUserEvents, readLimit)`)
	assertContains(t, text, `middleware.BodyLimit(usageEventsBulkJSONBodyLimit)`)
	assertContains(t, text, `usageEventsBulkJSONBodyLimit       = "25M"`)
	assertContains(t, text, `current.DELETE("/custom_rules_progress", server.abortCustomRulesProgress, requireLocalAccountAccess, writeLimit("custom-rules-progress"))`)
	assertContains(t, functionSource(text, "writePublicPayload"), `Cache-Control", "public, max-age=30, stale-while-revalidate=300"`)
	assertContains(t, functionSource(text, "NewRouter"), `Redis rate limiter unavailable`)
	assertContains(t, functionSource(text, "NewRouter"), `Redis status cache unavailable`)
	assertContains(t, functionSource(text, "NewRouter"), `Redis leaderboard cache unavailable`)
	assertContains(t, functionSource(text, "NewRouter"), `Redis job client unavailable`)
	if strings.Contains(functionSource(text, "fileExperts"), "AllHeartbeats") {
		t.Fatal("fileExperts should use an entity-scoped heartbeat query")
	}
	if strings.Contains(functionSource(text, "projectCommitRows"), "AllHeartbeats") {
		t.Fatal("projectCommitRows should use a project-scoped heartbeat query")
	}
	if strings.Contains(text, `duration, err := s.Store.UpsertExternalDuration(c.Request().Context(), user.ID, input)`) &&
		strings.Contains(functionSource(text, "createExternalDurationsBulk"), "UpsertExternalDuration(") {
		t.Fatal("createExternalDurationsBulk should use the batched store method")
	}
}

func TestGitHubOAuthOnlyFetchesEmailsWhenProfileEmailMissing(t *testing.T) {
	source, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatalf("read router.go: %v", err)
	}
	body := functionSource(string(source), "githubCallback")
	if !strings.Contains(body, `if strings.TrimSpace(gh.Email) == "" {`) {
		t.Fatal("githubCallback should skip /user/emails when /user already returned an email")
	}
}

func TestHeartbeatDumpDownloadDoesNotStreamAfterHeaders(t *testing.T) {
	source, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatalf("read router.go: %v", err)
	}
	text := string(source)
	if strings.Contains(text, "WriteHeader(http.StatusOK)") && strings.Contains(text, "ForEachHeartbeatForExport") {
		t.Fatal("heartbeat dump fallback should materialize before writing headers to avoid truncated 200 responses")
	}
}

func TestJobEventsAreIsolatedFromRouter(t *testing.T) {
	source, err := os.ReadFile("job_events.go")
	if err != nil {
		t.Fatalf("read job_events.go: %v", err)
	}
	text := string(source)
	assertContains(t, text, `writeEvent("data_dumps"`)
	assertContains(t, text, `writeEvent("custom_rules_progress"`)
}

func TestOptimizerAvoidsTemplateJSONAccessLogger(t *testing.T) {
	source, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatalf("read router.go: %v", err)
	}
	if strings.Contains(string(source), "middleware.LoggerWithConfig") {
		t.Fatal("access logging should use structured RequestLogger, not template JSON interpolation")
	}
	body := functionSource(string(source), "structuredRequestLogger")
	if strings.Contains(body, "map[string]any") {
		t.Fatal("access logging should avoid per-request map allocations")
	}
}

func TestPreparedCustomRulesCacheCapsEntries(t *testing.T) {
	server := &Server{customRulesCache: map[uuid.UUID]services.PreparedCustomRules{}}

	for i := 0; i < maxPreparedCustomRulesCacheEntries+10; i++ {
		server.cachePreparedCustomRules(uuid.New(), services.PreparedCustomRules{})
	}

	if got := len(server.customRulesCache); got > maxPreparedCustomRulesCacheEntries {
		t.Fatalf("custom rules cache entries = %d, want at most %d", got, maxPreparedCustomRulesCacheEntries)
	}
}

func assertContains(t *testing.T, text, needle string) {
	t.Helper()
	if !strings.Contains(text, needle) {
		t.Fatalf("expected router.go to contain %q", needle)
	}
}

func functionSource(source, name string) string {
	start := strings.Index(source, "func "+name)
	if start == -1 {
		start = strings.Index(source, "func (s *Server) "+name)
	}
	if start == -1 {
		return ""
	}
	next := strings.Index(source[start+1:], "\nfunc ")
	if next == -1 {
		return source[start:]
	}
	return source[start : start+1+next]
}
