package api

import (
	"bytes"
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
