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
	if strings.Contains(text, `duration, err := s.Store.UpsertExternalDuration(c.Request().Context(), user.ID, input)`) &&
		strings.Contains(functionSource(text, "createExternalDurationsBulk"), "UpsertExternalDuration") {
		t.Fatal("createExternalDurationsBulk should use the batched store method")
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
		return ""
	}
	next := strings.Index(source[start+len("func "+name):], "\nfunc ")
	if next == -1 {
		return source[start:]
	}
	return source[start : start+len("func "+name)+next]
}
