package stintcli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestProjectRootCountMatchesWakaTimeSlashCounting(t *testing.T) {
	tests := map[string]int{
		"/":                  1,
		"/home":              2,
		"/home/user":         3,
		"/home/user/project": 4,
		`C:\folder\project`:  3,
		`\\wsl$/Ubuntu-22.04/home/folder/project`: 5,
	}
	for root, want := range tests {
		got := projectRootCount(root)
		if got == nil || *got != want {
			t.Fatalf("projectRootCount(%q) = %#v, want %d", root, got, want)
		}
	}
}

func TestBuildHeartbeatEntityProjectFilePrecedesProjectFolderOverrideLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	projectOverrideDir := filepath.Join(dir, "configured-project")
	entityProjectDir := filepath.Join(dir, "entity-project")
	if err := os.MkdirAll(projectOverrideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(entityProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectOverrideDir, ".wakatime-project"), []byte("configured\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entityProjectDir, ".wakatime-project"), []byte("entity\nbranch\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(entityProjectDir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", ProjectFolder: projectOverrideDir})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Project != "entity" || hb.Branch != "branch" {
		t.Fatalf("entity .wakatime-project should precede --project-folder override: %#v", hb)
	}
}

func TestBuildHeartbeatDetectsMercurialProject(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".hg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hg", "branch"), []byte("feature\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "main.py")
	if err := os.WriteFile(file, []byte("import flask\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Project != filepath.Base(dir) || hb.Branch != "feature" {
		t.Fatalf("unexpected hg project/branch: %#v", hb)
	}
}

func TestBuildHeartbeatSanitizesFileWithoutClearingExplicitBranchAndDependenciesPatterns(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\nimport \"github.com/acme/pkg\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{
		Category:         "coding",
		Branch:           "main",
		Entity:           file,
		EntityType:       "file",
		HideBranchNames:  `not_matching`,
		HideDependencies: `not_matching`,
		HideFileNames:    `main\.go$`,
		Write:            true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Entity != "HIDDEN.go" || hb.Branch != "main" || strings.Join(hb.Dependencies, ",") != "github.com/acme/pkg" {
		t.Fatalf("unexpected sanitized heartbeat: %#v", hb)
	}
	if hb.Lines != nil || hb.ProjectRootCount != nil {
		t.Fatalf("file sanitization should clear position metadata: %#v", hb)
	}
}

func TestBuildHeartbeatHideProjectNamesCreatesWakaTimeProjectAlias(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\nimport \"github.com/acme/pkg\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{
		Category:         "coding",
		CursorPosition:   9,
		Entity:           file,
		EntityType:       "file",
		HideProjectNames: `main\.go$`,
		LineNumber:       1,
		LinesInFile:      2,
		Project:          "stint",
		Write:            true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Project == "" || hb.Project == "stint" {
		t.Fatalf("project name should be obfuscated in CLI payload: %#v", hb)
	}
	projectFile := filepath.Join(dir, ".wakatime-project")
	data, err := os.ReadFile(projectFile)
	if err != nil {
		t.Fatalf("expected .wakatime-project alias: %v", err)
	}
	if strings.TrimSpace(string(data)) != hb.Project {
		t.Fatalf(".wakatime-project = %q, heartbeat project = %q", strings.TrimSpace(string(data)), hb.Project)
	}
	if hb.Entity != file || hb.CursorPosition != nil || hb.LineNumber != nil || hb.Lines != nil || hb.ProjectRootCount != nil {
		t.Fatalf("unexpected project sanitization: %#v", hb)
	}
	if strings.Join(hb.Dependencies, ",") != "github.com/acme/pkg" {
		t.Fatalf("dependencies should remain unless dependency hiding matches: %#v", hb.Dependencies)
	}

	hb, err = BuildHeartbeat(Options{
		Category:         "coding",
		Entity:           file,
		EntityType:       "file",
		HideProjectNames: `main\.go$`,
		Project:          "stint",
		Write:            true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Project != strings.TrimSpace(string(data)) {
		t.Fatalf("existing .wakatime-project alias was not reused: %#v", hb)
	}
}

func TestBuildHeartbeatHidesProjectFolderWithoutDetectedRoot(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "unknown", "src")
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
	})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Entity != "main.go" || hb.ProjectRootCount != nil {
		t.Fatalf("expected rootless hide_project_folder to send only filename, got %#v", hb)
	}
}

func TestBuildHeartbeatFormatsRelativeProjectFolderOverrideLikeWakaTime(t *testing.T) {
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
	t.Chdir(dir)

	hb, err := BuildHeartbeat(Options{
		Entity:            file,
		EntityType:        "file",
		Category:          "coding",
		HideProjectFolder: true,
		ProjectFolder:     "project",
	})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Entity != filepath.Join("src", "main.go") || hb.ProjectRootCount != nil {
		t.Fatalf("expected relative project folder override to be formatted, got %#v", hb)
	}
}

func TestRunHeartbeatNormalizesWakaTimeEndpointAPIURL(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()
	if err := Run([]string{
		"heartbeat",
		"--api-url", server.URL + "/api/v1/users/current/heartbeats.bulk",
		"--key", "waka_test",
		"--entity", file,
		"--offline-queue-file", filepath.Join(dir, "queue.bdb"),
		"--heartbeat-rate-limit-seconds", "0",
		"--sync-ai-disabled",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/users/current/heartbeats.bulk" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestAPIURLFanoutNormalizesWakaTimeEndpointURLs(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths[r.Header.Get("Authorization")] = r.URL.Path
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
	cfg.Set("api_urls", ".*main\\.go$", server.URL+"/api/v1/users/current/heartbeats.bulk|waka_fanout")
	if err := cfg.Write(config); err != nil {
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
	if paths[basicAuthHeader("waka_default")] != "/api/v1/users/current/heartbeats.bulk" || paths[basicAuthHeader("waka_fanout")] != "/api/v1/users/current/heartbeats.bulk" {
		t.Fatalf("unexpected fanout paths: %#v", paths)
	}
}

func TestProjectMapExpandsRegexCaptureGroups(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "client42")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(projectDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(projectDir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("projectmap", regexp.QuoteMeta(filepath.Join(dir, "client"))+`([0-9]+)`, "client-{0}-{project}")
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Project != "client-42-client42" {
		t.Fatalf("project = %q", hb.Project)
	}
}

func TestProjectMapUsesFirstMatchingEntryLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "src", "main.go")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, ".wakatime.cfg")
	body := "[projectmap]\n" +
		regexp.QuoteMeta(filepath.Join(dir, "src")) + " = broad-first\n" +
		regexp.QuoteMeta(file) + " = narrow-second\n"
	if err := os.WriteFile(config, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", Config: cfg})
		if err != nil {
			t.Fatal(err)
		}
		if hb.Project != "broad-first" {
			t.Fatalf("projectmap should use first matching entry, got %q on iteration %d", hb.Project, i)
		}
	}
}

func TestAPIURLsURLOnlyValueUsesEffectiveDefaultKeyLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("project_api_key", `main\.go$`, "waka_project")
	cfg.Set("api_urls", `main\.go$`, "http://secondary.example/api/v1")

	targets, err := heartbeatTargets(Options{
		APIURL: "http://default.example/api/v1",
		APIKey: "waka_default",
		Config: cfg,
	}, file)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected default and secondary targets, got %#v", targets)
	}
	if targets[0].APIURL != "http://default.example/api/v1" || targets[0].APIKey != "waka_project" {
		t.Fatalf("unexpected default target: %#v", targets)
	}
	if targets[1].APIURL != "http://secondary.example/api/v1" || targets[1].APIKey != "waka_project" {
		t.Fatalf("URL-only api_urls should use effective default key, got %#v", targets)
	}
}

func TestAPIURLsPipeWithEmptyKeyIsRejectedLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("api_urls", `main\.go$`, "http://secondary.example/api/v1|")

	_, err := heartbeatTargets(Options{
		APIURL: "http://default.example/api/v1",
		APIKey: "waka_default",
		Config: cfg,
	}, file)
	if err == nil {
		t.Fatal("expected empty api_urls key to be rejected")
	}
	if !strings.Contains(err.Error(), "invalid api key format in api_urls") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectAPIKeyEmptyValueIsRejectedLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("project_api_key", `main\.go$`, "")

	_, err := heartbeatTargets(Options{
		APIURL: "http://default.example/api/v1",
		APIKey: "waka_default",
		Config: cfg,
	}, file)
	if err == nil {
		t.Fatal("expected empty project_api_key value to be rejected")
	}
	if !strings.Contains(err.Error(), "invalid api key format for") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGitSubmoduleProjectMapAndBranch(t *testing.T) {
	dir := t.TempDir()
	submoduleRoot := filepath.Join(dir, "repo", "lib", "billing")
	gitdir := filepath.Join(dir, "repo", ".git", "modules", "lib", "billing")
	if err := os.MkdirAll(submoduleRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(gitdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(submoduleRoot, ".git"), []byte("gitdir: ../../.git/modules/lib/billing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitdir, "HEAD"), []byte("ref: refs/heads/invoices\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(submoduleRoot, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("git_submodule_projectmap", regexp.QuoteMeta(filepath.Join("lib", "billing"))+`$`, "submodule-{project}")
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Project != "submodule-billing" || hb.Branch != "invoices" {
		t.Fatalf("unexpected submodule heartbeat: %#v", hb)
	}
}

func TestGitSubmoduleCanBeDisabled(t *testing.T) {
	dir := t.TempDir()
	submoduleRoot := filepath.Join(dir, "repo", "lib", "billing")
	gitdir := filepath.Join(dir, "repo", ".git", "modules", "lib", "billing")
	if err := os.MkdirAll(submoduleRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(gitdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(submoduleRoot, ".git"), []byte("gitdir: ../../.git/modules/lib/billing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitdir, "HEAD"), []byte("ref: refs/heads/invoices\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(submoduleRoot, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("git", "submodules_disabled", "billing")
	cfg.Set("git_submodule_projectmap", "billing", "should-not-apply")
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Project != "billing" || hb.Branch == "invoices" {
		t.Fatalf("expected submodule detection disabled, got %#v", hb)
	}
}

func TestIncludeOnlyWithProjectFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", IncludeOnlyProjectFile: true})
	if err == nil || !strings.Contains(err.Error(), ".wakatime-project") {
		t.Fatalf("expected include-only project file error, got %v", err)
	}
}

func TestParseCommonRejectsInvalidAPIURLLikeWakaTime(t *testing.T) {
	_, err := parseCommon([]string{
		"--api-url", "http://in valid",
		"--key", "waka_test",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid api url") {
		t.Fatalf("expected invalid api url error, got %v", err)
	}
}

func TestParseCommonDisablesNegativeProjectHeartbeatRateLimitLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "project")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	entity := filepath.Join(project, "main.go")
	if err := os.WriteFile(entity, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	projectConfig := Config{Sections: map[string]map[string]string{}}
	projectConfig.Set("settings", "heartbeat_rate_limit_seconds", "-1")
	if err := projectConfig.Write(filepath.Join(project, ".wakatime")); err != nil {
		t.Fatal(err)
	}

	opts, err := parseCommon([]string{
		"--entity", entity,
		"--key", "waka_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.HeartbeatRateLimit != 0 {
		t.Fatalf("negative project heartbeat rate limit should disable rate limiting, got %d", opts.HeartbeatRateLimit)
	}
}

func TestRunFileExpertsFiltersMissingProjectRootCountLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := Run([]string{"--file-experts", "--entity", file, "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("file-experts should not post a heartbeat without project_root_count")
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("expected no file-experts output for invalid heartbeat, got %q", out.String())
	}
}

func TestRunProjectCommitsUseExpectedEndpoints(t *testing.T) {
	responses := map[string]string{
		"/api/v1/users/current/projects/stint%20api/commits?branch=main+branch&page=2": `{"commits":[{"hash":"abcdef1234567890"}],"status":"ok"}`,
		"/api/v1/users/current/projects/stint%20api/commits/abcdef1":                   `{"commit":{"hash":"abcdef1234567890"},"status":"ok"}`,
	}
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
		paths = append(paths, path)
		body, ok := responses[path]
		if !ok {
			t.Fatalf("unexpected endpoint path: %s", path)
		}
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "list", args: []string{"projects", "stint api", "commits", "--branch", "main branch", "--page", "2"}, want: `"commits"`},
		{name: "detail", args: []string{"projects", "stint api", "commits", "abcdef1"}, want: `"hash":"abcdef1234567890"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append(append([]string{}, tt.args...), "--api-url", server.URL+"/api/v1", "--key", "waka_test", "--output", "raw-json")
			var out bytes.Buffer
			if err := Run(args, nil, &out, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), tt.want) {
				t.Fatalf("output missing %q: %q", tt.want, out.String())
			}
		})
	}
	for _, want := range []string{
		"/api/v1/users/current/projects/stint%20api/commits?branch=main+branch&page=2",
		"/api/v1/users/current/projects/stint%20api/commits/abcdef1",
	} {
		if !slices.Contains(paths, want) {
			t.Fatalf("expected endpoint %s in %#v", want, paths)
		}
	}
}
