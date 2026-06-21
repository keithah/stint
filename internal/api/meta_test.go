package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/keithah/stint/internal/config"
	"github.com/labstack/echo/v4"
)

func TestMetaIncludesClientIPAndServerURLs(t *testing.T) {
	server := &Server{Config: config.Config{BaseURL: "http://example.test"}}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.42, 10.0.0.4")
	req.RemoteAddr = "192.0.2.10:54123"
	rec := httptest.NewRecorder()

	if err := server.meta(e.NewContext(req, rec)); err != nil {
		t.Fatalf("meta returned error: %v", err)
	}

	var body struct {
		Data struct {
			APIURL   string `json:"api_url"`
			BaseURL  string `json:"base_url"`
			Hostname string `json:"hostname"`
			IP       string `json:"ip"`
			Version  string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("could not decode meta response: %v", err)
	}

	if body.Data.IP != "203.0.113.42" {
		t.Fatalf("expected forwarded client IP, got %q", body.Data.IP)
	}
	if body.Data.BaseURL != "http://example.test" {
		t.Fatalf("expected base_url from config, got %q", body.Data.BaseURL)
	}
	if body.Data.APIURL != "http://example.test/api/v1" {
		t.Fatalf("expected api_url from config, got %q", body.Data.APIURL)
	}
	if body.Data.Hostname == "" {
		t.Fatal("expected hostname to be present")
	}
	if body.Data.Version == "" {
		t.Fatal("expected version to be present")
	}
}

func TestClientIPFallsBackToRemoteAddrHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil)
	req.RemoteAddr = "192.0.2.10:54123"

	if got := clientIP(req); got != "192.0.2.10" {
		t.Fatalf("expected remote addr host, got %q", got)
	}
}
