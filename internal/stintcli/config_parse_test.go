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
	"time"

	_ "modernc.org/sqlite"
)

func TestRootEmptyConfigFlagFallsBackToDefaultLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WAKATIME_HOME", dir)
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "api_key", "waka_default_empty_config")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := Run([]string{"--config=", "--config-read", "api_key"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "waka_default_empty_config" {
		t.Fatalf("unexpected config read: %q", out.String())
	}
}

func TestConfigSubcommandsAcceptConfigSectionAlias(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".wakatime.cfg")
	if err := Run([]string{
		"config", "write",
		"--config", path,
		"--config-section", "git",
		"project_from_git_remote", "true",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run([]string{
		"config", "read",
		"--config", path,
		"--config-section", "git",
		"project_from_git_remote",
	}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != "true" {
		t.Fatalf("config read = %q", got)
	}
}

func TestParseCommonAcceptsWakaTimeConfigAliases(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "apikey", "waka_alias")
	cfg.Set("settings", "debug", "true")
	cfg.Set("settings", "hidefilenames", "true")
	cfg.Set("settings", "hide_projectnames", "true")
	cfg.Set("settings", "hidebranchnames", "true")
	cfg.Set("settings", "guess_language", "true")
	cfg.Set("settings", "hostname", "config-host")
	cfg.Set("settings", "include", `.*\.go$`)
	cfg.Set("settings", "exclude", "vendor")
	cfg.Set("settings", "ignore", "node_modules")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}

	opts, err := parseCommon([]string{"--config", config, "--include", `.*\.ts$`})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_alias" || !opts.Verbose || opts.Hostname != "config-host" {
		t.Fatalf("unexpected alias config: key=%q verbose=%v hostname=%q", opts.APIKey, opts.Verbose, opts.Hostname)
	}
	if opts.HideFileNames != "true" || opts.HideProjectNames != "true" || opts.HideBranchNames != "true" {
		t.Fatalf("unexpected hide aliases: %#v", opts)
	}
	if !opts.GuessLanguage {
		t.Fatalf("expected settings.guess_language to enable content language guessing")
	}
	if strings.Join(opts.Include, ",") != `.*\.ts$,.*\.go$` {
		t.Fatalf("include flags and config should compose: %v", opts.Include)
	}
	if strings.Join(opts.Exclude, ",") != "vendor,node_modules" {
		t.Fatalf("exclude and ignore should compose: %v", opts.Exclude)
	}
}

func TestParseCommonReadsSyncAIDisabledFromConfig(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "sync_ai_disabled", "true")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}

	opts, err := parseCommon([]string{"--config", config})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.SyncAIDisabled {
		t.Fatalf("expected settings.sync_ai_disabled to disable AI sync")
	}
}

func TestParseCommonBackoffAtMatchesWakaTimeInternalConfig(t *testing.T) {
	dir := t.TempDir()
	internalConfig := filepath.Join(dir, "wakatime-internal.cfg")
	future := time.Now().Add(2 * time.Hour)
	if err := WriteConfigValue(internalConfig, "internal", "backoff_retries", "3"); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfigValue(internalConfig, "internal", "backoff_at", future.Format(wakaTimeDateFormat)); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{"--internal-config", internalConfig, "--key", "waka_test"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.BackoffRetries != 3 {
		t.Fatalf("BackoffRetries = %d", opts.BackoffRetries)
	}
	if opts.BackoffAt.IsZero() || opts.BackoffAt.After(time.Now()) {
		t.Fatalf("future backoff_at should clamp to now like WakaTime, got %s", opts.BackoffAt.Format(wakaTimeDateFormat))
	}

	if err := WriteConfigValue(internalConfig, "internal", "backoff_at", "2021-08-30"); err != nil {
		t.Fatal(err)
	}
	opts, err = parseCommon([]string{"--internal-config", internalConfig, "--key", "waka_test"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.BackoffAt.IsZero() {
		t.Fatalf("invalid backoff_at should be ignored like WakaTime, got %s", opts.BackoffAt.Format(wakaTimeDateFormat))
	}
}

func TestHeartbeatRateLimitUsesWakaTimeInternalConfigState(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	internalConfig := filepath.Join(dir, "wakatime-internal.cfg")
	queue := filepath.Join(dir, "offline.bdb")
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var heartbeats []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&heartbeats); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(heartbeats, http.StatusCreated))
	}))
	defer server.Close()
	args := []string{
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--entity", file,
		"--offline-queue-file", queue,
		"--internal-config", internalConfig,
		"--heartbeat-rate-limit-seconds", "120",
		"--sync-ai-disabled",
	}
	if err := Run(args, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(internalConfig)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Get("internal", "heartbeats_last_sent_at") == "" {
		t.Fatalf("expected heartbeats_last_sent_at in internal config: %#v", cfg.Section("internal"))
	}
	var out bytes.Buffer
	if err := Run(args, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("network calls = %d", calls)
	}
	if strings.TrimSpace(out.String()) != "queued=1" {
		t.Fatalf("expected queued output, got %q", out.String())
	}
	count, err := CountQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("queue count = %d", count)
	}
	if fileExists(queue + ".last_sent") {
		t.Fatalf("unexpected legacy sidecar rate-limit file")
	}
}

func TestDefaultSectionConfigActsAsTopLevelWakaTimeConfig(t *testing.T) {
	config := filepath.Join(t.TempDir(), ".wakatime.cfg")
	if err := os.WriteFile(config, []byte("[DEFAULT]\napi_key = waka_default\napi_url = http://default.example/api/v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{"--config", config})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_default" || opts.APIURL != "http://default.example/api/v1" {
		t.Fatalf("DEFAULT config not applied: key=%q url=%q", opts.APIKey, opts.APIURL)
	}
}

func TestConfigAPIURLAliasesMatchWakaTime(t *testing.T) {
	cases := map[string]string{
		"api-url": "http://dashed.example/api/v1",
		"apiurl":  "http://legacy.example/api/v1",
	}
	for key, want := range cases {
		t.Run(key, func(t *testing.T) {
			config := filepath.Join(t.TempDir(), ".wakatime.cfg")
			if err := os.WriteFile(config, []byte("[settings]\napi_key = waka_alias\n"+key+" = "+want+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			opts, err := parseCommon([]string{"--config", config})
			if err != nil {
				t.Fatal(err)
			}
			if opts.APIURL != want {
				t.Fatalf("APIURL = %q, want %q", opts.APIURL, want)
			}
		})
	}
}

func TestConfigSectionAndSettingKeysAreCaseInsensitive(t *testing.T) {
	config := filepath.Join(t.TempDir(), ".wakatime.cfg")
	if err := os.WriteFile(config, []byte("[Settings]\nAPI_KEY = waka_case\nAPI_URL = http://case.example/api/v1\nDEBUG = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{"--config", config})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_case" || opts.APIURL != "http://case.example/api/v1" || !opts.Verbose {
		t.Fatalf("case-insensitive settings not applied: key=%q url=%q verbose=%v", opts.APIKey, opts.APIURL, opts.Verbose)
	}
}

func TestQuotedConfigValuesAreTrimmedLikeWakaTime(t *testing.T) {
	config := filepath.Join(t.TempDir(), ".wakatime.cfg")
	if err := os.WriteFile(config, []byte("[settings]\napi_key = \"waka_quoted\"\napi_url = 'http://quoted.example/api/v1'\nproxy = \"http://proxy.example:8080\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{"--config", config})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_quoted" || opts.APIURL != "http://quoted.example/api/v1" || opts.Proxy != "http://proxy.example:8080" {
		t.Fatalf("quoted config values not trimmed: key=%q url=%q proxy=%q", opts.APIKey, opts.APIURL, opts.Proxy)
	}
}

func TestParseCommonPreservesConfigZeroTimeoutLikeWakaTime(t *testing.T) {
	config := filepath.Join(t.TempDir(), ".wakatime.cfg")
	if err := os.WriteFile(config, []byte("[settings]\napi_key = waka_test\ntimeout = 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{"--config", config})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Timeout != 0 {
		t.Fatalf("config timeout = %d", opts.Timeout)
	}
}

func TestParseCommonUsesDefaultForInvalidHeartbeatRateLimitConfigLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "api_key", "waka_test")
	cfg.Set("settings", "heartbeat_rate_limit_seconds", "invalid")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{"--config", config})
	if err != nil {
		t.Fatal(err)
	}
	if opts.HeartbeatRateLimit != 120 {
		t.Fatalf("invalid shared rate limit should fall back to default, got %d", opts.HeartbeatRateLimit)
	}

	project := filepath.Join(dir, "project-invalid")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	entity := filepath.Join(project, "main.go")
	if err := os.WriteFile(entity, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	projectConfig := Config{Sections: map[string]map[string]string{}}
	projectConfig.Set("settings", "heartbeat_rate_limit_seconds", "invalid")
	if err := projectConfig.Write(filepath.Join(project, ".wakatime")); err != nil {
		t.Fatal(err)
	}
	opts, err = parseCommon([]string{"--config", config, "--entity", entity})
	if err != nil {
		t.Fatal(err)
	}
	if opts.HeartbeatRateLimit != 120 {
		t.Fatalf("invalid project rate limit should fall back to current default, got %d", opts.HeartbeatRateLimit)
	}
}

func TestVerboseFlagFalseTakesPrecedenceOverConfig(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "debug", "true")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{
		"--config", config,
		"--verbose=false",
		"--key", "waka_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Verbose {
		t.Fatal("config debug=true overrode explicit --verbose=false")
	}
}
