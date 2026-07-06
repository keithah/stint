package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/keithah/stint/internal/config"
	"github.com/keithah/stint/internal/db"
	"github.com/labstack/echo/v4"
	"golang.org/x/oauth2"
)

func TestGitHubOAuthStateRoundTrip(t *testing.T) {
	state, signed, err := newGitHubOAuthState("state-secret")
	if err != nil {
		t.Fatalf("newGitHubOAuthState returned error: %v", err)
	}
	if state == "" {
		t.Fatal("expected non-empty state")
	}
	if signed == state {
		t.Fatal("expected signed state cookie to differ from public state")
	}
	if !verifyGitHubOAuthState(state, signed, "state-secret") {
		t.Fatal("expected signed state to verify")
	}
}

func TestGitHubOAuthStateRejectsTampering(t *testing.T) {
	state, signed, err := newGitHubOAuthState("state-secret")
	if err != nil {
		t.Fatalf("newGitHubOAuthState returned error: %v", err)
	}
	if verifyGitHubOAuthState(state+"x", signed, "state-secret") {
		t.Fatal("expected tampered public state to be rejected")
	}
	if verifyGitHubOAuthState(state, signed+"x", "state-secret") {
		t.Fatal("expected tampered signed state to be rejected")
	}
	if verifyGitHubOAuthState(state, signed, "wrong-secret") {
		t.Fatal("expected wrong secret to be rejected")
	}
}

func TestGitHubLoginSetsSignedStateCookie(t *testing.T) {
	server := &Server{
		Config: config.Config{
			BaseURL:            "http://api.example.test",
			GitHubClientID:     "client",
			GitHubClientSecret: "secret",
			SessionSecret:      "state-secret",
		},
		OAuth: &oauth2.Config{
			ClientID:    "client",
			RedirectURL: "http://api.example.test/auth/github/callback",
			Endpoint: oauth2.Endpoint{
				AuthURL: "https://github.example.test/login/oauth/authorize",
			},
		},
	}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/auth/github/login", nil)
	rec := httptest.NewRecorder()

	if err := server.githubLogin(e.NewContext(req, rec)); err != nil {
		t.Fatalf("githubLogin returned error: %v", err)
	}

	location := rec.Header().Get(echo.HeaderLocation)
	if rec.Code != http.StatusFound || !strings.Contains(location, "state=") {
		t.Fatalf("expected redirect with OAuth state, code=%d location=%q", rec.Code, location)
	}
	if cookies := rec.Result().Cookies(); !hasCookie(cookies, githubOAuthStateCookieName) {
		t.Fatalf("expected %s cookie, got %#v", githubOAuthStateCookieName, cookies)
	}
}

func TestGitHubCallbackRejectsMissingStateCookie(t *testing.T) {
	server := &Server{Config: config.Config{SessionSecret: "state-secret"}}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=abc&state=bad", nil)
	rec := httptest.NewRecorder()

	if err := server.githubCallback(e.NewContext(req, rec)); err != nil {
		t.Fatalf("githubCallback returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for missing state cookie, got %d", rec.Code)
	}
}

func TestGitHubCallbackRejectsMissingCode(t *testing.T) {
	server := &Server{Config: config.Config{SessionSecret: "state-secret"}}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?state=state", nil)
	rec := httptest.NewRecorder()

	if err := server.githubCallback(e.NewContext(req, rec)); err != nil {
		t.Fatalf("githubCallback returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for missing code, got %d", rec.Code)
	}
}

func TestGitHubCallbackCreatesUserAndSession(t *testing.T) {
	ctx := context.Background()
	store := openTestPostgresStore(t, ctx)
	secret := "github-callback-session-secret-with-enough-bytes"
	server := &Server{
		Config: config.Config{
			BaseURL:            "http://api.example.test",
			WebBaseURL:         "http://web.example.test",
			SessionSecret:      secret,
			EnableRegistration: true,
		},
		Store: store,
		OAuth: &oauth2.Config{
			ClientID:     "client",
			ClientSecret: "secret",
			Endpoint: oauth2.Endpoint{
				TokenURL: "https://github.example.test/login/oauth/access_token",
			},
			RedirectURL: "http://api.example.test/auth/github/callback",
		},
	}
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://github.example.test/login/oauth/access_token":
			return testHTTPResponse(req, http.StatusOK, "application/x-www-form-urlencoded", "access_token=gho_test&token_type=bearer"), nil
		case "https://api.github.com/user":
			return testHTTPResponse(req, http.StatusOK, "application/json", `{"id":9001,"login":"github-owner","email":"","name":"GitHub Owner","avatar_url":"https://avatars.example.test/u/9001"}`), nil
		case "https://api.github.com/user/emails":
			return testHTTPResponse(req, http.StatusOK, "application/json", `[{"email":"owner@example.test","primary":true,"verified":true}]`), nil
		default:
			return testHTTPResponse(req, http.StatusNotFound, "text/plain", "not found"), nil
		}
	})}

	state := "callback-state"
	req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=valid-code&state="+state, nil)
	req = req.WithContext(context.WithValue(req.Context(), oauth2.HTTPClient, httpClient))
	req.AddCookie(&http.Cookie{Name: githubOAuthStateCookieName, Value: signedGitHubOAuthState(state, secret)})
	rec := httptest.NewRecorder()

	if err := server.githubCallback(echo.New().NewContext(req, rec)); err != nil {
		t.Fatalf("githubCallback returned error: %v", err)
	}
	if rec.Code != http.StatusFound || rec.Header().Get(echo.HeaderLocation) != "http://web.example.test/dashboard" {
		t.Fatalf("expected dashboard redirect, code=%d location=%q body=%s", rec.Code, rec.Header().Get(echo.HeaderLocation), rec.Body.String())
	}
	if !hasCookie(rec.Result().Cookies(), sessionCookieName) || !hasCookie(rec.Result().Cookies(), sessionJWTCookieName) {
		t.Fatalf("expected session cookies, got %#v", rec.Result().Cookies())
	}
	user, err := store.UserByGitHubID(ctx, 9001)
	if err != nil {
		t.Fatalf("expected GitHub user to be persisted: %v", err)
	}
	if user.GitHubUsername != "github-owner" || user.Email != "owner@example.test" {
		t.Fatalf("unexpected persisted GitHub profile: %#v", user)
	}
}

func TestGitHubPublicRepoNamesReturnsRepositoryNames(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://github.example.test/users/keith/repos?per_page=100&type=public" {
			return testHTTPResponse(req, http.StatusNotFound, "text/plain", "not found"), nil
		}
		return testHTTPResponse(req, http.StatusOK, "application/json", `[
			{"name":"stint","private":false},
			{"name":"secret","private":true},
			{"name":"agent-tools","private":false}
		]`), nil
	})}

	names, err := githubPublicRepoNames(context.Background(), client, "https://github.example.test", "Keith")
	if err != nil {
		t.Fatalf("githubPublicRepoNames returned error: %v", err)
	}
	if !names["stint"] || !names["agent-tools"] {
		t.Fatalf("expected public repo names, got %#v", names)
	}
	if names["secret"] {
		t.Fatalf("private repo should not be allowed: %#v", names)
	}
}

func TestEnsureGitHubRegistrationAllowedHonorsLimits(t *testing.T) {
	ctx := context.Background()
	store := openTestPostgresStore(t, ctx)
	if _, err := store.UpsertGitHubUser(ctx, db.GitHubProfile{ID: 9100, Username: "existing"}); err != nil {
		t.Fatalf("create existing user: %v", err)
	}

	server := &Server{Store: store, Config: config.Config{EnableRegistration: false}}
	if err := server.ensureGitHubRegistrationAllowed(ctx, 9100); err != nil {
		t.Fatalf("existing users should bypass closed registration: %v", err)
	}
	if err := server.ensureGitHubRegistrationAllowed(ctx, 9101); err != errRegistrationClosed {
		t.Fatalf("expected closed registration error, got %v", err)
	}

	server.Config.EnableRegistration = true
	server.Config.MaxUsers = 1
	if err := server.ensureGitHubRegistrationAllowed(ctx, 9101); err != errMaxUsersReached {
		t.Fatalf("expected max users error, got %v", err)
	}
}

func hasCookie(cookies []*http.Cookie, name string) bool {
	for _, cookie := range cookies {
		if cookie.Name == name && cookie.Value != "" {
			return true
		}
	}
	return false
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func testHTTPResponse(req *http.Request, status int, contentType, body string) *http.Response {
	header := http.Header{}
	header.Set("Content-Type", contentType)
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func TestPrimaryGitHubEmailPrefersExistingProfileEmail(t *testing.T) {
	got := primaryGitHubEmail("profile@example.com", []githubEmail{
		{Email: "primary@example.com", Primary: true, Verified: true},
	})

	if got != "profile@example.com" {
		t.Fatalf("expected profile email, got %q", got)
	}
}

func TestPrimaryGitHubEmailUsesPrimaryVerifiedEmail(t *testing.T) {
	got := primaryGitHubEmail("", []githubEmail{
		{Email: "unverified@example.com", Primary: true, Verified: false},
		{Email: "secondary@example.com", Primary: false, Verified: true},
		{Email: "primary@example.com", Primary: true, Verified: true},
	})

	if got != "primary@example.com" {
		t.Fatalf("expected primary verified email, got %q", got)
	}
}

func TestPrimaryGitHubEmailFallsBackToAnyVerifiedEmail(t *testing.T) {
	got := primaryGitHubEmail("", []githubEmail{
		{Email: "unverified@example.com", Primary: true, Verified: false},
		{Email: "verified@example.com", Primary: false, Verified: true},
	})

	if got != "verified@example.com" {
		t.Fatalf("expected verified email, got %q", got)
	}
}
