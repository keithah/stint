package stintcli

import (
	"bytes"
	"encoding/json"
	"errors"
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

func TestBuildHeartbeatIncludesExtendedAIMetadata(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := Options{
		AIAgent:           "codex",
		AIAgentComplexity: "high",
		AIAgentVersion:    "1.2.3",
		AIModel:           "gpt-5-codex",
		AIProvider:        "openai",
		AISession:         "session-1",
		Category:          "ai coding",
		CommitHash:        "abcdef123",
		Editor:            "codex",
		EditorVersion:     "2.0.0",
		Entity:            file,
		EntityType:        "file",
		Metadata:          `{"request_id":"req_123"}`,
		Plugin:            "stint-cli/test",
		PluginVersion:     "dev",
	}
	hb, err := BuildHeartbeat(opts)
	if err != nil {
		t.Fatal(err)
	}
	if hb.AIModel != opts.AIModel || hb.AIProvider != opts.AIProvider || hb.AIAgent != opts.AIAgent || hb.AIAgentVersion != opts.AIAgentVersion || hb.AIAgentComplexity != opts.AIAgentComplexity {
		t.Fatalf("unexpected AI metadata: %#v", hb)
	}
	if hb.CommitHash != opts.CommitHash || hb.Editor != opts.Editor || hb.EditorVersion != opts.EditorVersion || hb.PluginVersion != opts.PluginVersion {
		t.Fatalf("unexpected extended metadata: %#v", hb)
	}
	if hb.Metadata["request_id"] != "req_123" {
		t.Fatalf("metadata = %#v", hb.Metadata)
	}
	if hb.UserAgent != userAgent(opts.Plugin) {
		t.Fatalf("user_agent = %q", hb.UserAgent)
	}
}

func TestParseCommonRejectsInvalidHideRegexesLikeWakaTime(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		config  map[string]string
		message string
	}{
		{
			name:    "hide dependencies flag",
			args:    []string{"--hide-dependencies", "[0-9+"},
			message: "failed to parse regex hide dependencies param",
		},
		{
			name:    "hide file names config",
			config:  map[string]string{"hide_file_names": "[0-9+"},
			message: "failed to parse regex hide file names param",
		},
		{
			name:    "hide project names config",
			config:  map[string]string{"hide_project_names": "[0-9+"},
			message: "failed to parse regex hide project names param",
		},
		{
			name:    "hide branch names config",
			config:  map[string]string{"hide_branch_names": "[0-9+"},
			message: "failed to parse regex hide branch names param",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{}, tt.args...)
			if len(tt.config) > 0 {
				config := filepath.Join(t.TempDir(), ".wakatime.cfg")
				cfg := Config{Sections: map[string]map[string]string{}}
				for key, value := range tt.config {
					cfg.Set("settings", key, value)
				}
				if err := cfg.Write(config); err != nil {
					t.Fatal(err)
				}
				args = append([]string{"--config", config}, args...)
			}
			_, err := parseCommon(args)
			if err == nil {
				t.Fatal("expected invalid hide regex error")
			}
			if !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestBuildHeartbeatHidesDependencies(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n\nimport \"net/http\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", HideDependencies: "true"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hb.Dependencies) != 0 {
		t.Fatalf("expected hidden dependencies, got %#v", hb.Dependencies)
	}
}

func TestBuildHeartbeatSanitizesFileLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\nimport \"github.com/acme/pkg\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{
		Category:       "coding",
		CursorPosition: 4,
		Entity:         file,
		EntityType:     "file",
		HideFileNames:  `main\.go$`,
		LineNumber:     1,
		LinesInFile:    2,
		Write:          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Entity != "HIDDEN.go" {
		t.Fatalf("entity = %q", hb.Entity)
	}
	if hb.Branch != "" || hb.CursorPosition != nil || hb.LineNumber != nil || hb.Lines != nil || hb.ProjectRootCount != nil || hb.Dependencies != nil {
		t.Fatalf("metadata was not sanitized: %#v", hb)
	}
}

func TestWakaTimeBooleanFilterRegexes(t *testing.T) {
	if skip, err := excluded("/tmp/main.go", nil, []string{"true"}); err != nil || !skip {
		t.Fatalf("exclude=true skip=%v err=%v", skip, err)
	}
	if skip, err := excluded("/tmp/main.go", []string{"true"}, []string{"true"}); err != nil || skip {
		t.Fatalf("include=true should override exclude=true: skip=%v err=%v", skip, err)
	}
	if skip, err := excluded("/tmp/main.go", nil, []string{"false"}); err != nil || skip {
		t.Fatalf("exclude=false skip=%v err=%v", skip, err)
	}
	if skip, err := excluded("/tmp/main.go", nil, []string{"["}); err != nil || skip {
		t.Fatalf("invalid exclude regex should be ignored: skip=%v err=%v", skip, err)
	}
}

func TestWakaTimeIncludeExcludeFlagsSplitCommas(t *testing.T) {
	opts, err := parseCommon([]string{
		"--key", "waka_test",
		"--include", `keep\.go$,also_keep\.go$`,
		"--exclude", `skip\.go$,also_skip\.go$`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(opts.Include, "|") != `keep\.go$|also_keep\.go$` {
		t.Fatalf("include flags were not comma split like WakaTime: %#v", opts.Include)
	}
	if strings.Join(opts.Exclude, "|") != `skip\.go$|also_skip\.go$` {
		t.Fatalf("exclude flags were not comma split like WakaTime: %#v", opts.Exclude)
	}
}

func TestWakaTimePerlRegexFilters(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked", "main.go")
	allowed := filepath.Join(dir, "allowed", "main.go")
	for _, file := range []string{blocked, allowed} {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	blockedOpts, err := parseCommon([]string{
		"--key", "waka_test",
		"--entity", blocked,
		"--exclude", "^" + regexp.QuoteMeta(dir) + string(filepath.Separator) + `(?!allowed` + regexp.QuoteMeta(string(filepath.Separator)) + `).*`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildHeartbeat(blockedOpts); !errors.Is(err, errHeartbeatFiltered) {
		t.Fatalf("negative lookahead exclude should filter blocked path like WakaTime, got %v", err)
	}
	allowedOpts, err := parseCommon([]string{
		"--key", "waka_test",
		"--entity", allowed,
		"--exclude", "^" + regexp.QuoteMeta(dir) + string(filepath.Separator) + `(?!allowed` + regexp.QuoteMeta(string(filepath.Separator)) + `).*`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildHeartbeat(allowedOpts); err != nil {
		t.Fatalf("negative lookahead exclude should allow matching exception like WakaTime, got %v", err)
	}
}

func TestRunFilteredMainHeartbeatSucceedsWithoutPosting(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		t.Fatalf("unexpected request for filtered heartbeat: %s", r.URL.Path)
	}))
	defer server.Close()

	if err := Run([]string{
		"--entity", file,
		"--exclude", ".*",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--disable-offline",
		"--sync-ai-disabled",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("server calls = %d", calls)
	}
}

func TestRunTodayHidesCodingActivityWhenStatusBarCodingActivityDisabled(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "status_bar_coding_activity", "false")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	body := `{"data":{"grand_total":{"hours":2,"text":"2 hrs 17 mins"},"categories":[{"hours":2,"name":"Coding","text":"2 hrs 17 mins"}]},"has_team_features":false}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	var out bytes.Buffer
	if err := Run([]string{"today", "--config", config, "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("expected hidden coding activity output, got %q", out.String())
	}
}

func TestRunTodayHidesStatusTextWhenStatusBarDisabled(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "status_bar_enabled", "false")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	body := `{"data":{"grand_total":{"hours":2,"text":"2 hrs 17 mins"},"categories":[{"hours":2,"name":"Coding","text":"2 hrs 17 mins"}]},"has_team_features":false}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	var out bytes.Buffer
	if err := Run([]string{"today", "--config", config, "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("expected disabled status bar output, got %q", out.String())
	}
}

func TestProcessExtraHeartbeatFiltersByEntityNotLocalFile(t *testing.T) {
	dir := t.TempDir()
	localFile := filepath.Join(dir, "excluded-local.py")
	if err := os.WriteFile(localFile, []byte("print('ok')\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, skip, err := processExtraHeartbeat(Heartbeat{
		Entity:     "ssh://example.test/tmp/remote.py",
		EntityType: "file",
		LocalFile:  localFile,
		Time:       123,
	}, Options{Category: "coding", Exclude: []string{`excluded-local\.py$`}})
	if err != nil {
		t.Fatal(err)
	}
	if skip {
		t.Fatal("local_file matched exclude, but WakaTime filters by entity")
	}
	if hb.Language != "Python" || hb.Lines == nil || *hb.Lines != 1 {
		t.Fatalf("local file stats were not still used: %#v", hb)
	}
}

func TestHeartbeatAutomaticallyIncludesAIActivityUnlessDisabled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "sample")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
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
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-27T12:00:00Z","session_id":"codex-auto","cwd":"` + filepath.ToSlash(project) + `"}`,
		`{"timestamp":"2026-06-27T12:01:00Z","message":"change main","filePath":"main.go","input_tokens":3,"output_tokens":4}`,
	}, "\n")
	transcriptPath := filepath.Join(transcriptDir, "codex-auto.jsonl")
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	modTime := time.Date(2026, 6, 27, 12, 1, 0, 0, time.UTC)
	if err := os.Chtimes(transcriptPath, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	var batches [][]Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		batches = append(batches, posted)
		_, _ = w.Write([]byte(`{"responses":[]}`))
	}))
	defer server.Close()
	internalConfig := filepath.Join(t.TempDir(), "wakatime-internal.cfg")
	if err := Run([]string{
		"--entity", file,
		"--write",
		"--sync-ai-after", "1780000000",
		"--internal-config", internalConfig,
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--heartbeat-rate-limit-seconds", "0",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 || len(batches[0]) != 3 {
		t.Fatalf("posted batches = %#v", batches)
	}
	if batches[0][0].Entity != file || batches[0][0].Category != "" {
		t.Fatalf("unexpected main heartbeat: %#v", batches[0][0])
	}
	if batches[0][1].EntityType != "app" || batches[0][1].Category != "ai coding" || batches[0][1].AISession != "codex-auto" {
		t.Fatalf("expected AI app heartbeat, got %#v", batches[0][1])
	}
	if batches[0][2].EntityType != "file" || batches[0][2].Entity != file || batches[0][2].Category != "ai coding" {
		t.Fatalf("expected AI file heartbeat, got %#v", batches[0][2])
	}
	cfg, err := LoadConfig(internalConfig)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Get("internal", "ai_sync_after") == "" {
		t.Fatalf("expected ai_sync_after to be recorded")
	}

	batches = nil
	if err := Run([]string{
		"--entity", file,
		"--write",
		"--sync-ai-disabled",
		"--sync-ai-after", "1780000000",
		"--internal-config", filepath.Join(t.TempDir(), "disabled.cfg"),
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--heartbeat-rate-limit-seconds", "0",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 || len(batches[0]) != 1 {
		t.Fatalf("disabled posted batches = %#v", batches)
	}
	if batches[0][0].Entity != file || batches[0][0].Category != "" {
		t.Fatalf("unexpected disabled heartbeat: %#v", batches[0][0])
	}
}
