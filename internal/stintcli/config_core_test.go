package stintcli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestConfigReadWriteAndRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".wakatime.cfg")
	if err := InitConfig(path, "http://stint.local/api/v1", "waka_key"); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfigValue(path, "settings", "api_url", "http://new.local/api/v1"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run([]string{"--config", path, "--config-read", "api_url"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "http://new.local/api/v1" {
		t.Fatalf("unexpected config read: %q", out.String())
	}
}

func TestConfigInitWritesNativeStintConfigByDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)

	var out bytes.Buffer
	if err := Run([]string{"config", "init", "--api-url", "https://stint.example.com/api/v1", "--api-key", "stint_test"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(DefaultStintConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Get("settings", "api_url") != "https://stint.example.com/api/v1" || cfg.Get("settings", "api_key") != "stint_test" {
		t.Fatalf("unexpected native config: %#v", cfg.Section("settings"))
	}
	if _, err := os.Stat(DefaultWakaTimeConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("config init should not write wakatime config by default, stat err=%v", err)
	}
	if !strings.Contains(out.String(), ".stint.cfg") {
		t.Fatalf("expected native config path in output, got %q", out.String())
	}
}

func TestConfigReadErrorsWhenValueIsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".wakatime.cfg")
	if err := InitConfig(path, "http://stint.local/api/v1", "waka_key"); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--config", path, "--config-read", "missing"},
		{"config", "read", "--config", path, "missing"},
	} {
		err := Run(args, nil, &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), `settings.missing`) {
			t.Fatalf("Run(%v) error = %v", args, err)
		}
	}
}

func TestRootConfigReadEmptyKeyDispatchesLikeWakaTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".wakatime.cfg")
	if err := InitConfig(path, "http://stint.local/api/v1", "waka_key"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := Run([]string{"--config", path, "--config-read", ""}, nil, &out, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected config-read error")
	}
	if strings.Contains(out.String(), "Usage:") {
		t.Fatalf("empty config-read should dispatch to config reader, got help output: %q", out.String())
	}
	if !strings.Contains(err.Error(), "neither section nor key can be empty") {
		t.Fatalf("unexpected config-read error: %v", err)
	}
}

func TestRootConfigReadUsesImportedConfig(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, ".wakatime.cfg")
	importPath := filepath.Join(dir, "private.cfg")
	main := Config{Sections: map[string]map[string]string{}}
	main.Set("settings", "import_cfg", importPath)
	if err := main.Write(mainPath); err != nil {
		t.Fatal(err)
	}
	imported := Config{Sections: map[string]map[string]string{}}
	imported.Set("settings", "api_key", "waka_imported")
	if err := imported.Write(importPath); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run([]string{"--config", mainPath, "--config-read", "api_key"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "waka_imported" {
		t.Fatalf("unexpected config read: %q", out.String())
	}
}

func TestRootConfigReadExpandsHomeInImportedConfigPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mainPath := filepath.Join(home, ".wakatime.cfg")
	importPath := filepath.Join(home, "private.cfg")
	main := Config{Sections: map[string]map[string]string{}}
	main.Set("settings", "import_cfg", "~/private.cfg")
	if err := main.Write(mainPath); err != nil {
		t.Fatal(err)
	}
	imported := Config{Sections: map[string]map[string]string{}}
	imported.Set("settings", "api_key", "waka_imported_home")
	if err := imported.Write(importPath); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run([]string{"--config", mainPath, "--config-read", "api_key"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "waka_imported_home" {
		t.Fatalf("unexpected config read: %q", out.String())
	}
}

func TestRootConfigReadErrorsWhenImportedConfigHomeExpansionFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "import_cfg", "~missing-user/private.cfg")
	if err := cfg.Write(path); err != nil {
		t.Fatal(err)
	}
	err := Run([]string{"--config", path, "--config-read", "api_key"}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "failed to expand settings.import_cfg param") {
		t.Fatalf("expected import expansion error, got %v", err)
	}
}

func TestEntityHomeExpansionErrorMatchesWakaTime(t *testing.T) {
	err := Run([]string{
		"--entity", "~missing-user/main.go",
		"--key", "waka_test",
		"--sync-ai-disabled",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "failed expanding entity") {
		t.Fatalf("expected entity expansion error, got %v", err)
	}
}

func TestRootConfigWriteAcceptsRepeatedWakaTimePairs(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".wakatime.cfg")
	if err := Run([]string{
		"--config", path,
		"--config-write", "debug=true",
		"--config-write", "hide_file_names=false",
		"--config-write", "empty=",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Get("settings", "debug") != "true" || cfg.Get("settings", "hide_file_names") != "false" || cfg.Get("settings", "empty") != "" {
		t.Fatalf("unexpected config values: %#v", cfg.Section("settings"))
	}
}

func TestRootConfigWriteStripsNullBytesLikeWakaTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".wakatime.cfg")
	if err := Run([]string{
		"--config", path,
		"--config-write", "de\x00bug=tr\x00ue",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte{0}) {
		t.Fatalf("config still contains null byte: %q", data)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Get("settings", "debug") != "true" {
		t.Fatalf("debug = %q, file=%q", cfg.Get("settings", "debug"), data)
	}
}

func TestRootConfigWriteKeepsTwoArgumentExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".wakatime.cfg")
	if err := Run([]string{"--config", path, "--config-write", "debug", "true"}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Get("settings", "debug") != "true" {
		t.Fatalf("debug = %q", cfg.Get("settings", "debug"))
	}
}

func TestRootConfigWritePreservesSingleCommaValueLikeWakaTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".wakatime.cfg")
	if err := Run([]string{
		"--config", path,
		"--config-write", "include=alpha,beta",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Get("settings", "include"); got != "alpha,beta" {
		t.Fatalf("include = %q", got)
	}
}

func TestRunHeartbeatFansOutToConfiguredAPIURLs(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counts[r.Header.Get("Authorization")]++
		_, _ = w.Write([]byte(`{"responses":[]}`))
	}))
	defer server.Close()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "api_url", server.URL+"/api/v1")
	cfg.Set("settings", "api_key", "waka_default")
	cfg.Set("api_urls", ".*main\\.go$", server.URL+"/api/v1|waka_fanout")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{
		"--config", config,
		"--entity", file,
		"--offline-queue-file", filepath.Join(dir, "queue.bdb"),
		"--heartbeat-rate-limit-seconds", "0",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if counts[basicAuthHeader("waka_default")] != 1 || counts[basicAuthHeader("waka_fanout")] != 1 {
		t.Fatalf("unexpected fanout counts: %#v", counts)
	}
}

func TestRunHeartbeatPostsAPIURLFanoutInWakaTimeConfigOrder(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var authOrder []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authOrder = append(authOrder, r.Header.Get("Authorization"))
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()
	config := filepath.Join(dir, ".wakatime.cfg")
	body := "[settings]\n" +
		"api_url = " + server.URL + "/api/v1\n" +
		"api_key = waka_m_default\n" +
		"[api_urls]\n" +
		".*main\\.go$ = " + server.URL + "/api/v1|waka_z_first\n" +
		".*go$ = " + server.URL + "/api/v1|waka_a_second\n"
	if err := os.WriteFile(config, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{
		"--config", config,
		"--entity", file,
		"--offline-queue-file", filepath.Join(dir, "queue.bdb"),
		"--heartbeat-rate-limit-seconds", "0",
		"--sync-ai-disabled",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		basicAuthHeader("waka_m_default"),
		basicAuthHeader("waka_z_first"),
		basicAuthHeader("waka_a_second"),
	}
	if !slices.Equal(authOrder, want) {
		t.Fatalf("auth order = %#v, want %#v", authOrder, want)
	}
}

func TestImportedConfigOverridesMainConfig(t *testing.T) {
	dir := t.TempDir()
	mainConfig := filepath.Join(dir, ".wakatime.cfg")
	importedConfig := filepath.Join(dir, "imported.cfg")
	main := Config{Sections: map[string]map[string]string{}}
	main.Set("settings", "api_key", "waka_main")
	main.Set("settings", "api_url", "http://main.example/api/v1")
	main.Set("settings", "import_cfg", importedConfig)
	if err := main.Write(mainConfig); err != nil {
		t.Fatal(err)
	}
	imported := Config{Sections: map[string]map[string]string{}}
	imported.Set("settings", "api_key", "waka_imported")
	imported.Set("settings", "api_url", "http://imported.example/api/v1")
	imported.Set("settings", "hide_file_names", "true")
	if err := imported.Write(importedConfig); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{"--config", mainConfig})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_imported" || opts.APIURL != "http://imported.example/api/v1" {
		t.Fatalf("unexpected imported API config: key=%q url=%q", opts.APIKey, opts.APIURL)
	}
	if opts.HideFileNames != "true" {
		t.Fatalf("HideFileNames = %q", opts.HideFileNames)
	}
}

func TestInternalConfigHomeExpansionErrorMatchesWakaTime(t *testing.T) {
	_, err := parseCommon([]string{
		"--key", "waka_test",
		"--internal-config", "~missing-user/internal.cfg",
	})
	if err == nil || !strings.Contains(err.Error(), "failed to expand internal-config param") {
		t.Fatalf("expected internal config expansion error, got %v", err)
	}
}

func TestConfigPathHomeExpansionErrorMatchesWakaTime(t *testing.T) {
	_, err := parseCommon([]string{
		"--key", "waka_test",
		"--config", "~missing-user/.wakatime.cfg",
	})
	if err == nil || !strings.Contains(err.Error(), "failed to expand config param") {
		t.Fatalf("expected config expansion error, got %v", err)
	}
}

func TestExtraHeartbeatEntityHomeExpansionErrorMatchesWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdin := strings.NewReader(`[{"entity":"~missing-user/extra.go","type":"file","time":123}]`)
	err := Run([]string{
		"--entity", file,
		"--extra-heartbeats",
		"--api-url", "http://127.0.0.1:1/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", filepath.Join(dir, "queue.bdb"),
		"--heartbeat-rate-limit-seconds", "0",
		"--sync-ai-disabled",
	}, stdin, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "failed expanding entity") {
		t.Fatalf("expected extra heartbeat entity expansion error, got %v", err)
	}
}
