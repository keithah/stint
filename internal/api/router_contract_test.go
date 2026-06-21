package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/keithah/stint/internal/services"
	"github.com/labstack/echo/v4"
)

func TestStatsResponseStatusUsesAcceptedForStaleCache(t *testing.T) {
	if got := statsResponseStatus(services.Stats{IsUpToDate: false}); got != http.StatusAccepted {
		t.Fatalf("expected stale stats to return 202, got %d", got)
	}
	if got := statsResponseStatus(services.Stats{IsUpToDate: true}); got != http.StatusOK {
		t.Fatalf("expected up-to-date stats to return 200, got %d", got)
	}
}

func TestRouteRateLimitConstantsMatchSpec(t *testing.T) {
	if heartbeatIngestionRateLimit != 1000 {
		t.Fatalf("expected heartbeat ingestion limit 1000/min, got %d", heartbeatIngestionRateLimit)
	}
	if authenticatedReadRateLimit != 60 {
		t.Fatalf("expected authenticated read limit 60/min, got %d", authenticatedReadRateLimit)
	}
	if oauthTokenCreationRateLimit != 10 {
		t.Fatalf("expected OAuth token creation limit 10/hour, got %d", oauthTokenCreationRateLimit)
	}
}

func TestAuthContextRateLimitSubjectUsesCredentialIdentity(t *testing.T) {
	userID := uuid.New()

	apiKey := authContext{Kind: "api_key", Subject: "key-1"}
	if got := apiKey.RateLimitSubject(userID); got != "api_key:key-1" {
		t.Fatalf("expected API key subject, got %q", got)
	}

	session := authContext{Kind: "session"}
	if got := session.RateLimitSubject(userID); got != "session:"+userID.String() {
		t.Fatalf("expected session to fall back to user id, got %q", got)
	}
}

func TestOAuthTokenUserRateLimitKeyUsesUserID(t *testing.T) {
	userID := uuid.New()

	if got := oauthTokenUserRateLimitKey(userID); got != "oauth-token:user:"+userID.String() {
		t.Fatalf("expected OAuth token rate limit key to use user id, got %q", got)
	}
}

func TestOAuthRevokeRequiresClientCredentialsBeforeTokenValidation(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/oauth/revoke", strings.NewReader(""))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()

	err := (&Server{}).oauthRevoke(e.NewContext(req, rec))
	if err != nil {
		t.Fatalf("oauthRevoke returned error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing client credentials to return 401, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"error":"invalid_client"`) {
		t.Fatalf("expected invalid_client OAuth error, got %s", body)
	}
}

func TestOAuthRevokeOnlyIgnoresUnknownTokenErrors(t *testing.T) {
	sourceBytes, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatalf("could not read router.go: %v", err)
	}
	body := apiFunctionSource(string(sourceBytes), "oauthRevoke")
	for _, want := range []string{
		"errors.Is(err, pgx.ErrNoRows)",
		"oauthError(\"server_error\")",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("oauthRevoke must ignore only unknown-token revocation errors and report server failures; missing %q", want)
		}
	}
}

func TestHeartbeatPurgeModeUsesPerUserRetentionWhenNoGlobalOverride(t *testing.T) {
	if mode := heartbeatPurgeMode(0); mode != heartbeatPurgePerUser {
		t.Fatalf("expected zero retention override to use per-user retention, got %q", mode)
	}
	if mode := heartbeatPurgeMode(365); mode != heartbeatPurgeGlobal {
		t.Fatalf("expected positive retention override to use global purge, got %q", mode)
	}
}

func apiFunctionSource(source, name string) string {
	start := strings.Index(source, "func (s *Server) "+name)
	if start < 0 {
		return ""
	}
	next := strings.Index(source[start+1:], "\nfunc ")
	if next < 0 {
		return source[start:]
	}
	return source[start : start+1+next]
}

func TestRegistrationAllowsNewUser(t *testing.T) {
	tests := []struct {
		name         string
		enabled      bool
		maxUsers     int
		currentUsers int
		want         bool
	}{
		{name: "enabled unlimited", enabled: true, maxUsers: 0, currentUsers: 100, want: true},
		{name: "disabled", enabled: false, maxUsers: 0, currentUsers: 0, want: false},
		{name: "below cap", enabled: true, maxUsers: 2, currentUsers: 1, want: true},
		{name: "at cap", enabled: true, maxUsers: 2, currentUsers: 2, want: false},
		{name: "above cap", enabled: true, maxUsers: 2, currentUsers: 3, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := registrationAllowsNewUser(tt.enabled, tt.maxUsers, tt.currentUsers); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestValidateCustomRulesAcceptsRegexOperation(t *testing.T) {
	err := validateCustomRules([]services.CustomRule{
		{
			Action:      "change",
			Source:      "entity",
			Operation:   "regex",
			SourceValue: `rule-smoke/.+\.go$`,
			Destinations: []services.CustomRuleDestination{
				{Destination: "project", DestinationValue: "rewritten"},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected valid regex custom rule, got %v", err)
	}
}

func TestValidateCustomRulesRejectsInvalidRegex(t *testing.T) {
	err := validateCustomRules([]services.CustomRule{
		{
			Action:      "change",
			Source:      "entity",
			Operation:   "regex",
			SourceValue: `[`,
			Destinations: []services.CustomRuleDestination{
				{Destination: "project", DestinationValue: "rewritten"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected invalid regex custom rule to be rejected")
	}
}

func TestValidateCustomRulesRejectsUnknownOperation(t *testing.T) {
	err := validateCustomRules([]services.CustomRule{
		{
			Action:      "change",
			Source:      "entity",
			Operation:   "glob",
			SourceValue: "*.go",
			Destinations: []services.CustomRuleDestination{
				{Destination: "project", DestinationValue: "rewritten"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected unknown custom rule operation to be rejected")
	}
}
