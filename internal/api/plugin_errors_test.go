package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func TestPluginErrorsAcceptsWakaTimeDiagnosticsPayload(t *testing.T) {
	server := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plugins/errors", strings.NewReader(`{
		"architecture":"amd64",
		"cli_version":"dev",
		"error_message":"boom",
		"logs":"log line",
		"platform":"linux",
		"plugin":"vim",
		"stacktrace":"stack"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	userID := uuid.New()
	c.Set("auth", authContext{Kind: "api_key", Subject: userID.String(), Scopes: []string{scopeWriteHeartbeats}})

	if err := server.pluginErrors(c); err != nil {
		t.Fatalf("pluginErrors returned error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			OK bool `json:"ok"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Data.OK {
		t.Fatalf("expected ok response, got %#v", body)
	}
}

func TestPluginErrorsRejectsEmptyDiagnosticsPayload(t *testing.T) {
	server := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plugins/errors", strings.NewReader(`{"plugin":"vim"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := server.pluginErrors(c); err != nil {
		t.Fatalf("pluginErrors returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}
