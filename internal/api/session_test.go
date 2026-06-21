package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestSessionTokenFromCookieReadsCurrentSessionCookie(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()

	token, ok := sessionTokenFromCookie(e.NewContext(req, rec))

	if !ok {
		t.Fatal("expected session token to be found")
	}
	if token != "session-token" {
		t.Fatalf("expected session token, got %q", token)
	}
}

func TestClearSessionCookiesExpiresSessionAndJWTCookies(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	clearSessionCookies(c)

	cookies := rec.Result().Cookies()
	assertExpiredCookie(t, cookies, sessionCookieName)
	assertExpiredCookie(t, cookies, sessionJWTCookieName)
}

func assertExpiredCookie(t *testing.T, cookies []*http.Cookie, name string) {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == name {
			if cookie.Value != "" {
				t.Fatalf("expected %s cookie value to be empty, got %q", name, cookie.Value)
			}
			if cookie.MaxAge != -1 {
				t.Fatalf("expected %s cookie MaxAge=-1, got %d", name, cookie.MaxAge)
			}
			if !cookie.HttpOnly {
				t.Fatalf("expected %s cookie to be HttpOnly", name)
			}
			if cookie.Path != "/" {
				t.Fatalf("expected %s cookie path /, got %q", name, cookie.Path)
			}
			return
		}
	}
	t.Fatalf("expected expired %s cookie, got %#v", name, cookies)
}
