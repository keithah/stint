package stintcli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRunCustomPricingUpsertAndDeleteUseExpectedEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			if r.URL.EscapedPath() != "/api/v1/users/current/custom_pricing" {
				t.Fatalf("put path = %s, want custom pricing endpoint", r.URL.EscapedPath())
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["model"] != "gpt-5" {
				t.Fatalf("unexpected payload: %#v", payload)
			}
			_, _ = w.Write([]byte(`{"data":[{"model":"gpt-5"}]}`))
		case http.MethodDelete:
			if r.URL.EscapedPath() != "/api/v1/users/current/custom_pricing/gpt-5%20mini" {
				t.Fatalf("delete path = %s, want escaped custom pricing model", r.URL.EscapedPath())
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("method = %s, want PUT or DELETE", r.Method)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"custom-pricing", "upsert", "--stdin", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, strings.NewReader(`{"model":"gpt-5","input_per_million_usd":1.25,"output_per_million_usd":10}`), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"model":"gpt-5"`) {
		t.Fatalf("upsert output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"custom-pricing", "delete", "gpt-5 mini", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("delete should not print a 204 response body, got %q", out.String())
	}
}

func TestRunBillingPrefsUpsertAndDeleteUseExpectedEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			if r.URL.EscapedPath() != "/api/v1/users/current/billing_prefs" {
				t.Fatalf("put path = %s, want billing prefs endpoint", r.URL.EscapedPath())
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["agent"] != "codex" || payload["billing_type"] != "subscription" {
				t.Fatalf("unexpected payload: %#v", payload)
			}
			_, _ = w.Write([]byte(`{"data":[{"agent":"codex"}]}`))
		case http.MethodDelete:
			if r.URL.EscapedPath() != "/api/v1/users/current/billing_prefs/claude%20code" {
				t.Fatalf("delete path = %s, want escaped billing agent", r.URL.EscapedPath())
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("method = %s, want PUT or DELETE", r.Method)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"billing-prefs", "upsert", "--stdin", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, strings.NewReader(`{"agent":"codex","billing_type":"subscription"}`), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"agent":"codex"`) {
		t.Fatalf("upsert output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"billing-prefs", "delete", "claude code", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("delete should not print a 204 response body, got %q", out.String())
	}
}

func TestRunAICostsReplacePutsArrayBody(t *testing.T) {
	dir := t.TempDir()
	bodyPath := filepath.Join(dir, "ai-costs.json")
	if err := os.WriteFile(bodyPath, []byte(`[{"agent":"Codex","input_cost_per_million_cents":300,"output_cost_per_million_cents":1200}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v1/users/current/ai_costs" {
			t.Fatalf("path = %s, want ai_costs endpoint", r.URL.EscapedPath())
		}
		var payload []map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload) != 1 || payload[0]["agent"] != "Codex" {
			t.Fatalf("unexpected payload: %#v", payload)
		}
		_, _ = w.Write([]byte(`{"data":[{"agent":"Codex"}]}`))
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"ai-costs", "replace", bodyPath, "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"agent":"Codex"`) {
		t.Fatalf("replace output missing response: %q", out.String())
	}
}
