package stintcli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestRunHeartbeatPostsWakaTimeBulkPayload(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.py")
	if err := os.WriteFile(file, []byte("print('ok')\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var got []Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users/current/heartbeats.bulk" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != basicAuthHeader("waka_test") {
			t.Fatalf("unexpected auth %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"responses":[[{"data":{"id":"hb"}} ,201]]}`))
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{
		"heartbeat",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--entity", file,
		"--write",
		"--plugin", "unit/1.0",
		"--offline-queue-file", filepath.Join(dir, "queue.bdb"),
		"--config", filepath.Join(dir, "missing.cfg"),
		"--heartbeat-rate-limit-seconds", "0",
		"--output", "raw-json",
	}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d heartbeats", len(got))
	}
	if got[0].Entity != file || !got[0].IsWrite || got[0].Language != "Python" {
		t.Fatalf("unexpected heartbeat: %#v", got[0])
	}
	if !strings.Contains(out.String(), `"responses"`) {
		t.Fatalf("expected response JSON, got %q", out.String())
	}
}

func TestRunHeartbeatSuccessIsSilentByDefaultLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := Run([]string{
		"--entity", file,
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--heartbeat-rate-limit-seconds", "0",
		"--offline-queue-file", filepath.Join(dir, "offline.bdb"),
		"--sync-ai-disabled",
	}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if out.String() != "" {
		t.Fatalf("default heartbeat output = %q, want empty", out.String())
	}
}

func TestRunHeartbeatExplicitOutputStillPrintsResponse(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := Run([]string{
		"--entity", file,
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--heartbeat-rate-limit-seconds", "0",
		"--offline-queue-file", filepath.Join(dir, "offline.bdb"),
		"--sync-ai-disabled",
		"--output", "raw-json",
	}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"responses"`) {
		t.Fatalf("explicit heartbeat output = %q", out.String())
	}
}

func TestHeartbeatFailureRecordsBackoff(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	internalConfig := filepath.Join(dir, "wakatime-internal.cfg")
	queue := filepath.Join(dir, "offline.bdb")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer server.Close()
	var out bytes.Buffer
	if err := Run([]string{
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--entity", file,
		"--offline-queue-file", queue,
		"--internal-config", internalConfig,
		"--heartbeat-rate-limit-seconds", "0",
	}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(internalConfig)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Get("internal", "backoff_retries") != "1" || cfg.Get("internal", "backoff_at") == "" {
		t.Fatalf("unexpected backoff config: %#v", cfg.Section("internal"))
	}
	if strings.TrimSpace(out.String()) != "queued=1" {
		t.Fatalf("expected queued output, got %q", out.String())
	}
}

func TestHeartbeatSuccessResetsExpiredBackoff(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	internalConfig := filepath.Join(dir, "wakatime-internal.cfg")
	if err := WriteConfigValue(internalConfig, "internal", "backoff_retries", "1"); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfigValue(internalConfig, "internal", "backoff_at", time.Now().Add(-time.Hour).Format(wakaTimeDateFormat)); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"responses":[]}`))
	}))
	defer server.Close()
	if err := Run([]string{
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--entity", file,
		"--offline-queue-file", filepath.Join(dir, "offline.bdb"),
		"--internal-config", internalConfig,
		"--heartbeat-rate-limit-seconds", "0",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(internalConfig)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Get("internal", "backoff_retries") != "0" || cfg.Get("internal", "backoff_at") != "" {
		t.Fatalf("expected reset backoff config, got %#v", cfg.Section("internal"))
	}
}

func TestRunHeartbeatMarksNearbyHumanHeartbeatAsAICoding(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "sample")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(home, ".codex", "sessions")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptPath := filepath.Join(transcriptDir, "codex-nearby.jsonl")
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-27T12:00:00Z","session_id":"codex-nearby","cwd":"` + filepath.ToSlash(project) + `"}`,
		`{"timestamp":"2026-06-27T12:01:00Z","message":"change main","filePath":"main.go"}`,
	}, "\n")
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	modTime := time.Date(2026, 6, 27, 12, 1, 0, 0, time.UTC)
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

	humanTime := time.Date(2026, 6, 27, 12, 2, 0, 0, time.UTC)
	if err := Run([]string{
		"--entity", file,
		"--time", strconv.FormatFloat(float64(humanTime.Unix()), 'f', -1, 64),
		"--human-line-changes", "0",
		"--sync-ai-after", "1780000000",
		"--heartbeat-rate-limit-seconds", "0",
		"--internal-config", filepath.Join(t.TempDir(), "wakatime-internal.cfg"),
		"--offline-queue-file", filepath.Join(t.TempDir(), "offline.bdb"),
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for _, hb := range posted {
		if hb.Entity == file {
			if hb.Category != aiCodingCategory {
				t.Fatalf("human heartbeat category = %q, want %q; posted=%#v", hb.Category, aiCodingCategory, posted)
			}
			return
		}
	}
	t.Fatalf("human heartbeat not posted: %#v", posted)
}

func TestRunHeartbeatMarksThirtyMinuteNoChangeHumanHeartbeatAsAICoding(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "sample")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(home, ".codex", "sessions")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptPath := filepath.Join(transcriptDir, "codex-thirty-minute.jsonl")
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-27T12:00:00Z","session_id":"codex-thirty-minute","cwd":"` + filepath.ToSlash(project) + `"}`,
		`{"timestamp":"2026-06-27T12:01:00Z","message":"change main","filePath":"main.go"}`,
	}, "\n")
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	modTime := time.Date(2026, 6, 27, 12, 1, 0, 0, time.UTC)
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

	humanTime := time.Date(2026, 6, 27, 12, 20, 0, 0, time.UTC)
	if err := Run([]string{
		"--entity", file,
		"--time", strconv.FormatFloat(float64(humanTime.Unix()), 'f', -1, 64),
		"--human-line-changes", "0",
		"--sync-ai-after", "1780000000",
		"--heartbeat-rate-limit-seconds", "0",
		"--internal-config", filepath.Join(t.TempDir(), "wakatime-internal.cfg"),
		"--offline-queue-file", filepath.Join(t.TempDir(), "offline.bdb"),
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for _, hb := range posted {
		if hb.Entity == file {
			if hb.Category != aiCodingCategory {
				t.Fatalf("human heartbeat category = %q, want %q; posted=%#v", hb.Category, aiCodingCategory, posted)
			}
			return
		}
	}
	t.Fatalf("human heartbeat not posted: %#v", posted)
}

func TestRunHeartbeatKeepsThirtyMinuteHumanEditCategory(t *testing.T) {
	humanChanges := 3
	human := []Heartbeat{{
		Category:         "debugging",
		Entity:           "/tmp/human-edit.go",
		EntityType:       "file",
		HumanLineChanges: &humanChanges,
		Time:             1200,
	}}
	ai := []Heartbeat{{
		Category:   aiCodingCategory,
		Entity:     "/tmp/ai.go",
		EntityType: "file",
		Time:       60,
	}}

	human = mergeHumanHeartbeatsWithAI(human, ai)

	if human[0].Category != "debugging" {
		t.Fatalf("human edit category = %q, want debugging", human[0].Category)
	}
}

func TestRunHeartbeatDropsDuplicateHumanHeartbeatNearAIFileHeartbeat(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "sample")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(home, ".codex", "sessions")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptPath := filepath.Join(transcriptDir, "codex-duplicate.jsonl")
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-27T12:00:00Z","session_id":"codex-duplicate","cwd":"` + filepath.ToSlash(project) + `"}`,
		`{"timestamp":"2026-06-27T12:01:00Z","message":"change main","filePath":"main.go"}`,
	}, "\n")
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	modTime := time.Date(2026, 6, 27, 12, 1, 0, 0, time.UTC)
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

	humanTime := time.Date(2026, 6, 27, 12, 1, 1, 0, time.UTC)
	if err := Run([]string{
		"--entity", file,
		"--time", strconv.FormatFloat(float64(humanTime.Unix()), 'f', -1, 64),
		"--human-line-changes", "0",
		"--sync-ai-after", "1780000000",
		"--heartbeat-rate-limit-seconds", "0",
		"--internal-config", filepath.Join(t.TempDir(), "wakatime-internal.cfg"),
		"--offline-queue-file", filepath.Join(t.TempDir(), "offline.bdb"),
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	var fileCount int
	for _, hb := range posted {
		if hb.Entity == file {
			fileCount++
			if hb.AISession != "codex-duplicate" {
				t.Fatalf("expected remaining file heartbeat to be AI heartbeat, got %#v", hb)
			}
		}
	}
	if fileCount != 1 {
		t.Fatalf("file heartbeat count = %d, want 1; posted=%#v", fileCount, posted)
	}
}

func TestRunHeartbeatPreservesHumanFileStatsOnDuplicateAIHeartbeat(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "sample")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(home, ".codex", "sessions")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptPath := filepath.Join(transcriptDir, "codex-preserve.jsonl")
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-27T12:00:00Z","session_id":"codex-preserve","cwd":"` + filepath.ToSlash(project) + `"}`,
		`{"timestamp":"2026-06-27T12:01:00Z","message":"change main","filePath":"main.go"}`,
	}, "\n")
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	modTime := time.Date(2026, 6, 27, 12, 1, 0, 0, time.UTC)
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

	humanTime := time.Date(2026, 6, 27, 12, 1, 1, 0, time.UTC)
	if err := Run([]string{
		"--entity", file,
		"--time", strconv.FormatFloat(float64(humanTime.Unix()), 'f', -1, 64),
		"--human-line-changes", "0",
		"--lines-in-file", "7",
		"--sync-ai-after", "1780000000",
		"--heartbeat-rate-limit-seconds", "0",
		"--internal-config", filepath.Join(t.TempDir(), "wakatime-internal.cfg"),
		"--offline-queue-file", filepath.Join(t.TempDir(), "offline.bdb"),
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	for _, hb := range posted {
		if hb.Entity == file && hb.AISession == "codex-preserve" {
			if hb.Lines == nil || *hb.Lines != 7 {
				t.Fatalf("AI heartbeat lines = %#v, want 7; posted=%#v", hb.Lines, posted)
			}
			if hb.ProjectRootCount == nil || *hb.ProjectRootCount == 0 {
				t.Fatalf("AI heartbeat project_root_count = %#v; posted=%#v", hb.ProjectRootCount, posted)
			}
			return
		}
	}
	t.Fatalf("AI file heartbeat not posted: %#v", posted)
}
