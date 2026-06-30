package stintcli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestSyncAIActivityIncludeOverridesExclude(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	includedDir := filepath.Join(home, "included")
	excludedDir := filepath.Join(home, "excluded")
	if err := os.MkdirAll(includedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(excludedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	includedFile := filepath.Join(includedDir, "included.go")
	excludedFile := filepath.Join(excludedDir, "excluded.go")
	if err := os.WriteFile(includedFile, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(excludedFile, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(home, ".codex", "sessions")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptPath := filepath.Join(transcriptDir, "codex-include.jsonl")
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-27T12:00:00Z","session_id":"codex-include","cwd":"` + filepath.ToSlash(home) + `"}`,
		`{"timestamp":"2026-06-27T12:01:00Z","filePath":"` + filepath.ToSlash(includedFile) + `"}`,
		`{"timestamp":"2026-06-27T12:02:00Z","filePath":"` + filepath.ToSlash(excludedFile) + `"}`,
	}, "\n")
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	modTime := time.Date(2026, 6, 27, 12, 2, 0, 0, time.UTC)
	if err := os.Chtimes(transcriptPath, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	var posted []Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()
	if err := Run([]string{
		"--sync-ai-activity",
		"--sync-ai-after", "1780000000",
		"--exclude", `.*`,
		"--include", "^" + regexp.QuoteMeta(includedDir) + string(filepath.Separator) + ".*",
		"--heartbeat-rate-limit-seconds", "0",
		"--internal-config", filepath.Join(t.TempDir(), "wakatime-internal.cfg"),
		"--offline-queue-file", filepath.Join(t.TempDir(), "offline.bdb"),
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 2 {
		t.Fatalf("posted = %#v", posted)
	}
	var sawApp, sawIncluded bool
	for _, hb := range posted {
		sawApp = sawApp || hb.EntityType == "app"
		sawIncluded = sawIncluded || hb.Entity == includedFile
		if hb.Entity == excludedFile {
			t.Fatalf("excluded file was posted: %#v", posted)
		}
	}
	if !sawApp || !sawIncluded {
		t.Fatalf("expected app and included file heartbeats, got %#v", posted)
	}
}
