package api

import (
	"bytes"
	"compress/gzip"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestReadImportBodyReadsRawBody(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/users/current/imports/wakatime", strings.NewReader(`{"data":[]}`))
	c := echo.New().NewContext(req, httptest.NewRecorder())

	got, err := readImportBody(c)
	if err != nil {
		t.Fatalf("readImportBody returned error: %v", err)
	}
	if string(got) != `{"data":[]}` {
		t.Fatalf("unexpected raw body: %s", got)
	}
}

func TestReadImportBodyReadsMultipartFile(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "wakatime.json")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte(`{"data":[{"project":"stint"}]}`)); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest("POST", "/api/v1/users/current/imports/wakatime", &body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	c := echo.New().NewContext(req, httptest.NewRecorder())

	got, err := readImportBody(c)
	if err != nil {
		t.Fatalf("readImportBody returned error: %v", err)
	}
	if !strings.Contains(string(got), `"project":"stint"`) {
		t.Fatalf("unexpected multipart body: %s", got)
	}
}

func TestReadImportBodyRequiresMultipartFile(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/users/current/imports/wakatime", strings.NewReader("--missing--"))
	req.Header.Set(echo.HeaderContentType, "multipart/form-data; boundary=missing")
	c := echo.New().NewContext(req, httptest.NewRecorder())

	if _, err := readImportBody(c); err == nil {
		t.Fatal("expected missing multipart file to return an error")
	}
}

func TestReadImportBodyRejectsOversizedExpandedGzip(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(bytes.Repeat([]byte("x"), maxImportBodyBytes+1)); err != nil {
		t.Fatalf("write gzip payload: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close gzip payload: %v", err)
	}

	req := httptest.NewRequest("POST", "/api/v1/users/current/imports/wakatime", &compressed)
	c := echo.New().NewContext(req, httptest.NewRecorder())

	_, err := readImportBody(c)
	if err == nil {
		t.Fatal("expected oversized expanded gzip to return an error")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected size error, got %v", err)
	}
}
