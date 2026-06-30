package stintcli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestParseCommonAcceptsAPIKeyAlias(t *testing.T) {
	opts, err := parseCommon([]string{
		"--api-key", "waka_alias",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_alias" {
		t.Fatalf("APIKey = %q", opts.APIKey)
	}

	opts, err = parseCommon([]string{
		"--api-key", "waka_alias",
		"--key", "waka_key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_key" {
		t.Fatalf("--key should take precedence over --api-key, got %q", opts.APIKey)
	}
}

func TestRunShareTokensUseExpectedEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.EscapedPath() {
		case "GET /api/v1/users/current/share_tokens":
			_, _ = w.Write([]byte(`{"data":[{"id":"share-1","name":"review"}]}`))
		case "POST /api/v1/users/current/share_tokens":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["name"] != "team review" {
				t.Fatalf("unexpected payload: %#v", payload)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"id":"share-2","name":"team review"}}`))
		case "DELETE /api/v1/users/current/share_tokens/share%201":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"share-tokens", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id":"share-1"`) {
		t.Fatalf("list output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"share_tokens", "create", "team review", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name":"team review"`) {
		t.Fatalf("create output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"share-tokens", "delete", "share 1", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("delete should not print a 204 response body, got %q", out.String())
	}
}

func TestRunAccountMutationsUseExpectedEndpoints(t *testing.T) {
	var requests []string
	var updateBody map[string]any
	var deleteBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.EscapedPath())
		switch r.Method + " " + r.URL.EscapedPath() {
		case "PUT /api/v1/users/current":
			if err := json.NewDecoder(r.Body).Decode(&updateBody); err != nil {
				t.Fatalf("decode update body: %v", err)
			}
			_, _ = w.Write([]byte(`{"data":{"timezone":"America/Los_Angeles","writes_only":true}}`))
		case "DELETE /api/v1/users/current":
			if err := json.NewDecoder(r.Body).Decode(&deleteBody); err != nil {
				t.Fatalf("decode delete body: %v", err)
			}
			_, _ = w.Write([]byte(`{"data":{"deleted":true}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run(
		[]string{"account", "update", "--stdin", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"},
		strings.NewReader(`{"timezone":"America/Los_Angeles","writes_only":true}`),
		&out,
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if updateBody["timezone"] != "America/Los_Angeles" || updateBody["writes_only"] != true {
		t.Fatalf("unexpected account update body: %#v", updateBody)
	}
	if !strings.Contains(out.String(), `"writes_only":true`) {
		t.Fatalf("unexpected account update output: %q", out.String())
	}

	out.Reset()
	err = Run([]string{"account", "delete", "--confirm", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if deleteBody["confirmation"] != "DELETE" {
		t.Fatalf("unexpected account delete body: %#v", deleteBody)
	}
	if !strings.Contains(out.String(), `"deleted":true`) {
		t.Fatalf("unexpected account delete output: %q", out.String())
	}
	for _, want := range []string{
		"PUT /api/v1/users/current",
		"DELETE /api/v1/users/current",
	} {
		if !slices.Contains(requests, want) {
			t.Fatalf("expected request %s in %#v", want, requests)
		}
	}
}

func TestRunAccountDeleteRequiresConfirmation(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	err := Run([]string{"account", "delete", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--confirm") {
		t.Fatalf("expected confirmation error, got %v", err)
	}
	if called {
		t.Fatal("account delete should not call the server without --confirm")
	}
}

func TestRunAPIKeysUseExpectedEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.EscapedPath() {
		case "GET /api/v1/api_keys":
			_, _ = w.Write([]byte(`{"data":[{"id":"key-1","name":"default"}]}`))
		case "POST /api/v1/api_keys":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["name"] != "cli key" {
				t.Fatalf("unexpected key name: %#v", payload)
			}
			scopes, ok := payload["scopes"].([]any)
			if !ok || len(scopes) != 2 || scopes[0] != "write_heartbeats" || scopes[1] != "read_stats" {
				t.Fatalf("unexpected scopes: %#v", payload["scopes"])
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"api_key":"waka_new","key":{"id":"key-2","name":"cli key"}}}`))
		case "DELETE /api/v1/api_keys/key%201":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"api-keys", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id":"key-1"`) {
		t.Fatalf("list output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"api-keys", "create", "cli key", "--scope", "write_heartbeats", "--scope", "read_stats", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"api_key":"waka_new"`) {
		t.Fatalf("create output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"api-keys", "delete", "key 1", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("delete should not print a 204 response body, got %q", out.String())
	}
}

func TestRunOAuthAppsUseExpectedEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.EscapedPath() {
		case "GET /api/v1/oauth/apps":
			_, _ = w.Write([]byte(`{"data":[{"id":"app-1","name":"Local app"}]}`))
		case "POST /api/v1/oauth/apps":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["name"] != "CLI OAuth" {
				t.Fatalf("unexpected app name: %#v", payload)
			}
			redirects, ok := payload["redirect_uris"].([]any)
			if !ok || len(redirects) != 1 || redirects[0] != "http://localhost:3000/callback" {
				t.Fatalf("unexpected redirect_uris: %#v", payload["redirect_uris"])
			}
			scopes, ok := payload["scopes"].([]any)
			if !ok || len(scopes) != 1 || scopes[0] != "read_stats" {
				t.Fatalf("unexpected scopes: %#v", payload["scopes"])
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"id":"app-2","name":"CLI OAuth","client_id":"client-1"}}`))
		case "DELETE /api/v1/oauth/apps/app%201":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"oauth-apps", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id":"app-1"`) {
		t.Fatalf("list output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"oauth", "apps", "create", "CLI OAuth", "--redirect-uri", "http://localhost:3000/callback", "--scope", "read_stats", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"client_id":"client-1"`) {
		t.Fatalf("create output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"oauth-apps", "delete", "app 1", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("delete should not print a 204 response body, got %q", out.String())
	}
}

func TestRunOAuthTokenAndRevokeUseExpectedEndpoints(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.EscapedPath())
		if got := r.Header.Get("Authorization"); got != basicAuthHeader("client-1:secret-1") {
			t.Fatalf("Authorization = %q, want OAuth client basic auth", got)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/x-www-form-urlencoded") {
			t.Fatalf("Content-Type = %q, want form", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		switch r.Method + " " + r.URL.EscapedPath() {
		case "POST /oauth/token":
			switch r.Form.Get("grant_type") {
			case "authorization_code":
				if r.Form.Get("code") != "auth-code" || r.Form.Get("redirect_uri") != "http://localhost/callback" {
					t.Fatalf("unexpected authorization_code form: %#v", r.Form)
				}
				_, _ = w.Write([]byte(`{"access_token":"access-1","refresh_token":"refresh-1","token_type":"Bearer"}`))
			case "refresh_token":
				if r.Form.Get("refresh_token") != "refresh-1" {
					t.Fatalf("unexpected refresh_token form: %#v", r.Form)
				}
				_, _ = w.Write([]byte(`{"access_token":"access-2","refresh_token":"refresh-2","token_type":"Bearer"}`))
			default:
				t.Fatalf("unexpected token grant form: %#v", r.Form)
			}
		case "POST /oauth/revoke":
			if r.Form.Get("token") != "access-1" {
				t.Fatalf("unexpected revoke form: %#v", r.Form)
			}
			_, _ = w.Write([]byte(`{"revoked":true}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"oauth", "token", "--client-id", "client-1", "--client-secret", "secret-1", "--code", "auth-code", "--redirect-uri", "http://localhost/callback", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"access_token":"access-1"`) {
		t.Fatalf("authorization_code output missing response: %q", out.String())
	}

	out.Reset()
	err = Run([]string{"oauth", "token", "--client-id", "client-1", "--client-secret", "secret-1", "--refresh-token", "refresh-1", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"access_token":"access-2"`) {
		t.Fatalf("refresh_token output missing response: %q", out.String())
	}

	out.Reset()
	err = Run([]string{"oauth", "revoke", "access-1", "--client-id", "client-1", "--client-secret", "secret-1", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"revoked":true`) {
		t.Fatalf("revoke output missing response: %q", out.String())
	}

	for _, want := range []string{
		"POST /oauth/token",
		"POST /oauth/revoke",
	} {
		if !slices.Contains(seen, want) {
			t.Fatalf("expected request %s in %#v", want, seen)
		}
	}
}
