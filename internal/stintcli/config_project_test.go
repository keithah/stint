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

	_ "modernc.org/sqlite"
)

func TestBuildHeartbeatHidesConfiguredProjectFolderLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "project")
	nested := filepath.Join(project, "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(nested, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{
		Entity:            file,
		EntityType:        "file",
		Category:          "coding",
		HideProjectFolder: true,
		ProjectFolder:     project,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Entity != filepath.Join("src", "main.go") || hb.ProjectRootCount != nil {
		t.Fatalf("expected configured project folder to be hidden, got %#v", hb)
	}
}

func TestProjectMapAndProjectAPIKeyFromConfig(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counts[r.Header.Get("Authorization")]++
		var got []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Project != "mapped-project" {
			t.Fatalf("unexpected heartbeat: %#v", got)
		}
		_, _ = w.Write([]byte(`{"responses":[]}`))
	}))
	defer server.Close()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "api_url", server.URL+"/api/v1")
	cfg.Set("settings", "api_key", "waka_default")
	cfg.Set("projectmap", ".*main\\.go$", "mapped-project")
	cfg.Set("project_api_key", ".*main\\.go$", "waka_project")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"--config", config, "--entity", file, "--offline-queue-file", filepath.Join(dir, "queue.bdb"), "--heartbeat-rate-limit-seconds", "0"}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if counts[basicAuthHeader("waka_project")] != 1 || counts[basicAuthHeader("waka_default")] != 0 {
		t.Fatalf("unexpected auth counts: %#v", counts)
	}
}

func TestRoutingConfigSkipsInvalidRegexPatternsLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != basicAuthHeader("waka_default") {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		var heartbeats []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&heartbeats); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(heartbeats, http.StatusCreated))
	}))
	defer server.Close()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "api_url", server.URL+"/api/v1")
	cfg.Set("settings", "api_key", "waka_default")
	cfg.Set("project_api_key", "(?", "waka_project")
	cfg.Set("api_urls", "(?", server.URL+"/api/v1|waka_secondary")
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
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestWakaTimeRegexConfigIsCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "MAIN.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("projectmap", `main\.go$`, "mapped-case")
	cfg.Set("project_api_key", `main\.go$`, "waka_project")
	cfg.Set("api_urls", `main\.go$`, "http://secondary.example/api/v1|waka_secondary")

	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Project != "mapped-case" {
		t.Fatalf("project = %q", hb.Project)
	}
	targets, err := heartbeatTargets(Options{APIURL: "http://default.example/api/v1", APIKey: "waka_default", Config: cfg}, file)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || targets[0].APIKey != "waka_project" || targets[1].APIKey != "waka_secondary" {
		t.Fatalf("unexpected targets: %#v", targets)
	}
}

func TestWakaTimeConfigRegexListsPreserveCommas(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "foo", "main.go")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "api_key", "waka_test")
	cfg.Set("settings", "exclude", `foo,bar\.go$`)
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{"--config", config, "--entity", file})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildHeartbeat(opts); err != nil {
		t.Fatalf("comma regex should be preserved as one pattern like WakaTime, got %v", err)
	}
}

func TestProjectConfigOverridesHeartbeatFlags(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "project")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	projectConfig := Config{Sections: map[string]map[string]string{}}
	projectConfig.Set("settings", "api_key", "waka_project")
	projectConfig.Set("settings", "api_url", "http://project.example/api/v1")
	projectConfig.Set("settings", "exclude_unknown_project", "false")
	projectConfig.Set("settings", "include_only_with_project_file", "false")
	projectConfig.Set("settings", "hide_file_names", "false")
	projectConfig.Set("settings", "include", ".*\\.go$")
	projectConfig.Set("settings", "exclude", "vendor")
	if err := projectConfig.Write(filepath.Join(project, ".wakatime")); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{
		"--entity", file,
		"--key", "waka_flag",
		"--api-url", "http://flag.example/api/v1",
		"--exclude-unknown-project",
		"--include-only-with-project-file",
		"--hide-file-names", "true",
		"--include", ".*\\.ts$",
		"--exclude", "node_modules",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_project" || opts.APIURL != "http://project.example/api/v1" {
		t.Fatalf("unexpected project API config: key=%q url=%q", opts.APIKey, opts.APIURL)
	}
	if opts.ExcludeUnknownProject || opts.IncludeOnlyProjectFile || opts.HideFileNames != "false" {
		t.Fatalf("unexpected project heartbeat overrides: %#v", opts)
	}
	if strings.Join(opts.Include, ",") != `.*\.go$` || strings.Join(opts.Exclude, ",") != "vendor" {
		t.Fatalf("unexpected project filters: include=%v exclude=%v", opts.Include, opts.Exclude)
	}
}

func TestProjectConfigGuessLanguageTakesPrecedenceOverFlag(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "project")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	projectConfig := Config{Sections: map[string]map[string]string{}}
	projectConfig.Set("settings", "guess_language", "false")
	if err := projectConfig.Write(filepath.Join(project, ".wakatime")); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{
		"--entity", file,
		"--guess-language",
		"--key", "waka_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.GuessLanguage {
		t.Fatal("project config guess_language=false should override --guess-language")
	}
}

func TestProjectConfigDoesNotPromoteBaseConfigOverFlags(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "project")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mainConfig := filepath.Join(dir, ".wakatime.cfg")
	main := Config{Sections: map[string]map[string]string{}}
	main.Set("settings", "api_key", "waka_main")
	main.Set("settings", "api_url", "http://main.example/api/v1")
	main.Set("settings", "include", ".*\\.go$")
	if err := main.Write(mainConfig); err != nil {
		t.Fatal(err)
	}
	projectConfig := Config{Sections: map[string]map[string]string{}}
	projectConfig.Set("settings", "hide_dependencies", "true")
	if err := projectConfig.Write(filepath.Join(project, ".wakatime")); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{
		"--config", mainConfig,
		"--entity", file,
		"--key", "waka_flag",
		"--api-url", "http://flag.example/api/v1",
		"--include", ".*\\.ts$",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_flag" || opts.APIURL != "http://flag.example/api/v1" {
		t.Fatalf("base config overrode flags: key=%q url=%q", opts.APIKey, opts.APIURL)
	}
	if strings.Join(opts.Include, ",") != `.*\.ts$,.*\.go$` {
		t.Fatalf("base include should compose with flag: %v", opts.Include)
	}
	if opts.HideDependencies != "true" {
		t.Fatalf("project hide_dependencies not applied: %q", opts.HideDependencies)
	}
}

func TestFilterAndSanitizeBooleanFlagsFalseTakePrecedenceOverConfig(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "exclude_unknown_project", "true")
	cfg.Set("settings", "include_only_with_project_file", "true")
	cfg.Set("settings", "hide_project_folder", "true")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{
		"--config", config,
		"--exclude-unknown-project=false",
		"--include-only-with-project-file=false",
		"--hide-project-folder=false",
		"--key", "waka_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.ExcludeUnknownProject || opts.IncludeOnlyProjectFile || opts.HideProjectFolder {
		t.Fatalf("explicit false filter/sanitize flags were overridden by config: %#v", opts)
	}
}
