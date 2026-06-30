package stintcli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRunImportWakaTimeUploadsMultipartFile(t *testing.T) {
	dir := t.TempDir()
	dumpPath := filepath.Join(dir, "dump.json")
	if err := os.WriteFile(dumpPath, []byte(`{"data":[{"entity":"main.go"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v1/users/current/imports/wakatime" {
			t.Fatalf("path = %s, want import endpoint", r.URL.EscapedPath())
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "multipart/form-data;") {
			t.Fatalf("Content-Type = %q, want multipart/form-data", got)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if header.Filename != "dump.json" {
			t.Fatalf("multipart filename = %q, want dump.json", header.Filename)
		}
		body, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != `{"data":[{"entity":"main.go"}]}` {
			t.Fatalf("multipart body = %q", string(body))
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"data":{"inserted":1}}`))
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"import", "wakatime", dumpPath, "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"inserted":1`) {
		t.Fatalf("import output missing response body: %q", out.String())
	}
}

func TestRunImportWakaTimePostsRawStdin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v1/users/current/imports/wakatime" {
			t.Fatalf("path = %s, want import endpoint", r.URL.EscapedPath())
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != `{"data":[{"entity":"stdin.go"}]}` {
			t.Fatalf("raw import body = %q", string(body))
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"data":{"queued":1}}`))
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"import", "wakatime", "--stdin", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, strings.NewReader(`{"data":[{"entity":"stdin.go"}]}`), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"queued":1`) {
		t.Fatalf("import output missing response body: %q", out.String())
	}
}
