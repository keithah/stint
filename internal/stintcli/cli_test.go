package stintcli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
	"golang.org/x/crypto/ssh"
	_ "modernc.org/sqlite"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "stintcli-test-home-*")
	if err != nil {
		panic(err)
	}
	oldHome, hadHome := os.LookupEnv("WAKATIME_HOME")
	oldUserHome, hadUserHome := os.LookupEnv("HOME")
	if err := os.Setenv("WAKATIME_HOME", dir); err != nil {
		panic(err)
	}
	if err := os.Setenv("HOME", dir); err != nil {
		panic(err)
	}
	code := m.Run()
	if hadHome {
		_ = os.Setenv("WAKATIME_HOME", oldHome)
	} else {
		_ = os.Unsetenv("WAKATIME_HOME")
	}
	if hadUserHome {
		_ = os.Setenv("HOME", oldUserHome)
	} else {
		_ = os.Unsetenv("HOME")
	}
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

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

func TestVersionVerboseOutputMatchesWakaTimeShape(t *testing.T) {
	for _, args := range [][]string{
		{"--version", "--verbose"},
		{"version", "--verbose"},
	} {
		var out bytes.Buffer
		if err := Run(args, nil, &out, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		output := out.String()
		for _, want := range []string{"stint-cli", "Version:", "Commit:", "Built:", "OS/Arch:"} {
			if !strings.Contains(output, want) {
				t.Fatalf("Run(%v) verbose version output missing %q: %q", args, want, output)
			}
		}
	}
}

func TestVersionMetadataCanBeInjectedAtBuildTime(t *testing.T) {
	oldVersion, oldCommit, oldBuildDate := versionValue, commitValue, buildDateValue
	versionValue = "1.2.3"
	commitValue = "abc123"
	buildDateValue = "2026-06-28T12:00:00Z"
	t.Cleanup(func() {
		versionValue = oldVersion
		commitValue = oldCommit
		buildDateValue = oldBuildDate
	})

	if Version() != "1.2.3" {
		t.Fatalf("Version() = %q", Version())
	}
	output := verboseVersion()
	for _, want := range []string{"Version: 1.2.3", "Commit: abc123", "Built: 2026-06-28T12:00:00Z"} {
		if !strings.Contains(output, want) {
			t.Fatalf("verbose version output missing %q: %q", want, output)
		}
	}
}

func TestUserAgentUsesWakaTimeCompatibleShape(t *testing.T) {
	oldVersion := versionValue
	versionValue = "1.2.3"
	t.Cleanup(func() { versionValue = oldVersion })

	ua := userAgent("vim-wakatime/10.0.0")
	if !strings.HasPrefix(ua, "stint-cli/1.2.3 (") {
		t.Fatalf("user agent prefix = %q", ua)
	}
	for _, want := range []string{runtime.GOOS, runtime.GOARCH, runtime.Version(), "vim-wakatime/10.0.0"} {
		if !strings.Contains(ua, want) {
			t.Fatalf("user agent missing %q: %q", want, ua)
		}
	}
	if strings.Contains(ua, "stint-cli/1.2.3 ("+runtime.GOOS+"-"+runtime.GOARCH+")") {
		t.Fatalf("user agent still uses short legacy shape: %q", ua)
	}
}

func TestUserAgentDefaultsUnknownPluginLikeWakaTime(t *testing.T) {
	if ua := userAgent(""); !strings.HasSuffix(ua, " Unknown/0") {
		t.Fatalf("default user agent = %q", ua)
	}
}

func TestUserAgentNormalizesMissingPluginVersionsLikeWakaTime(t *testing.T) {
	ua := userAgent("Claude/ macos-wakatime/5.28.3")
	if !strings.HasSuffix(ua, " Claude/unknown macos-wakatime/5.28.3") {
		t.Fatalf("user agent did not normalize missing plugin version: %q", ua)
	}
}

func TestHelpListsCurrentWakaTimeFlags(t *testing.T) {
	var out bytes.Buffer
	if err := Run([]string{"--help"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	for _, flag := range []string{
		"--ai-line-changes",
		"--alternate-branch",
		"--alternate-language",
		"--alternate-project",
		"--api-url",
		"--apiurl",
		"--category",
		"--config",
		"--config-read",
		"--config-section",
		"--config-write",
		"--cursorpos",
		"--disable-offline",
		"--disableoffline",
		"--entity",
		"--entity-type",
		"--exclude",
		"--exclude-unknown-project",
		"--extra-heartbeats",
		"--file",
		"--file-experts",
		"--guess-language",
		"--heartbeat-rate-limit-seconds",
		"--hide-branch-names",
		"--hide-file-names",
		"--hide-filenames",
		"--hidefilenames",
		"--hide-project-folder",
		"--hide-project-names",
		"--hostname",
		"--human-line-changes",
		"--include",
		"--include-only-with-project-file",
		"--internal-config",
		"--is-unsaved-entity",
		"--key",
		"--language",
		"--lineno",
		"--lines-in-file",
		"--local-file",
		"--log-file",
		"--logfile",
		"--log-to-stdout",
		"--metrics",
		"--no-ssl-verify",
		"--offline-count",
		"--offline-queue-file",
		"--offline-queue-file-legacy",
		"--output",
		"--plugin",
		"--print-offline-heartbeats",
		"--project",
		"--project-folder",
		"--proxy",
		"--send-diagnostics-on-errors",
		"--ssl-certs-file",
		"--sync-ai-activity",
		"--sync-ai-after",
		"--sync-ai-disabled",
		"--sync-ai-disable",
		"--sync-ai-heartbeats",
		"--sync-offline-activity",
		"--time",
		"--timeout",
		"--today",
		"--today-goal",
		"--today-hide-categories",
		"--today-max-categories",
		"--user-agent",
		"--verbose",
		"--version",
		"--write",
	} {
		if !strings.Contains(out.String(), flag) {
			t.Fatalf("help output missing WakaTime-compatible flag %s:\n%s", flag, out.String())
		}
	}
	for _, command := range []string{"stint collect", "stint heartbeats", "stint all-time", "stint machine-names", "stint user-agents", "stint external-durations", "stint usage-events", "stint data-dumps", "stint custom-rules", "stint import wakatime", "stint custom-pricing", "stint billing-prefs", "stint ai-costs", "stint leaderboards", "stint events", "stint insights", "stint durations", "stint summaries"} {
		if !strings.Contains(out.String(), command) {
			t.Fatalf("help output missing %s command:\n%s", command, out.String())
		}
	}
	wakaSection := between(out.String(), "WakaTime-compatible root flags:", "Stint extensions:")
	extensionSection := after(out.String(), "Stint extensions:")
	for _, flag := range []string{"--apiurl", "--disableoffline", "--hide-dependencies", "--logfile", "--offline-queue-file", "--offline-queue-file-legacy", "--sync-ai-heartbeats", "--today-max-categories"} {
		if !strings.Contains(wakaSection, flag) {
			t.Fatalf("WakaTime-compatible section missing %s:\n%s", flag, wakaSection)
		}
	}
	for _, flag := range []string{"--apiurl", "--hide-dependencies", "--offline-queue-file", "--offline-queue-file-legacy", "--today-max-categories"} {
		if strings.Contains(extensionSection, flag) {
			t.Fatalf("WakaTime-compatible %s flag should not be listed as a Stint extension:\n%s", flag, extensionSection)
		}
	}
}

func between(source, start, end string) string {
	_, tail, ok := strings.Cut(source, start)
	if !ok {
		return ""
	}
	head, _, ok := strings.Cut(tail, end)
	if !ok {
		return tail
	}
	return head
}

func after(source, marker string) string {
	_, tail, ok := strings.Cut(source, marker)
	if !ok {
		return ""
	}
	return tail
}

func TestUserAgentPlatformIncludesKernelWhenAvailable(t *testing.T) {
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		t.Skip("uname unavailable")
	}
	kernel := strings.TrimSpace(string(out))
	if kernel == "" {
		t.Skip("uname returned empty kernel")
	}
	want := runtime.GOOS + "-" + kernel + "-" + runtime.GOARCH
	if got := userAgentPlatform(); got != want {
		t.Fatalf("userAgentPlatform() = %q, want %q", got, want)
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

func TestConfigParseErrorFallsBackToOfflineQueueLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	entity := filepath.Join(dir, "main.go")
	if err := os.WriteFile(entity, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	queue := filepath.Join(dir, "offline.bdb")
	err := Run([]string{
		"--config", dir,
		"--entity", entity,
		"--key", "waka_test",
		"--offline-queue-file", queue,
		"--sync-ai-disabled",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected config parse error")
	}
	count, countErr := CountQueue(queue)
	if countErr != nil {
		t.Fatal(countErr)
	}
	if count != 1 {
		t.Fatalf("queued fallback heartbeats = %d, want 1", count)
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

func TestRootEmptyEntityFlagDispatchesHeartbeatLikeWakaTime(t *testing.T) {
	err := Run([]string{
		"--entity=",
		"--key", "waka_test",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected empty entity error")
	}
	if strings.Contains(err.Error(), "provide a command") {
		t.Fatalf("empty --entity should dispatch heartbeat path, got %v", err)
	}
	if !strings.Contains(err.Error(), "--entity is required") {
		t.Fatalf("expected heartbeat entity error, got %v", err)
	}
}

func TestRootEmptyFileAliasDispatchesHeartbeatLikeWakaTime(t *testing.T) {
	err := Run([]string{
		"--file=",
		"--key", "waka_test",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected empty entity error")
	}
	if strings.Contains(err.Error(), "provide a command") {
		t.Fatalf("empty --file should dispatch heartbeat path, got %v", err)
	}
	if !strings.Contains(err.Error(), "--entity is required") {
		t.Fatalf("expected heartbeat entity error, got %v", err)
	}
}

func TestRootEmptyEntityFallsBackToFileAliasLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
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
		"--entity=",
		"--file", file,
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--heartbeat-rate-limit-seconds", "0",
		"--sync-ai-disabled",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 1 || posted[0].Entity != file {
		t.Fatalf("posted = %#v", posted)
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

func TestBuildHeartbeatDetectsProjectLanguageAndLines(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".wakatime-project"), []byte("custom/{project}\nmain\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", Write: true})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Project != "custom/"+filepath.Base(dir) {
		t.Fatalf("project = %q", hb.Project)
	}
	if hb.Branch != "main" {
		t.Fatalf("branch = %q", hb.Branch)
	}
	if hb.Language != "Go" {
		t.Fatalf("language = %q", hb.Language)
	}
	if hb.Lines == nil || *hb.Lines != 2 {
		t.Fatalf("lines = %#v", hb.Lines)
	}
	if hb.ProjectRootCount == nil || *hb.ProjectRootCount == 0 {
		t.Fatalf("project root count = %#v", hb.ProjectRootCount)
	}
	if hb.UserAgent == "" {
		t.Fatalf("expected user agent")
	}
}

func TestBuildHeartbeatSkipsLineCountingForLargeFilesLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "large.go")
	body := bytes.Repeat([]byte("package main\n"), (5*1024*1024/len("package main\n"))+2)
	if err := os.WriteFile(file, body, 0o600); err != nil {
		t.Fatal(err)
	}

	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Lines != nil {
		t.Fatalf("large files should not be counted automatically, got %#v", hb.Lines)
	}
}

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

func TestBuildHeartbeatFormatsLocalFileEntityLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	realFile := filepath.Join(dir, "real.go")
	if err := os.WriteFile(realFile, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkFile := filepath.Join(dir, "link.go")
	if err := os.Symlink(realFile, linkFile); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	hb, err := BuildHeartbeat(Options{Entity: linkFile, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Entity != realFile {
		t.Fatalf("entity = %q, want %q", hb.Entity, realFile)
	}
}

func TestBuildHeartbeatModifiesXcodeBundleEntitiesLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	playground := filepath.Join(dir, "Demo.playground")
	if err := os.Mkdir(playground, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(playground, "Contents.swift"), []byte("print(\"hi\")\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: playground, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Entity != filepath.Join(playground, "Contents.swift") || hb.Language != "Swift" {
		t.Fatalf("playground heartbeat = %#v", hb)
	}

	project := filepath.Join(dir, "Demo.xcodeproj")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "project.pbxproj"), []byte("// !$*UTF8*$!\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err = BuildHeartbeat(Options{Entity: project, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Entity != filepath.Join(project, "project.pbxproj") {
		t.Fatalf("xcodeproj entity = %q", hb.Entity)
	}
}

func TestBuildHeartbeatGuessLanguageFromContents(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "script")
	if err := os.WriteFile(file, []byte("#!/usr/bin/env bash\necho hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Language != "" {
		t.Fatalf("language without guessing = %q", hb.Language)
	}
	hb, err = BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", GuessLanguage: true})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Language != "Bash" {
		t.Fatalf("language with guessing = %q", hb.Language)
	}
}

func TestBuildHeartbeatGuessLanguageFromVimModelineLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "script")
	if err := os.WriteFile(file, []byte("/* vim: tw=60 ft=python ts=2: */\nprint('hi')\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", GuessLanguage: true})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Language != "Python" {
		t.Fatalf("language = %q, want Python", hb.Language)
	}
}

func TestBuildHeartbeatDetectsCPlusPlusHeadersLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	header := filepath.Join(dir, "widget.h")
	if err := os.WriteFile(header, []byte("#pragma once\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "widget.cpp"), []byte("#include \"widget.h\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: header, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Language != "C++" {
		t.Fatalf("language = %q, want C++", hb.Language)
	}
}

func TestBuildHeartbeatDetectsMatlabMFilesLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "analysis.m")
	if err := os.WriteFile(file, []byte("disp('hi')\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sample.mat"), []byte("matlab data"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Language != "Matlab" {
		t.Fatalf("language = %q, want Matlab", hb.Language)
	}
}

func TestBuildHeartbeatDetectsForthFSFilesLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "stack.fs")
	if err := os.WriteFile(file, []byte(": square dup * ;\n\n\\ Forth line comment\n\n( stack effect comment )\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Language != "Forth" {
		t.Fatalf("language = %q, want Forth", hb.Language)
	}
}

func TestBuildHeartbeatDetectsFSharpFSFilesLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "program.fs")
	if err := os.WriteFile(file, []byte("let describe value =\n    match value with\n    | Some text -> text\n    | None -> \"missing\"\n\n// F# line comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Language != "F#" {
		t.Fatalf("language = %q, want F#", hb.Language)
	}
}

func TestBuildHeartbeatDetectsCMakeSpecialFilenameLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "CMmakeLists.txt")
	if err := os.WriteFile(file, []byte("cmake_minimum_required(VERSION 3.20)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Language != "CMake" {
		t.Fatalf("language = %q, want CMake", hb.Language)
	}
}

func TestBuildHeartbeatDetectsJSXFilesLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "component.jsx")
	if err := os.WriteFile(file, []byte("export function Component() { return <div />; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Language != "JSX" {
		t.Fatalf("language = %q, want JSX", hb.Language)
	}
}

func TestBuildHeartbeatNormalizesTopLanguageAliasesLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	tests := map[string]string{
		".ruby-version":         "Ruby",
		".Rprofile":             "S",
		"crontab":               "Crontab",
		"file.cfm":              "ColdFusion",
		"file.fhtml":            "Velocity",
		"file.fsi":              "F#",
		"file.gs":               "Gosu",
		"file.i":                "SWIG",
		"file.inc":              "Pawn",
		"file.j":                "Objective-J",
		"file.kif":              "newLisp",
		"file.lasso9":           "Lasso",
		"file.markdown":         "Markdown",
		"file.marko":            "Marko",
		"file.mustache":         "Mustache",
		"file.mo":               "Modelica",
		"file.pug":              "Pug",
		"file.re":               "Reason",
		"file.sketch":           "Sketch Drawing",
		"file.slim":             "Slim",
		"file.sublime-settings": "Sublime Text Config",
		"file.swg":              "SWIG",
		"file.svh":              "SystemVerilog",
		"file.txt":              "Text",
		"file.vue":              "Vue.js",
		"file.vm":               "Velocity",
		"file.xaml":             "XAML",
		"file.xpl":              "XSLT",
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			file := filepath.Join(dir, name)
			if err := os.WriteFile(file, []byte("sample\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
			if err != nil {
				t.Fatal(err)
			}
			if hb.Language != want {
				t.Fatalf("language = %q, want %s", hb.Language, want)
			}
		})
	}
}

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

func TestBuildHeartbeatKeepsRemoteEntityAndUsesLocalFileForStats(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".wakatime-project"), []byte("remote-project\nremote-dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	localFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(localFile, []byte("package main\nimport \"github.com/acme/pkg\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	remoteEntity := "ssh://user@example.org/home/me/project/main.go"
	hb, err := BuildHeartbeat(Options{
		Category:   "coding",
		Entity:     remoteEntity,
		EntityType: "file",
		LocalFile:  localFile,
		Write:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Entity != "ssh://example.org/home/me/project/main.go" {
		t.Fatalf("entity = %q", hb.Entity)
	}
	if hb.Project != "remote-project" || hb.Branch != "remote-dev" {
		t.Fatalf("unexpected project/branch from local file: %#v", hb)
	}
	if hb.Language != "Go" {
		t.Fatalf("language = %q", hb.Language)
	}
	if hb.Lines == nil || *hb.Lines != 2 {
		t.Fatalf("lines = %#v", hb.Lines)
	}
	if strings.Join(hb.Dependencies, ",") != "github.com/acme/pkg" {
		t.Fatalf("dependencies = %#v", hb.Dependencies)
	}
}

func TestBuildHeartbeatDownloadsRemoteEntityWithoutLocalFile(t *testing.T) {
	original := remoteFileDownload
	var downloadedPath string
	remoteFileDownload = func(client remoteFileClient, localPath string) error {
		if client.Host != "example.org" || client.Path != "/home/me/project/main.go" {
			t.Fatalf("unexpected remote client: %#v", client)
		}
		downloadedPath = localPath
		return os.WriteFile(localPath, []byte("package main\nimport \"github.com/acme/pkg\"\n"), 0o600)
	}
	t.Cleanup(func() { remoteFileDownload = original })

	remoteEntity := "sftp://user@example.org/home/me/project/main.go"
	hb, err := BuildHeartbeat(Options{
		Category:   "coding",
		Entity:     remoteEntity,
		EntityType: "file",
		Write:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Entity != "sftp://example.org/home/me/project/main.go" || !hb.IsWrite {
		t.Fatalf("unexpected remote heartbeat: %#v", hb)
	}
	if hb.Language != "Go" || hb.Lines == nil || *hb.Lines != 2 {
		t.Fatalf("remote stats not applied: %#v", hb)
	}
	if strings.Join(hb.Dependencies, ",") != "github.com/acme/pkg" {
		t.Fatalf("dependencies = %#v", hb.Dependencies)
	}
	if downloadedPath == "" {
		t.Fatalf("expected remote downloader to run")
	}
	if _, err := os.Stat(downloadedPath); !os.IsNotExist(err) {
		t.Fatalf("remote temp file was not cleaned up: %s err=%v", downloadedPath, err)
	}
}

func TestBuildHeartbeatRemoteEntitySkipsProjectFileFilterLikeWakaTime(t *testing.T) {
	original := remoteFileDownload
	remoteFileDownload = func(_ remoteFileClient, localPath string) error {
		return os.WriteFile(localPath, []byte("package main\n"), 0o600)
	}
	t.Cleanup(func() { remoteFileDownload = original })
	hb, err := BuildHeartbeat(Options{
		Entity:                 "ssh://example.test/home/me/main.go",
		EntityType:             "file",
		IncludeOnlyProjectFile: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Entity != "ssh://example.test/home/me/main.go" {
		t.Fatalf("entity = %q", hb.Entity)
	}
}

func TestBuildHeartbeatUnsavedRemoteEntityDoesNotBypassProjectFileFilterLikeWakaTime(t *testing.T) {
	_, err := BuildHeartbeat(Options{
		Entity:                 "ssh://example.test/home/me/main.go",
		EntityType:             "file",
		IsUnsavedEntity:        true,
		IncludeOnlyProjectFile: true,
		Category:               "coding",
	})
	if err == nil || !strings.Contains(err.Error(), "project has no .wakatime-project") {
		t.Fatalf("expected unsaved remote-looking entity to require project file like WakaTime, got %v", err)
	}
}

func TestParseRemoteFileClient(t *testing.T) {
	client, err := parseRemoteFileClient("ssh://alice:secret@example.org:222/home/me/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if client.User != "alice" || client.Pass != "secret" || client.Host != "example.org" || client.Port != 222 || client.Path != "/home/me/main.go" {
		t.Fatalf("unexpected remote client: %#v", client)
	}
	client, err = parseRemoteFileClient("sftp://example.org/home/me/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if client.Port != 22 || client.User != "" || client.Path != "/home/me/main.go" {
		t.Fatalf("unexpected default remote client: %#v", client)
	}
	client, err = parseRemoteFileClient("sftp://example.org")
	if err != nil {
		t.Fatal(err)
	}
	if client.Host != "example.org" || client.Port != 22 || client.Path != "" {
		t.Fatalf("unexpected host-only remote client: %#v", client)
	}
}

func TestParseRemoteFileClientUsesSSHConfigHostAliases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte("Host prod\n  HostName prod.example.org\n  User deploy\n  Port 2222\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := parseRemoteFileClient("sftp://prod/home/me/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if client.Host != "prod.example.org" || client.User != "deploy" || client.Port != 2222 || client.Path != "/home/me/main.go" {
		t.Fatalf("unexpected ssh config remote client: %#v", client)
	}
}

func TestParseRemoteFileClientUsesDerivedHostSSHConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	config := "Host prod\n  HostName prod.example.org\nHost prod.example.org\n  User deploy\n  Port 2222\n"
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := parseRemoteFileClient("ssh://prod/home/me/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if client.Host != "prod.example.org" || client.User != "deploy" || client.Port != 2222 {
		t.Fatalf("unexpected derived-host ssh config remote client: %#v", client)
	}
}

func TestRemoteSSHIdentityFilesUseSSHConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	identityFile := filepath.Join(sshDir, "prod_key")
	if err := os.WriteFile(identityFile, []byte("not a real key"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := "Host prod\n  HostName prod.example.org\nHost prod.example.org\n  IdentityFile " + identityFile + "\n"
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := parseRemoteFileClient("ssh://prod/home/me/main.go")
	if err != nil {
		t.Fatal(err)
	}
	identityFiles := remoteSSHIdentityFiles(client)
	if len(identityFiles) == 0 || identityFiles[0] != identityFile {
		t.Fatalf("identity files = %#v, want first %q", identityFiles, identityFile)
	}
}

func TestRemoteKnownHostsFilesUseSSHConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	knownHostsFile := filepath.Join(sshDir, "prod_known_hosts")
	if err := os.WriteFile(knownHostsFile, []byte("prod.example.org ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDummyKeyForPathSelectionOnly\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := "Host prod\n  HostName prod.example.org\nHost prod.example.org\n  UserKnownHostsFile " + knownHostsFile + "\n"
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := parseRemoteFileClient("ssh://prod/home/me/main.go")
	if err != nil {
		t.Fatal(err)
	}
	files := remoteKnownHostsFiles(client)
	if len(files) == 0 || files[0] != knownHostsFile {
		t.Fatalf("known hosts files = %#v, want first %q", files, knownHostsFile)
	}
}

func TestRemoteHostKeyCallbackHonorsStrictHostKeyCheckingNo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	knownPublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	knownPublicKey, err := ssh.NewPublicKey(knownPublic)
	if err != nil {
		t.Fatal(err)
	}
	knownHostsLine := "other.example.org " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(knownPublicKey))) + "\n"
	if err := os.WriteFile(filepath.Join(sshDir, "known_hosts"), []byte(knownHostsLine), 0o600); err != nil {
		t.Fatal(err)
	}
	config := "Host prod\n  HostName prod.example.org\n  StrictHostKeyChecking no\n"
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	client, err := parseRemoteFileClient("ssh://prod/home/me/main.go")
	if err != nil {
		t.Fatal(err)
	}
	callback, err := remoteHostKeyCallback(client)
	if err != nil {
		t.Fatal(err)
	}
	if err := callback("prod.example.org:22", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}, publicKey); err != nil {
		t.Fatalf("StrictHostKeyChecking no should accept unknown key, got %v", err)
	}
}

func TestRemoteHostKeyCallbackRejectsMissingKnownHostWhenStrictYes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	config := "Host prod\n  HostName prod.example.org\n  StrictHostKeyChecking yes\n"
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := parseRemoteFileClient("ssh://prod/home/me/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remoteHostKeyCallback(client); err == nil || !strings.Contains(err.Error(), "known host key not found") {
		t.Fatalf("StrictHostKeyChecking yes without known host should fail like WakaTime, got %v", err)
	}
}

func TestRemoteHostKeyCallbackHonorsHostKeyAlias(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	knownHostsLine := "prod-alias " + strings.TrimSpace(string(ssh.MarshalAuthorizedKey(publicKey))) + "\n"
	if err := os.WriteFile(filepath.Join(sshDir, "known_hosts"), []byte(knownHostsLine), 0o600); err != nil {
		t.Fatal(err)
	}
	config := "Host prod\n  HostName prod.example.org\n  HostKeyAlias prod-alias\n"
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := parseRemoteFileClient("ssh://prod/home/me/main.go")
	if err != nil {
		t.Fatal(err)
	}
	callback, err := remoteHostKeyCallback(client)
	if err != nil {
		t.Fatal(err)
	}
	if err := callback("prod.example.org:22", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 22}, publicKey); err != nil {
		t.Fatalf("HostKeyAlias should match known_hosts alias, got %v", err)
	}
}

func TestRemoteDownloadFallbackPolicy(t *testing.T) {
	if shouldFallbackRemoteDownload(fmt.Errorf("ssh: handshake failed")) != true {
		t.Fatalf("expected generic ssh failure to allow scp fallback")
	}
	if shouldFallbackRemoteDownload(fmt.Errorf("ssh: host key mismatch")) != false {
		t.Fatalf("host key mismatch must not fall back to scp")
	}
	if shouldFallbackRemoteDownload(fmt.Errorf("knownhosts: key mismatch")) != false {
		t.Fatalf("knownhosts key mismatch must not fall back to scp")
	}
}

func TestParseCommonAcceptsExtendedMetadataAliases(t *testing.T) {
	opts, err := parseCommon([]string{
		"--model-name", "claude-sonnet-4.5",
		"--llm-provider", "anthropic",
		"--ai-agent-name", "claude",
		"--revision", "abc123",
		"--plugin-version", "1.0.0",
		"--editor", "vscode",
		"--editor-version", "2.0.0",
		"--metadata", `{"source":"test"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.AIModel != "claude-sonnet-4.5" || opts.AIProvider != "anthropic" || opts.AIAgent != "claude" || opts.CommitHash != "abc123" {
		t.Fatalf("unexpected aliases: %#v", opts)
	}
	if opts.PluginVersion != "1.0.0" || opts.Editor != "vscode" || opts.EditorVersion != "2.0.0" || opts.Metadata == "" {
		t.Fatalf("unexpected metadata aliases: %#v", opts)
	}
}

func TestParseCommonEmptyExtendedMetadataAliasesDoNotEraseEarlierValues(t *testing.T) {
	opts, err := parseCommon([]string{
		"--ai-model", "gpt-5-codex",
		"--ai-model-name", "",
		"--ai-provider", "openai",
		"--provider", "",
		"--ai-agent", "codex",
		"--ai-agent-name", "",
		"--commit-hash", "abc123",
		"--revision", "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.AIModel != "gpt-5-codex" {
		t.Fatalf("AIModel = %q", opts.AIModel)
	}
	if opts.AIProvider != "openai" {
		t.Fatalf("AIProvider = %q", opts.AIProvider)
	}
	if opts.AIAgent != "codex" {
		t.Fatalf("AIAgent = %q", opts.AIAgent)
	}
	if opts.CommitHash != "abc123" {
		t.Fatalf("CommitHash = %q", opts.CommitHash)
	}
}

func TestParseCommonFalseBooleanAliasesDoNotEraseEarlierValues(t *testing.T) {
	opts, err := parseCommon([]string{
		"--sync-ai-activity",
		"--sync-ai-heartbeats=false",
		"--sync-ai-disabled",
		"--sync-ai-disable=false",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.SyncAIActivity {
		t.Fatal("SyncAIActivity = false")
	}
	if !opts.SyncAIDisabled {
		t.Fatal("SyncAIDisabled = false")
	}
}

func TestParseCommonAcceptsAPIKeyAlias(t *testing.T) {
	opts, err := parseCommon([]string{
		"--api-key", "waka_alias",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_alias" {
		t.Fatalf("APIKey = %q", opts.APIKey)
	}

	opts, err = parseCommon([]string{
		"--api-key", "waka_alias",
		"--key", "waka_key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_key" {
		t.Fatalf("--key should take precedence over --api-key, got %q", opts.APIKey)
	}
}

func TestParseCommonEmptyPrimaryFlagsFallBackToDeprecatedAliasesLikeWakaTime(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "alias.log")
	opts, err := parseCommon([]string{
		"--key=",
		"--api-key", "waka_alias",
		"--api-url=",
		"--apiurl", "http://alias.example/api/v1",
		"--log-file=",
		"--logfile", logFile,
		"--hide-file-names=",
		"--hidefilenames", "true",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_alias" {
		t.Fatalf("APIKey = %q", opts.APIKey)
	}
	if opts.APIURL != "http://alias.example/api/v1" {
		t.Fatalf("APIURL = %q", opts.APIURL)
	}
	if opts.LogFile != logFile {
		t.Fatalf("LogFile = %q", opts.LogFile)
	}
	if opts.HideFileNames != "true" {
		t.Fatalf("HideFileNames = %q", opts.HideFileNames)
	}
}

func TestParseCommonUsesWakaTimeAPIKeyEnvFallback(t *testing.T) {
	t.Setenv("WAKATIME_API_KEY", "waka_env")
	config := filepath.Join(t.TempDir(), "missing.cfg")
	opts, err := parseCommon([]string{"--config", config})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_env" {
		t.Fatalf("APIKey = %q", opts.APIKey)
	}

	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "api_key", "waka_config")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	opts, err = parseCommon([]string{"--config", config})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_config" {
		t.Fatalf("config api_key should take precedence over WAKATIME_API_KEY, got %q", opts.APIKey)
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

func TestParseCommonUsesSettingsOfflineUnlessDisableFlagSet(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "offline", "false")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}

	opts, err := parseCommon([]string{"--config", config})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.DisableOffline {
		t.Fatalf("expected settings.offline=false to disable offline queue")
	}

	opts, err = parseCommon([]string{"--config", config, "--disable-offline=false"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.DisableOffline {
		t.Fatalf("explicit --disable-offline=false should take precedence over settings.offline=false")
	}
}

func TestParseCommonUsesProjectSettingsOffline(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "offline", "true")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	projectCfg := Config{Sections: map[string]map[string]string{}}
	projectCfg.Set("settings", "offline", "false")
	if err := projectCfg.Write(filepath.Join(dir, ".wakatime")); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	opts, err := parseCommon([]string{"--config", config, "--entity", file})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.DisableOffline {
		t.Fatalf("expected project settings.offline=false to disable offline queue")
	}
}

func TestClientLogToStdoutOverridesLogFile(t *testing.T) {
	var logOut bytes.Buffer
	dir := t.TempDir()
	logFile := filepath.Join(dir, "wakatime.log")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client, err := NewClient(Options{
		APIKey:      "waka_test",
		APIURL:      server.URL,
		LogFile:     logFile,
		LogToStdout: true,
		LogWriter:   &logOut,
		Timeout:     1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(context.Background(), "/healthz"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logOut.String(), "GET "+server.URL+"/healthz status=200") {
		t.Fatalf("expected stdout log, got %q", logOut.String())
	}
	if _, err := os.Stat(logFile); !os.IsNotExist(err) {
		t.Fatalf("log-to-stdout should not write log file, stat err=%v", err)
	}
}

func TestRunMetricsFromConfigWritesProfiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WAKATIME_HOME", dir)
	t.Setenv("HOME", dir)
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "metrics", "true")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := Run([]string{"--config", config, "--version"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "metrics"))
	if err != nil {
		t.Fatal(err)
	}
	var cpu, mem bool
	for _, entry := range entries {
		cpu = cpu || strings.HasPrefix(entry.Name(), "cpu_")
		mem = mem || strings.HasPrefix(entry.Name(), "mem_")
	}
	if !cpu || !mem {
		t.Fatalf("expected cpu and mem profiles, got %#v", entries)
	}
}

func TestStartMetricsProfilingContinuesWhenCPUAlreadyProfiling(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WAKATIME_HOME", dir)
	cpuProfile, err := os.Create(filepath.Join(t.TempDir(), "existing-cpu.profile"))
	if err != nil {
		t.Fatal(err)
	}
	defer cpuProfile.Close()
	if err := pprof.StartCPUProfile(cpuProfile); err != nil {
		t.Fatal(err)
	}
	defer pprof.StopCPUProfile()

	stop, err := startMetricsProfiling()
	if err != nil {
		t.Fatal(err)
	}
	stop()

	entries, err := os.ReadDir(filepath.Join(dir, "metrics"))
	if err != nil {
		t.Fatal(err)
	}
	var cpu, mem int
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "cpu_") && strings.HasSuffix(entry.Name(), ".profile") {
			cpu++
		}
		if strings.HasPrefix(entry.Name(), "mem_") && strings.HasSuffix(entry.Name(), ".profile") {
			mem++
		}
	}
	if cpu != 1 || mem != 1 {
		t.Fatalf("profiles cpu=%d mem=%d entries=%#v", cpu, mem, entries)
	}
}

func TestHeartbeatIDMatchesWakaTimeFormat(t *testing.T) {
	cursor := 42
	hb := Heartbeat{
		Branch:         "heartbeat",
		Category:       "coding",
		CursorPosition: &cursor,
		Entity:         "/tmp/main.go",
		EntityType:     "file",
		IsWrite:        true,
		Project:        "wakatime",
		Time:           1592868313.541149,
	}
	if hb.ID() != "1592868313.541149-42-file-coding-wakatime-heartbeat-/tmp/main.go-true" {
		t.Fatalf("unexpected heartbeat id: %q", hb.ID())
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

func TestBuildHeartbeatDetectsSubversionProject(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".svn"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".svn", "wc.db"), []byte("stub"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Project != filepath.Base(dir) {
		t.Fatalf("expected svn project from root, got %#v", hb)
	}
}

func TestBuildHeartbeatDetectsDependencies(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.py")
	if err := os.WriteFile(file, []byte("import os, flask, simplejson as json\nfrom django.conf import settings\nfrom sys import path\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(hb.Dependencies, ",")
	for _, dep := range []string{"django", "flask", "simplejson"} {
		if !strings.Contains(got, dep) {
			t.Fatalf("missing dependency %q in %#v", dep, hb.Dependencies)
		}
	}
	for _, dep := range []string{"django.conf", "os", "sys"} {
		if strings.Contains(got, dep) {
			t.Fatalf("unexpected dependency %q in %#v", dep, hb.Dependencies)
		}
	}
}

func TestBuildHeartbeatDetectsDependenciesFromResolvedLanguageLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	goTextFile := filepath.Join(dir, "main.txt")
	if err := os.WriteFile(goTextFile, []byte("package main\n\nimport \"net/http\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: goTextFile, EntityType: "file", Category: "coding", Language: "Go"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(hb.Dependencies, ",") != "net/http" {
		t.Fatalf("explicit Go dependency parse = %#v, want net/http", hb.Dependencies)
	}

	goFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(goFile, []byte("package main\n\nimport \"net/http\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err = BuildHeartbeat(Options{Entity: goFile, EntityType: "file", Category: "coding", Language: "Python"})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Dependencies != nil {
		t.Fatalf("explicit Python dependency parse on Go source = %#v, want nil", hb.Dependencies)
	}
}

func TestBuildHeartbeatDependencyLanguageAliasesMatchWakaTime(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name     string
		language string
		body     string
		want     string
	}{
		{name: "CSharp", language: "CSharp", body: "using Newtonsoft.Json;\n", want: "Newtonsoft"},
		{name: "CPP", language: "CPP", body: "#include <vector>\n", want: "vector"},
		{name: "ObjectiveC", language: "ObjectiveC", body: "#import <Foundation/Foundation.h>\n", want: "Foundation"},
		{name: "Visual Basic .NET", language: "Visual Basic .NET", body: "Imports Newtonsoft.Json\n", want: "Newtonsoft"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(dir, strings.ReplaceAll(tt.name, " ", "_")+".txt")
			if err := os.WriteFile(file, []byte(tt.body), 0o600); err != nil {
				t.Fatal(err)
			}
			hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", Language: tt.language})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(hb.Dependencies, ",") != tt.want {
				t.Fatalf("%s dependencies = %#v, want %q", tt.language, hb.Dependencies, tt.want)
			}
		})
	}
}

func TestPrimaryHeartbeatDoesNotSerializeUnsavedEntityFlag(t *testing.T) {
	hb, err := BuildHeartbeat(Options{
		Entity:          "unsaved.go",
		EntityType:      "file",
		Category:        "coding",
		IsUnsavedEntity: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hb.IsUnsavedEntity {
		t.Fatal("expected unsaved state to remain available for local processing")
	}
	data, err := json.Marshal(hb)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["is_unsaved_entity"]; ok {
		t.Fatalf("primary heartbeat serialized internal is_unsaved_entity flag: %s", data)
	}
}

func TestBuildHeartbeatSkipsDependenciesForUnsavedEntityLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n\nimport \"net/http\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", IsUnsavedEntity: true})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Dependencies != nil {
		t.Fatalf("unsaved entity dependencies = %#v, want nil", hb.Dependencies)
	}
}

func TestBuildHeartbeatSkipsAutomaticLineCountForUnsavedEntityLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", IsUnsavedEntity: true})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Lines != nil {
		t.Fatalf("unsaved entity auto lines = %#v, want nil", hb.Lines)
	}
	hb, err = BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", IsUnsavedEntity: true, LinesInFile: 3, LinesInFileSet: true})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Lines == nil || *hb.Lines != 3 {
		t.Fatalf("explicit unsaved lines = %#v, want 3", hb.Lines)
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

func TestBuildHeartbeatHidesProjectFolderAndRemoteCredentials(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", HideProjectFolder: true, ProjectFolder: dir})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Entity != "main.go" || hb.ProjectRootCount != nil {
		t.Fatalf("project folder was not hidden: %#v", hb)
	}

	original := remoteFileDownload
	remoteFileDownload = func(_ remoteFileClient, localPath string) error {
		return os.WriteFile(localPath, []byte("package main\n"), 0o600)
	}
	t.Cleanup(func() { remoteFileDownload = original })
	remoteHB, err := BuildHeartbeat(Options{Entity: "ssh://alice:secret@example.org/home/me/main.go", EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if remoteHB.Entity != "ssh://example.org/home/me/main.go" {
		t.Fatalf("remote credentials were not hidden: %q", remoteHB.Entity)
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

func TestDetectPackageJSONDependencies(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "package.json")
	if err := os.WriteFile(file, []byte(`{"dependencies":{"react":"latest"},"devDependencies":{"typescript":"latest"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(hb.Dependencies, ",") != "npm,react,typescript" {
		t.Fatalf("dependencies = %#v", hb.Dependencies)
	}
}

func TestDetectPackageJSONDependenciesPreservesWakaTimeOrder(t *testing.T) {
	file := filepath.Join(t.TempDir(), "package.json")
	body := `{"dependencies":{"wakatime":"latest","another_dep":"latest"},"devDependencies":{"test_framework":"latest","another_dev_dep":"latest"}}`
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got := detectDependencies(file)
	want := []string{"npm", "wakatime", "another_dep", "test_framework", "another_dev_dep"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("dependencies = %#v, want %#v", got, want)
	}
}

func TestDetectDependenciesCapsWakaTimeDependencyCount(t *testing.T) {
	var body strings.Builder
	body.WriteString("package main\n\nimport (\n")
	for i := 0; i < maxDependenciesCount+5; i++ {
		body.WriteString(fmt.Sprintf("\t\"example.com/dep%04d\"\n", i))
	}
	body.WriteString(")\n")
	file := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(file, []byte(body.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	got := detectDependencies(file)
	if len(got) != maxDependenciesCount {
		t.Fatalf("dependency count = %d, want %d", len(got), maxDependenciesCount)
	}
	if got[0] != "example.com/dep0000" || got[len(got)-1] != "example.com/dep0999" {
		t.Fatalf("dependency cap did not preserve first-seen dependencies: first=%q last=%q", got[0], got[len(got)-1])
	}
}

func TestDetectDependenciesOnlyReadsLargeFileHeadLikeWakaTime(t *testing.T) {
	var body strings.Builder
	body.Write(bytes.Repeat([]byte("// filler\n"), (maxFileStatsBytes/len("// filler\n"))+1))
	body.WriteString("import \"example.com/late\"\n")
	file := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(file, []byte(body.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := detectDependencies(file); len(got) != 0 {
		t.Fatalf("dependency after first 5 MiB should not be detected, got %#v", got)
	}
}

func TestDetectPackageJSONDependenciesOnlyReadsLargeFileHeadLikeWakaTime(t *testing.T) {
	var body strings.Builder
	body.WriteString("{")
	body.WriteString(`"filler":"`)
	body.Write(bytes.Repeat([]byte("x"), maxFileStatsBytes+1))
	body.WriteString(`","dependencies":{"late":"latest"}}`)
	file := filepath.Join(t.TempDir(), "package.json")
	if err := os.WriteFile(file, []byte(body.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := detectDependencies(file); strings.Join(got, ",") != "npm" {
		t.Fatalf("dependency after first 5 MiB should not be detected, got %#v", got)
	}
}

func TestDetectBowerJSONDependencies(t *testing.T) {
	tests := []struct {
		name string
		file string
		body string
		want string
	}{
		{
			name: "bower",
			file: "bower.json",
			body: `{"dependencies":{"bootstrap":"latest"},"devDependencies":{"moment":"latest"}}`,
			want: "bower,bootstrap,moment",
		},
		{
			name: "component",
			file: "component.json",
			body: `{"dependencies":{"component/jquery":"latest"}}`,
			want: "bower,component/jquery",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), tt.file)
			if err := os.WriteFile(file, []byte(tt.body), 0o600); err != nil {
				t.Fatal(err)
			}
			hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding"})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(hb.Dependencies, ",") != tt.want {
				t.Fatalf("dependencies = %#v, want %s", hb.Dependencies, tt.want)
			}
		})
	}
}

func TestDetectAdditionalLanguageDependencies(t *testing.T) {
	tests := []struct {
		name string
		file string
		body string
		want []string
	}{
		{
			name: "csharp",
			file: "Program.cs",
			body: "using System;\nusing Microsoft.Extensions.Logging;\nusing WakaTime.Forms;\nusing static Math.Foo;\nusing Task = Fart.Threading.Tasks.Task;\nusing static Proper.Bar;\n",
			want: []string{"WakaTime", "Math", "Fart", "Proper"},
		},
		{
			name: "c",
			file: "main.c",
			body: "#include <stdio.h>\n#include <math.h>\n#include <openssl/ssl.h>\n",
			want: []string{"math", "openssl"},
		},
		{
			name: "cpp",
			file: "main.cpp",
			body: "#include <iostream>\n#include <openssl/ssl.h>\n#include \"wakatime/client.h\"\n",
			want: []string{"openssl", "wakatime"},
		},
		{
			name: "typescript",
			file: "App.tsx",
			body: "import React from 'react'\nimport Footer from '../components/Footer.tsx'\nimport constants from \"./lib/constants.js\"\nconst fp = require('lodash/fp')\nimport pkg from '@scope/package'\n",
			want: []string{"react", "Footer", "constants", "fp", "package"},
		},
		{
			name: "typescript multiline import",
			file: "App.ts",
			body: "import {\n  alpha,\n  bravo,\n} from './november';\nimport charlie from 'delta';\n",
			want: []string{"november", "delta"},
		},
		{
			name: "javascript waka fixture imports",
			file: "es6.js",
			body: "import Alpha from './bravo';\nimport { charlie, delta } from '../../echo/foxtrot';\nimport golf from './hotel/india.js';\nimport juliett from 'kilo';\nimport {\n  lima,\n  mike,\n} from './november';\nimport * from '/modules/oscar';\nimport * as papa from 'quebec';\nimport {romeo as sierra} from from 'tango.jsx';\nimport 'uniform.js';\nimport victorDefault, * as victorModule from '/modules/victor.js';\nimport whiskeyDefault, {whiskeyOne, whiskeyTwo} from 'whiskey';\n",
			want: []string{"bravo", "foxtrot", "india", "kilo", "november", "oscar", "quebec", "tango", "uniform", "victor", "whiskey"},
		},
		{
			name: "gruntfile",
			file: "Gruntfile",
			body: "module.exports = function(grunt) {\n  require('grunt');\n}\n",
			want: []string{"grunt"},
		},
		{
			name: "python",
			file: "main.py",
			body: "import os, flask, simplejson as json\nfrom django.conf import settings\nfrom sys import path\n",
			want: []string{"flask", "simplejson", "django"},
		},
		{
			name: "go",
			file: "main.go",
			body: "package main\n\nimport (\n\t\"fmt\"\n\t\"log\"\n\t\"os\"\n\t\"github.com/acme/pkg\"\n)\n",
			want: []string{"log", "os", "github.com/acme/pkg"},
		},
		{
			name: "java",
			file: "Hello.java",
			body: "import java.io.*;\nimport static com.googlecode.javacv.jna.highgui.cvReleaseCapture;\nimport javax.servlet.*;\nimport com.colorfulwolf.webcamapplet.gui.ImagePanel;\nimport com.foobar.*;\nimport package com.apackage.something;\nimport namespace com.anamespace.other;\n",
			want: []string{"googlecode.javacv", "colorfulwolf.webcamapplet", "foobar", "apackage.something", "anamespace.other"},
		},
		{
			name: "kotlin",
			file: "Main.kt",
			body: "import java.util.List\nimport com.squareup.moshi.Moshi\nimport org.example.tools.*\n",
			want: []string{"squareup.moshi", "example.tools"},
		},
		{
			name: "scala",
			file: "Main.scala",
			body: "import __root__.com.alpha.SomeClass\nimport _root_.com.bravo.something\nimport com.charlie._\nimport golf\nimport juliett.kilo.Lima\n",
			want: []string{"com.alpha.SomeClass", "com.bravo.something", "com.charlie", "golf", "juliett.kilo.Lima"},
		},
		{
			name: "scala grouped imports",
			file: "Grouped.scala",
			body: "import com.alpha.SomeClass\nimport com.bravo.something.{User, UserPreferences}\nimport com.charlie.{Delta => Foxtrot}\nimport __root__.golf._\nimport com.hotel.india._\nimport juliett.kilo\n",
			want: []string{"com.alpha.SomeClass", "com.bravo.something", "com.charlie", "golf", "com.hotel.india", "juliett.kilo"},
		},
		{
			name: "haskell",
			file: "Main.hs",
			body: "import qualified Data.Map as Map\nimport Control.Monad\n",
			want: []string{"Data", "Control"},
		},
		{
			name: "elm",
			file: "Main.elm",
			body: "import Html exposing (text)\nimport Json.Decode as Decode\n",
			want: []string{"Html", "Json"},
		},
		{
			name: "rust",
			file: "lib.rs",
			body: "extern crate proc_macro;\nextern crate phrases;\nuse serde::Serialize;\n",
			want: []string{"proc_macro", "phrases"},
		},
		{
			name: "haxe",
			file: "Main.hx",
			body: "import alpha.ds.StringMap;\nimport bravo.macro.*;\nimport Math.random;\nimport charlie.fromCharCode in f;\nimport delta.something;\nimport haxe.ds.StringMap;\n",
			want: []string{"alpha", "bravo", "Math", "charlie", "delta"},
		},
		{
			name: "html",
			file: "index.html",
			body: `<script src="/static/app.js"></script>` + "\n" + `<script type="text/javascript" src="{{ STATIC_URL }}/libs/json2.js"></script>` + "\n" + `<script src="this is a` + "\n" + ` multiline value"></script>` + "\n",
			want: []string{`"/static/app.js"`, `"libs/json2.js"`, "\"this is a\n multiline value\""},
		},
		{
			name: "objective-c",
			file: "ViewController.m",
			body: "#import \"SomeViewController.h\"\n#import 'OtherViewController.h'\n#import <UIKit/UIKit.h>\n#import <PromiseKit/PromiseKit.h>\n",
			want: []string{"SomeViewController", "OtherViewController", "UIKit", "PromiseKit"},
		},
		{
			name: "php",
			file: "service.php",
			body: "<?php\nuse Interop\\Container\\ContainerInterface;\nrequire 'ServiceLocator.php';\nrequire \"ServiceLocatorTwo.php\";\nuse FooBarOne\\Classname as Another;\nuse function FooBarThree\\Full\\functionNameThree;\nuse FooBarSeven\\Full\\ClassnameSeven as AnotherSeven, FooBarEight\\Full\\NSnameEight;\n",
			want: []string{"Interop", "'ServiceLocator.php'", "'ServiceLocatorTwo.php'", "FooBarOne", "FooBarThree", "FooBarSeven", "FooBarEight"},
		},
		{
			name: "swift",
			file: "ViewController.swift",
			body: "import Foundation\nimport UIKit\nimport PromiseKit\n",
			want: []string{"UIKit", "PromiseKit"},
		},
		{
			name: "vbnet",
			file: "Main.vb",
			body: "Imports System\nImports Microsoft.VisualBasic\nImports WakaTime.Core\nImports mat = Math.Foo\nImports pr = Proper.Bar\n",
			want: []string{"WakaTime", "Math", "Proper"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tt.file)
			if err := os.WriteFile(path, []byte(tt.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got := detectDependencies(path)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("dependencies = %#v, want %#v", got, tt.want)
			}
		})
	}
}

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

func TestRunHeartbeatRoutesEachExtraHeartbeatToMatchingTargets(t *testing.T) {
	dir := t.TempDir()
	mainFile := filepath.Join(dir, "main.go")
	extraFile := filepath.Join(dir, "extra.go")
	for _, file := range []string{mainFile, extraFile} {
		if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	posted := map[string][]Heartbeat{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var heartbeats []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&heartbeats); err != nil {
			t.Fatal(err)
		}
		posted[r.Header.Get("Authorization")] = append(posted[r.Header.Get("Authorization")], heartbeats...)
		_, _ = w.Write(bulkResponseFor(heartbeats, http.StatusCreated))
	}))
	defer server.Close()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "api_url", server.URL+"/api/v1")
	cfg.Set("settings", "api_key", "waka_default")
	cfg.Set("api_urls", `extra\.go$`, server.URL+"/api/v1|waka_extra")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	stdin := strings.NewReader(`[{"entity":` + strconv.Quote(extraFile) + `,"type":"file","time":2}]`)
	if err := Run([]string{
		"--config", config,
		"--entity", mainFile,
		"--extra-heartbeats",
		"--offline-queue-file", filepath.Join(dir, "queue.bdb"),
		"--heartbeat-rate-limit-seconds", "0",
	}, stdin, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	defaultPosts := posted[basicAuthHeader("waka_default")]
	extraPosts := posted[basicAuthHeader("waka_extra")]
	if len(defaultPosts) != 2 || len(extraPosts) != 1 || extraPosts[0].Entity != extraFile {
		t.Fatalf("unexpected routed posts: %#v", posted)
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

func TestHeartbeatActiveBackoffQueuesWithoutNetwork(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	internalConfig := filepath.Join(dir, "wakatime-internal.cfg")
	if err := WriteConfigValue(internalConfig, "internal", "backoff_retries", "1"); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfigValue(internalConfig, "internal", "backoff_at", time.Now().Format(wakaTimeDateFormat)); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"responses":[]}`))
	}))
	defer server.Close()
	var out bytes.Buffer
	if err := Run([]string{
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--entity", file,
		"--offline-queue-file", filepath.Join(dir, "offline.bdb"),
		"--internal-config", internalConfig,
		"--heartbeat-rate-limit-seconds", "0",
	}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("expected no network call during backoff, got %d", calls)
	}
	if strings.TrimSpace(out.String()) != "queued=1" {
		t.Fatalf("expected queued output, got %q", out.String())
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

func TestAPIURLsBlankValueUsesEffectiveDefaultTarget(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("project_api_key", `main\.go$`, "waka_project")
	cfg.Set("api_urls", `main\.go$`, "")

	targets, err := heartbeatTargets(Options{
		APIURL: "http://default.example/api/v1",
		APIKey: "waka_default",
		Config: cfg,
	}, file)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].APIURL != "http://default.example/api/v1" || targets[0].APIKey != "waka_project" {
		t.Fatalf("blank api_urls should dedupe to effective default target, got %#v", targets)
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

func TestGitProjectFromGitRemote(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(projectDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".git", "config"), []byte("[remote \"origin\"]\n\turl = git@github.com:keithah/stint.git\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(projectDir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("git", "project_from_git_remote", "true")
	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Project != "keithah/stint" {
		t.Fatalf("project = %q", hb.Project)
	}
}

func TestWakaTimeProjectPlaceholderUsesGitRemoteProjectLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(projectDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".wakatime-project"), []byte("custom/{project}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, ".git", "config"), []byte("[remote \"origin\"]\n\turl = git@github.com:keithah/stint.git\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(projectDir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("git", "project_from_git_remote", "true")

	hb, err := BuildHeartbeat(Options{Entity: file, EntityType: "file", Category: "coding", Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if hb.Project != "custom/keithah/stint" {
		t.Fatalf("project = %q", hb.Project)
	}
}

func TestProjectFromRemoteURLHandlesHTTPS(t *testing.T) {
	if got := projectFromRemoteURL("https://github.com/keithah/stint.git"); got != "keithah/stint" {
		t.Fatalf("project = %q", got)
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

func TestAPIKeyVaultCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-only")
	}
	dir := t.TempDir()
	vault := filepath.Join(dir, "vault.sh")
	if err := os.WriteFile(vault, []byte("#!/bin/sh\necho waka_vault\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "api_key_vault_cmd", vault)
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{"--config", config})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_vault" {
		t.Fatalf("APIKey = %q", opts.APIKey)
	}
}

func TestAPIKeyVaultCommandRunsThroughShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command fixture is unix-only")
	}
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "api_key_vault_cmd", `printf 'waka_%s' shell`)
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{"--config", config})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_shell" {
		t.Fatalf("APIKey = %q", opts.APIKey)
	}
}

func TestAPIKeyConfigWinsOverVaultCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command fixture is unix-only")
	}
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "api_key", "waka_config")
	cfg.Set("settings", "api_key_vault_cmd", "exit 7")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{"--config", config})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_config" {
		t.Fatalf("APIKey = %q", opts.APIKey)
	}
}

func TestAPIKeyVaultCommandFailureReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command fixture is unix-only")
	}
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "api_key_vault_cmd", "exit 7")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	_, err := parseCommon([]string{"--config", config})
	if err == nil || !strings.Contains(err.Error(), "failed to read api key from vault") {
		t.Fatalf("expected vault command error, got %v", err)
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

func TestWakaTimeHomeResourcePathsMatchUpstream(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WAKATIME_HOME", dir)
	if DefaultWakaTimeConfigPath() != filepath.Join(dir, ".wakatime.cfg") {
		t.Fatalf("config path = %q", DefaultWakaTimeConfigPath())
	}
	if DefaultQueuePath() != filepath.Join(dir, "offline_heartbeats.bdb") {
		t.Fatalf("queue path = %q", DefaultQueuePath())
	}
	if DefaultLegacyQueuePath() != filepath.Join(dir, ".wakatime.bdb") {
		t.Fatalf("legacy queue path = %q", DefaultLegacyQueuePath())
	}
	if DefaultLogFilePath() != filepath.Join(dir, "wakatime.log") {
		t.Fatalf("log path = %q", DefaultLogFilePath())
	}
	if DefaultInternalConfigPath() != filepath.Join(dir, "wakatime-internal.cfg") {
		t.Fatalf("internal config path = %q", DefaultInternalConfigPath())
	}
}

func TestParseCommonUsesWakaTimeCompatibleDefaults(t *testing.T) {
	opts, err := parseCommon([]string{"--config", filepath.Join(t.TempDir(), "missing.cfg")})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Timeout != defaultTimeoutSeconds {
		t.Fatalf("timeout = %d", opts.Timeout)
	}
	if opts.PrintOffline != defaultPrintOfflineMax {
		t.Fatalf("print offline default = %d", opts.PrintOffline)
	}
	if opts.SyncOffline != defaultQueueMaxSync {
		t.Fatalf("sync offline default = %d", opts.SyncOffline)
	}
}

func TestParseCommonPreservesExplicitZeroTimeoutLikeWakaTime(t *testing.T) {
	opts, err := parseCommon([]string{
		"--timeout", "0",
		"--key", "waka_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Timeout != 0 {
		t.Fatalf("timeout = %d", opts.Timeout)
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

func TestAPIURLFlagTakesPrecedenceOverDeprecatedAPIURLAlias(t *testing.T) {
	opts, err := parseCommon([]string{
		"--api-url", "http://modern.example/api/v1",
		"--apiurl", "http://deprecated.example/api/v1",
		"--key", "waka_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIURL != "http://modern.example/api/v1" {
		t.Fatalf("APIURL = %q", opts.APIURL)
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

func TestParseCommonDisablesNegativeHeartbeatRateLimitLikeWakaTime(t *testing.T) {
	opts, err := parseCommon([]string{
		"--heartbeat-rate-limit-seconds", "-1",
		"--key", "waka_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.HeartbeatRateLimit != 0 {
		t.Fatalf("negative heartbeat rate limit should disable rate limiting, got %d", opts.HeartbeatRateLimit)
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

func TestLogFileFlagTakesPrecedenceOverDeprecatedLogfileAlias(t *testing.T) {
	modern := filepath.Join(t.TempDir(), "modern.log")
	deprecated := filepath.Join(t.TempDir(), "deprecated.log")
	opts, err := parseCommon([]string{
		"--log-file", modern,
		"--logfile", deprecated,
		"--key", "waka_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.LogFile != modern {
		t.Fatalf("LogFile = %q", opts.LogFile)
	}
}

func TestEntityFlagTakesPrecedenceOverDeprecatedFileAlias(t *testing.T) {
	opts, err := parseCommon([]string{
		"--entity", "/tmp/modern.go",
		"--file", "/tmp/deprecated.go",
		"--key", "waka_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Entity != "/tmp/modern.go" {
		t.Fatalf("Entity = %q", opts.Entity)
	}
}

func TestHideFileNamesFlagTakesPrecedenceOverDeprecatedAliases(t *testing.T) {
	opts, err := parseCommon([]string{
		"--hide-file-names", "modern",
		"--hide-filenames", "deprecated-one",
		"--hidefilenames", "deprecated-two",
		"--key", "waka_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.HideFileNames != "modern" {
		t.Fatalf("HideFileNames = %q", opts.HideFileNames)
	}
}

func TestDisableOfflineFlagTakesPrecedenceOverDeprecatedAlias(t *testing.T) {
	opts, err := parseCommon([]string{
		"--disable-offline=false",
		"--disableoffline=true",
		"--key", "waka_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.DisableOffline {
		t.Fatal("deprecated disableoffline alias overrode explicit disable-offline=false")
	}
}

func TestNoSSLVerifyFlagFalseTakesPrecedenceOverConfig(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "no_ssl_verify", "true")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{
		"--config", config,
		"--no-ssl-verify=false",
		"--key", "waka_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.NoSSLVerify {
		t.Fatal("config no_ssl_verify=true overrode explicit --no-ssl-verify=false")
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

func TestSendDiagnosticsFlagFalseTakesPrecedenceOverConfig(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "send_diagnostics_on_errors", "true")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{
		"--config", config,
		"--send-diagnostics-on-errors=false",
		"--key", "waka_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.SendDiagnosticsOnError {
		t.Fatal("config send_diagnostics_on_errors=true overrode explicit --send-diagnostics-on-errors=false")
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

func TestParseCommonRejectsInvalidWakaTimeCategoryAndEntityType(t *testing.T) {
	if _, err := parseCommon([]string{"--category", "invalid"}); err == nil || !strings.Contains(err.Error(), `invalid category "invalid"`) {
		t.Fatalf("expected invalid category error, got %v", err)
	}
	if _, err := parseCommon([]string{"--entity-type", "invalid"}); err == nil || !strings.Contains(err.Error(), `invalid entity type "invalid"`) {
		t.Fatalf("expected invalid entity type error, got %v", err)
	}
	if opts, err := parseCommon([]string{"--entity-type", "event", "--category", "writing docs"}); err != nil || opts.EntityType != "event" || opts.Category != "writing docs" {
		t.Fatalf("expected valid event/category, opts=%#v err=%v", opts, err)
	}
}

func TestRunDefaultExplicitCodingAndNullCategoriesAreOmittedLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var batches [][]Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		batches = append(batches, posted)
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()

	baseArgs := []string{
		"--entity", file,
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", filepath.Join(dir, "queue.bdb"),
		"--heartbeat-rate-limit-seconds", "0",
		"--sync-ai-disabled",
	}
	if err := Run(baseArgs, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := Run(append(append([]string{}, baseArgs...), "--category", "coding"), nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := Run(append(append([]string{}, baseArgs...), "--category", "null"), nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(batches) != 3 || len(batches[0]) != 1 || len(batches[1]) != 1 || len(batches[2]) != 1 {
		t.Fatalf("unexpected batches: %#v", batches)
	}
	if batches[0][0].Category != "" {
		t.Fatalf("default category should be omitted, got %q", batches[0][0].Category)
	}
	if batches[1][0].Category != "" {
		t.Fatalf("explicit coding category should be omitted, got %q", batches[1][0].Category)
	}
	if batches[2][0].Category != "" {
		t.Fatalf("explicit null category should be omitted, got %q", batches[2][0].Category)
	}
}

func TestRunPreservesExplicitZeroIntegerHeartbeatFlagsLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
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
		"--entity", file,
		"--cursorpos", "0",
		"--lineno", "0",
		"--lines-in-file", "0",
		"--human-line-changes", "0",
		"--ai-line-changes", "0",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", filepath.Join(dir, "queue.bdb"),
		"--heartbeat-rate-limit-seconds", "0",
		"--sync-ai-disabled",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 1 {
		t.Fatalf("posted = %#v", posted)
	}
	hb := posted[0]
	if hb.CursorPosition == nil || *hb.CursorPosition != 0 {
		t.Fatalf("cursorpos = %#v", hb.CursorPosition)
	}
	if hb.LineNumber == nil || *hb.LineNumber != 0 {
		t.Fatalf("lineno = %#v", hb.LineNumber)
	}
	if hb.Lines == nil || *hb.Lines != 0 {
		t.Fatalf("lines = %#v", hb.Lines)
	}
	if hb.HumanLineChanges == nil || *hb.HumanLineChanges != 0 {
		t.Fatalf("human_line_changes = %#v", hb.HumanLineChanges)
	}
	if hb.AILineChanges == nil || *hb.AILineChanges != 0 {
		t.Fatalf("ai_line_changes = %#v", hb.AILineChanges)
	}
}

func TestRunPreservesExplicitZeroAITokenFlags(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
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
		"--entity", file,
		"--ai-input-tokens", "0",
		"--ai-output-tokens", "0",
		"--ai-prompt-length", "0",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", filepath.Join(dir, "queue.bdb"),
		"--heartbeat-rate-limit-seconds", "0",
		"--sync-ai-disabled",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 1 {
		t.Fatalf("posted = %#v", posted)
	}
	hb := posted[0]
	if hb.AIInputTokens == nil || *hb.AIInputTokens != 0 {
		t.Fatalf("ai_input_tokens = %#v", hb.AIInputTokens)
	}
	if hb.AIOutputTokens == nil || *hb.AIOutputTokens != 0 {
		t.Fatalf("ai_output_tokens = %#v", hb.AIOutputTokens)
	}
	if hb.AIPromptLength == nil || *hb.AIPromptLength != 0 {
		t.Fatalf("ai_prompt_length = %#v", hb.AIPromptLength)
	}
}

func TestRunPreservesAITokenFlags(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
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
		"--entity", file,
		"--ai-input-tokens", "123",
		"--ai-output-tokens", "456",
		"--ai-prompt-length", "789",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", filepath.Join(dir, "queue.bdb"),
		"--heartbeat-rate-limit-seconds", "0",
		"--sync-ai-disabled",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 1 {
		t.Fatalf("posted = %#v", posted)
	}
	hb := posted[0]
	if hb.AIInputTokens == nil || *hb.AIInputTokens != 123 {
		t.Fatalf("ai_input_tokens = %#v", hb.AIInputTokens)
	}
	if hb.AIOutputTokens == nil || *hb.AIOutputTokens != 456 {
		t.Fatalf("ai_output_tokens = %#v", hb.AIOutputTokens)
	}
	if hb.AIPromptLength == nil || *hb.AIPromptLength != 789 {
		t.Fatalf("ai_prompt_length = %#v", hb.AIPromptLength)
	}
}

func TestRunPreservesExplicitFalseWriteFlagLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var posted []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"responses":[]}`))
	}))
	defer server.Close()

	if err := Run([]string{
		"--entity", file,
		"--write=false",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", filepath.Join(dir, "queue.bdb"),
		"--heartbeat-rate-limit-seconds", "0",
		"--sync-ai-disabled",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 1 {
		t.Fatalf("posted = %#v", posted)
	}
	value, ok := posted[0]["is_write"]
	if !ok || value != false {
		t.Fatalf("is_write = %#v present=%v payload=%#v", value, ok, posted[0])
	}
}

func TestBuildHeartbeatDetectsWakaTimeCategories(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		filepath.Join("pkg", "thing_test.go"):          "writing tests",
		filepath.Join("pkg", "thing.spec.js"):          "writing tests",
		filepath.Join("pkg", "testdata", "fixture.go"): "writing tests",
		"README.md": "writing docs",
		"guide.mdx": "writing docs",
		"main.go":   "",
	}
	for rel, want := range cases {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("content\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		hb, err := BuildHeartbeat(Options{Entity: path, EntityType: "file"})
		if err != nil {
			t.Fatal(err)
		}
		if hb.Category != want {
			t.Fatalf("%s category = %q, want %q", rel, hb.Category, want)
		}
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

func TestRunMissingMainHeartbeatSucceedsWithoutPosting(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		t.Fatalf("unexpected request for missing heartbeat: %s", r.URL.Path)
	}))
	defer server.Close()

	if err := Run([]string{
		"--entity", filepath.Join(dir, "missing.go"),
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

func TestNewClientUsesProxySSLAndTimeoutOptions(t *testing.T) {
	proxyURL, err := url.Parse("http://127.0.0.1:9999")
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(Options{APIURL: "https://example.test/api/v1", APIKey: "waka_test", Proxy: proxyURL.String(), NoSSLVerify: true, Timeout: 7})
	if err != nil {
		t.Fatal(err)
	}
	if client.client.Timeout.String() != "7s" {
		t.Fatalf("timeout = %s", client.client.Timeout)
	}
	transport := client.client.Transport.(*http.Transport)
	req := httptest.NewRequest(http.MethodGet, "https://example.test", nil)
	gotProxy, err := transport.Proxy(req)
	if err != nil {
		t.Fatal(err)
	}
	if gotProxy.String() != proxyURL.String() {
		t.Fatalf("proxy = %s", gotProxy)
	}
	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("expected TLS InsecureSkipVerify, got %#v", transport.TLSClientConfig)
	}
}

func TestNewClientHonorsExplicitZeroTimeoutLikeWakaTime(t *testing.T) {
	client, err := NewClient(Options{APIURL: "https://example.test/api/v1", APIKey: "waka_test", Timeout: 0})
	if err != nil {
		t.Fatal(err)
	}
	if client.client.Timeout != 0 {
		t.Fatalf("timeout = %s", client.client.Timeout)
	}
}

func TestNewClientKeepsProxyFromEnvironmentFallback(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9998")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("NO_PROXY", "")
	client, err := NewClient(Options{APIURL: "https://example.test/api/v1", APIKey: "waka_test", Timeout: 7})
	if err != nil {
		t.Fatal(err)
	}
	transport := client.client.Transport.(*http.Transport)
	req := httptest.NewRequest(http.MethodGet, "https://example.test", nil)
	gotProxy, err := testTransportProxy(transport, req)
	if err != nil {
		t.Fatal(err)
	}
	if gotProxy == nil || gotProxy.String() != "http://127.0.0.1:9998" {
		t.Fatalf("proxy = %v", gotProxy)
	}
}

func TestNewClientHonorsNoProxyEnvironmentLikeWakaTime(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9998")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("NO_PROXY", "example.test")
	client, err := NewClient(Options{APIURL: "https://example.test/api/v1", APIKey: "waka_test", Timeout: 7})
	if err != nil {
		t.Fatal(err)
	}
	transport := client.client.Transport.(*http.Transport)
	req := httptest.NewRequest(http.MethodGet, "https://example.test", nil)
	gotProxy, err := testTransportProxy(transport, req)
	if err != nil {
		t.Fatal(err)
	}
	if gotProxy != nil {
		t.Fatalf("proxy = %v, want nil", gotProxy)
	}
}

func TestNewClientHonorsURLShapedNoProxyEnvironmentLikeWakaTime(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9998")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("NO_PROXY", "https://some.org,https://api.wakatime.com")
	client, err := NewClient(Options{APIURL: wakaTimeAPIURL, APIKey: "waka_test", Timeout: 7})
	if err != nil {
		t.Fatal(err)
	}
	transport := client.client.Transport.(*http.Transport)
	req := httptest.NewRequest(http.MethodGet, wakaTimeAPIURL+"/users/current", nil)
	gotProxy, err := testTransportProxy(transport, req)
	if err != nil {
		t.Fatal(err)
	}
	if gotProxy != nil {
		t.Fatalf("proxy = %v, want nil", gotProxy)
	}
}

func testTransportProxy(transport *http.Transport, req *http.Request) (*url.URL, error) {
	if transport.Proxy == nil {
		return nil, nil
	}
	return transport.Proxy(req)
}

func TestNewClientNormalizesWakaTimeProxyForms(t *testing.T) {
	client, err := NewClient(Options{APIURL: "https://example.test/api/v1", APIKey: "waka_test", Proxy: "john:secret@example.org:8888", Timeout: 7})
	if err != nil {
		t.Fatal(err)
	}
	transport := client.client.Transport.(*http.Transport)
	req := httptest.NewRequest(http.MethodGet, "https://example.test", nil)
	gotProxy, err := transport.Proxy(req)
	if err != nil {
		t.Fatal(err)
	}
	if gotProxy.String() != "http://john:secret@example.org:8888" {
		t.Fatalf("proxy = %s", gotProxy)
	}
	if _, err := NewClient(Options{APIURL: "https://example.test/api/v1", APIKey: "waka_test", Proxy: "ftp://example.org:21"}); err == nil {
		t.Fatalf("expected unsupported proxy scheme error")
	}
}

func TestClientRetriesHTTPSProxyAsHTTPForPlainProxy(t *testing.T) {
	var calls int
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.String() != "http://example.test/api/v1/meta" {
			t.Fatalf("proxy request URL = %q", r.URL.String())
		}
		_, _ = w.Write([]byte(`{"data":true}`))
	}))
	defer proxy.Close()

	client, err := NewClient(Options{
		APIURL:  "http://example.test/api/v1",
		APIKey:  "waka_test",
		Proxy:   strings.Replace(proxy.URL, "http://", "https://", 1),
		Timeout: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := client.Get(context.Background(), "/meta")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != `{"data":true}` || calls != 1 {
		t.Fatalf("body=%q calls=%d", body, calls)
	}
}

func TestNewClientAcceptsNTLMProxyCredentialsWithoutClobberingAPIAuth(t *testing.T) {
	var auth, proxyAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		proxyAuth = r.Header.Get("Proxy-Authorization")
		_, _ = w.Write([]byte(`{"data":true}`))
	}))
	defer server.Close()
	client, err := NewClient(Options{APIURL: server.URL + "/api/v1", APIKey: "waka_test", Proxy: `domain\\john:secret`, Timeout: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(context.Background(), "/meta"); err != nil {
		t.Fatal(err)
	}
	if auth != basicAuthHeader("waka_test") {
		t.Fatalf("authorization = %q", auth)
	}
	expectedProxyAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(`domain\\john:secret`))
	if proxyAuth != expectedProxyAuth {
		t.Fatalf("proxy auth = %q, want %q", proxyAuth, expectedProxyAuth)
	}
	transport := client.client.Transport.(*http.Transport)
	if got := transport.ProxyConnectHeader.Get("Proxy-Authorization"); got != expectedProxyAuth {
		t.Fatalf("proxy connect auth = %q", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestClientRetriesWakaTimeDNSFallback(t *testing.T) {
	var hosts []string
	client, err := NewClient(Options{APIURL: wakaTimeAPIURL, APIKey: "waka_test", Timeout: 1})
	if err != nil {
		t.Fatal(err)
	}
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		hosts = append(hosts, r.URL.Host)
		if len(hosts) == 1 {
			return nil, &net.DNSError{Err: "no such host", Name: "api.wakatime.com"}
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"data":true}`)),
			Request:    r,
		}, nil
	})
	if _, err := client.PostJSON(context.Background(), "/users/current/heartbeats.bulk", map[string]string{"x": "y"}); err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 {
		t.Fatalf("calls = %d, hosts = %v", len(hosts), hosts)
	}
	if hosts[0] != "api.wakatime.com" {
		t.Fatalf("first host = %q", hosts[0])
	}
	if hosts[1] != wakaTimeAPIIPv4 && hosts[1] != wakaTimeAPIIPv6 {
		t.Fatalf("fallback host = %q", hosts[1])
	}
}

func TestClientDoesNotRetryCustomAPIDNSFailure(t *testing.T) {
	var calls int
	client, err := NewClient(Options{APIURL: "https://custom.example.com/api/v1", APIKey: "waka_test", Timeout: 1})
	if err != nil {
		t.Fatal(err)
	}
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, &net.DNSError{Err: "no such host", Name: r.URL.Host}
	})
	_, err = client.Get(context.Background(), "/meta")
	if err == nil {
		t.Fatal("expected dns error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestClientSendsWakaTimeMachineAndTimezoneHeaders(t *testing.T) {
	t.Setenv("TZ", "America/Los_Angeles")
	var contentType, machine, timezone string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		machine = r.Header.Get("X-Machine-Name")
		timezone = r.Header.Get("Timezone")
		_, _ = w.Write([]byte(`{"data":true}`))
	}))
	defer server.Close()
	client, err := NewClient(Options{APIURL: server.URL + "/api/v1", APIKey: "waka_test", Hostname: "dev box", Timeout: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(context.Background(), "/meta"); err != nil {
		t.Fatal(err)
	}
	if machine != "dev+box" {
		t.Fatalf("X-Machine-Name = %q", machine)
	}
	if contentType != "application/json" {
		t.Fatalf("Content-Type = %q", contentType)
	}
	if timezone != "America/Los_Angeles" {
		t.Fatalf("Timezone = %q", timezone)
	}
}

func TestLocalTimezoneNameFallsBackToUTC(t *testing.T) {
	t.Setenv("TZ", "Not/AZone")
	if got := localTimezoneName(); got == "" {
		t.Fatalf("expected fallback timezone")
	}
}

func TestMachineNameUsesGitpodFallback(t *testing.T) {
	t.Setenv("GITPOD_WORKSPACE_ID", "workspace-id")
	if got := machineName(""); got != "Gitpod" {
		t.Fatalf("machineName = %q", got)
	}
	if got := machineName("explicit-host"); got != "explicit-host" {
		t.Fatalf("machineName override = %q", got)
	}
}

func TestClientWritesLogFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":true}`))
	}))
	defer server.Close()
	logFile := filepath.Join(t.TempDir(), "wakatime.log")
	client, err := NewClient(Options{APIURL: server.URL + "/api/v1", APIKey: "waka_test", LogFile: logFile, Timeout: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(t.Context(), "/meta"); err != nil {
		t.Fatal(err)
	}
	logs, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logs), "GET "+server.URL+"/api/v1/meta status=200") {
		t.Fatalf("unexpected logs: %s", logs)
	}
}

func TestClientCreatesLogDirectoryWithoutWorldPermissions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":true}`))
	}))
	defer server.Close()
	logDir := filepath.Join(t.TempDir(), "nested")
	client, err := NewClient(Options{APIURL: server.URL + "/api/v1", APIKey: "waka_test", LogFile: filepath.Join(logDir, "wakatime.log"), Timeout: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(t.Context(), "/meta"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(logDir)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode&0o007 != 0 {
		t.Fatalf("log directory mode = %o, expected no world permissions", mode)
	}
}

func TestClientRotatesOversizedLogFile(t *testing.T) {
	originalMaxSize := maxLogFileSizeBytes
	originalBackups := maxLogFileBackups
	maxLogFileSizeBytes = 64
	maxLogFileBackups = 2
	t.Cleanup(func() {
		maxLogFileSizeBytes = originalMaxSize
		maxLogFileBackups = originalBackups
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":true}`))
	}))
	defer server.Close()
	logFile := filepath.Join(t.TempDir(), "wakatime.log")
	if err := os.WriteFile(logFile, []byte(strings.Repeat("old log line\n", 10)), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(Options{APIURL: server.URL + "/api/v1", APIKey: "waka_test", LogFile: logFile, Timeout: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(t.Context(), "/meta"); err != nil {
		t.Fatal(err)
	}
	rotated, err := os.ReadFile(logFile + ".1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rotated), "old log line") {
		t.Fatalf("rotated log missing old contents: %q", rotated)
	}
	current, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(current), "old log line") || !strings.Contains(string(current), "/api/v1/meta status=200") {
		t.Fatalf("current log was not rewritten with fresh request only: %q", current)
	}
}

func TestRunSendsDiagnosticsOnVerboseError(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(dir, "wakatime.log")
	if err := os.WriteFile(logFile, []byte("previous log line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var diagnostics diagnosticsPayload
	var diagnosticCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/users/current/heartbeats.bulk" {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		if r.URL.Path != "/api/v1/plugins/errors" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		diagnosticCalls++
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.Header.Get("Authorization") != basicAuthHeader("waka_test") {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&diagnostics); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	err := Run([]string{
		"--entity", file,
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--log-file", logFile,
		"--verbose",
		"--send-diagnostics-on-errors",
		"--disable-offline",
		"--sync-ai-disabled",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatalf("expected command error")
	}
	if diagnosticCalls != 1 {
		t.Fatalf("diagnostic calls = %d", diagnosticCalls)
	}
	if !strings.Contains(diagnostics.ErrorMessage, "status 500") {
		t.Fatalf("diagnostic error = %#v", diagnostics)
	}
	if !strings.Contains(diagnostics.Logs, "previous log line\n") || !strings.Contains(diagnostics.Logs, "/api/v1/users/current/heartbeats.bulk status=500") {
		t.Fatalf("diagnostic logs = %q", diagnostics.Logs)
	}
	if diagnostics.Plugin != "" || diagnostics.Platform == "" || diagnostics.Architecture == "" || diagnostics.CLIVersion == "" || diagnostics.Stacktrace == "" {
		t.Fatalf("diagnostics missing metadata: %#v", diagnostics)
	}
}

func TestRunDoesNotSendDiagnosticsOnNonVerboseError(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	diagnosticCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/plugins/errors" {
			diagnosticCalls++
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()
	err := Run([]string{
		"--entity", file,
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--send-diagnostics-on-errors",
		"--disable-offline",
		"--sync-ai-disabled",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatalf("expected command error")
	}
	if diagnosticCalls != 0 {
		t.Fatalf("diagnostic calls = %d", diagnosticCalls)
	}
}

func TestDiagnosticOptionsParsesCollectFlags(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "wakatime.log")
	opts, err := diagnosticOptions([]string{
		"collect",
		"--api-url", "http://collector.example/api/v1",
		"--key", "waka_collect",
		"--log-file", logFile,
		"--verbose",
		"--send-diagnostics-on-errors",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIURL != "http://collector.example/api/v1" || opts.APIKey != "waka_collect" || opts.LogFile != logFile {
		t.Fatalf("unexpected collect diagnostic options: %#v", opts)
	}
	if !opts.Verbose || !opts.SendDiagnosticsOnError {
		t.Fatalf("collect diagnostic flags were not parsed: %#v", opts)
	}
}

func TestDiagnosticOptionsParsesAllReadCommandCommonFlags(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "wakatime.log")
	commands := [][]string{
		{"all-time"},
		{"machine-names"},
		{"user-agents"},
		{"events"},
		{"usage-events", "summary", "--range", "last_7_days"},
		{"insights", "languages", "last_7_days"},
		{"durations", "2026-06-28"},
		{"summaries", "2026-06-01", "2026-06-30"},
		{"pricing-sources"},
		{"pricing-models"},
	}
	for _, command := range commands {
		args := append([]string{}, command...)
		args = append(args,
			"--api-url", "http://diagnostics.example/api/v1",
			"--key", "waka_diagnostic",
			"--log-file", logFile,
			"--verbose",
			"--send-diagnostics-on-errors",
		)
		opts, err := diagnosticOptions(args)
		if err != nil {
			t.Fatalf("diagnosticOptions(%v): %v", command, err)
		}
		if opts.APIURL != "http://diagnostics.example/api/v1" || opts.APIKey != "waka_diagnostic" || opts.LogFile != logFile {
			t.Fatalf("diagnosticOptions(%v) missed common flags: %#v", command, opts)
		}
		if !opts.Verbose || !opts.SendDiagnosticsOnError {
			t.Fatalf("diagnosticOptions(%v) missed diagnostic flags: %#v", command, opts)
		}
	}
}

func TestRunWritesDefaultLogFileUnderWakaTimeHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WAKATIME_HOME", dir)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"grand_total":{"hours":0,"text":"1 min"},"categories":[{"hours":0,"name":"Coding","text":"1 min"}]},"has_team_features":false}`))
	}))
	defer server.Close()
	var out bytes.Buffer
	if err := Run([]string{"--api-url", server.URL + "/api/v1", "--key", "waka_test", "--today"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	logs, err := os.ReadFile(filepath.Join(dir, "wakatime.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logs), "GET "+server.URL+"/api/v1/users/current/statusbar/today status=200") {
		t.Fatalf("unexpected default logs: %s", logs)
	}
}

func TestRunTodayRendersWakaTimeStatusBarOutput(t *testing.T) {
	body := `{"data":{"grand_total":{"hours":2,"text":"2 hrs 17 mins"},"categories":[{"hours":2,"name":"Coding","text":"2 hrs 17 mins"},{"hours":0,"name":"Debugging","text":"7 secs"},{"hours":0,"name":"AI Coding","text":"6 secs"}]},"has_team_features":true}`
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := Run([]string{"today", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "2 hrs 17 mins" {
		t.Fatalf("unexpected text today output: %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"today", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--today-hide-categories", "false"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "2 hrs 17 mins Coding, 7 secs Debugging..." {
		t.Fatalf("unexpected visible-category today output: %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"today", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--today-hide-categories", "false", "--today-max-categories", "0"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "2 hrs 17 mins Coding, 7 secs Debugging, 6 secs AI Coding" {
		t.Fatalf("unexpected unlimited today output: %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"today", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--today-hide-categories", "false", "--today-max-categories", "2"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "2 hrs 17 mins Coding, 7 secs Debugging..." {
		t.Fatalf("unexpected truncated today output: %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"today", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--today-hide-categories", "true"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "2 hrs 17 mins" {
		t.Fatalf("unexpected hidden-category today output: %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"today", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--today-hide-categories", "false", "--output", "json"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"has_team_features":true`) || !strings.Contains(out.String(), `"text":"2 hrs 17 mins Coding, 7 secs Debugging..."`) {
		t.Fatalf("unexpected json today output: %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"today", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != body {
		t.Fatalf("unexpected raw today output: %q", out.String())
	}
	for _, path := range paths {
		if path != "/api/v1/users/current/statusbar/today" {
			t.Fatalf("unexpected today endpoint path: %s", path)
		}
	}
}

func TestRunTodayUsesStatusBarConfigLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "status_bar_show_categories", "true")
	cfg.Set("settings", "status_bar_hide_minutes", "true")
	cfg.Set("settings", "status_bar_max_categories", "1")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	body := `{"data":{"grand_total":{"hours":2,"text":"2 hrs 17 mins"},"categories":[{"hours":2,"name":"Coding","text":"2 hrs 17 mins"},{"hours":0,"name":"Debugging","text":"7 secs"}]},"has_team_features":false}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	var out bytes.Buffer
	if err := Run([]string{"today", "--config", config, "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "2 hrs Coding" {
		t.Fatalf("unexpected configured today output: %q", out.String())
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

func TestRunTodayExplicitZeroMaxCategoriesOverridesConfigLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "status_bar_show_categories", "true")
	cfg.Set("settings", "status_bar_max_categories", "1")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	body := `{"data":{"grand_total":{"hours":2,"text":"2 hrs 17 mins"},"categories":[{"hours":2,"name":"Coding","text":"2 hrs 17 mins"},{"hours":0,"name":"Debugging","text":"7 secs"}]},"has_team_features":false}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	var out bytes.Buffer
	if err := Run([]string{"today", "--config", config, "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--today-max-categories", "0"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "2 hrs 17 mins Coding, 7 secs Debugging" {
		t.Fatalf("unexpected explicit-zero max output: %q", out.String())
	}
}

func TestRunTodayDefaultsToTwoVisibleCategoriesLikeWakaTime(t *testing.T) {
	body := `{"data":{"grand_total":{"hours":2,"text":"2 hrs 17 mins"},"categories":[{"hours":2,"name":"Coding","text":"2 hrs 17 mins"},{"hours":0,"name":"Debugging","text":"7 secs"},{"hours":0,"name":"AI Coding","text":"6 secs"}]},"has_team_features":false}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := Run([]string{"today", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--today-hide-categories", "false"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "2 hrs 17 mins Coding, 7 secs Debugging..." {
		t.Fatalf("unexpected default visible-category output: %q", out.String())
	}
}

func TestParseCommonRejectsInvalidStatusBarOptionsLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	for key, value := range map[string]string{
		"status_bar_enabled":         "invalid",
		"status_bar_coding_activity": "invalid",
		"status_bar_show_categories": "invalid",
		"status_bar_hide_minutes":    "invalid",
		"status_bar_max_categories":  "-1",
	} {
		config := filepath.Join(dir, key+".cfg")
		cfg := Config{Sections: map[string]map[string]string{}}
		cfg.Set("settings", key, value)
		cfg.Set("settings", "api_key", "waka_test")
		if err := cfg.Write(config); err != nil {
			t.Fatal(err)
		}
		_, err := parseCommon([]string{"--config", config})
		if err == nil {
			t.Fatalf("expected invalid status bar error for %s", key)
		}
	}
	if _, err := parseCommon([]string{"--today-hide-categories", "invalid"}); err == nil {
		t.Fatalf("expected invalid today-hide-categories error")
	}
	if _, err := parseCommon([]string{"--output", "invalid"}); err == nil {
		t.Fatalf("expected invalid output error")
	}
}

func TestRunTodayGoalRendersWakaTimeGoalOutput(t *testing.T) {
	goalID := "00000000-0000-4000-8000-000000000000"
	body := `{"data":{"id":"` + goalID + `","chart_data":[{"actual_seconds_text":"12 mins"},{"actual_seconds_text":"1 hr 2 mins"}]}}`
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := Run([]string{"today-goal", goalID, "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "1 hr 2 mins" {
		t.Fatalf("unexpected today-goal text output: %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"--today-goal", goalID, "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != body {
		t.Fatalf("unexpected today-goal raw output: %q", out.String())
	}
	for _, path := range paths {
		if path != "/api/v1/users/current/goals/"+goalID {
			t.Fatalf("unexpected today-goal endpoint path: %s", path)
		}
	}
}

func TestRunTodayGoalRejectsInvalidGoalID(t *testing.T) {
	err := Run([]string{"today-goal", "not-a-uuid", "--key", "waka_test"}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "goal id invalid") {
		t.Fatalf("expected invalid goal id error, got %v", err)
	}
}

func TestRootTodayGoalEmptyValueDispatchesLikeWakaTime(t *testing.T) {
	var out bytes.Buffer
	err := Run([]string{"--today-goal", "", "--api-url", "http://example.com/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected today-goal error")
	}
	if strings.Contains(out.String(), "Usage:") {
		t.Fatalf("empty today-goal should dispatch to today-goal command, got help output: %q", out.String())
	}
	if !strings.Contains(err.Error(), "goal id invalid") {
		t.Fatalf("unexpected today-goal error: %v", err)
	}
}

func TestRunFileExpertsRendersWakaTimeOutput(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".wakatime-project"), []byte("experts-project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	body := `{"data":[{"total":{"decimal":"0.67","digital":"0:40","text":"40 mins","total_seconds":2409},"user":{"id":"u1","is_current_user":true,"long_name":"John Doe","name":"John"}},{"total":{"decimal":"0.35","digital":"0:21","text":"21 mins","total_seconds":1301},"user":{"id":"u2","is_current_user":false,"long_name":"Karl Marx","name":"Karl"}},{"total":{"decimal":"0.00","digital":"0:00","text":"0 secs","total_seconds":0},"user":{"id":"u3","is_current_user":false,"long_name":"Nick Fury","name":"Nick"}}]}`
	var payloads []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users/current/file_experts" {
			t.Fatalf("unexpected file-experts endpoint path: %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		payloads = append(payloads, payload)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := Run([]string{"file-experts", file, "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "You: 40 mins | Karl: 21 mins" {
		t.Fatalf("unexpected file-experts text output: %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"--file-experts", "--entity", file, "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "json"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"you":`) || !strings.Contains(out.String(), `"other":`) || strings.Contains(out.String(), "Nick") {
		t.Fatalf("unexpected file-experts json output: %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"file-experts", file, "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != body {
		t.Fatalf("unexpected file-experts raw output: %q", out.String())
	}

	if len(payloads) != 3 || payloads[0]["entity"] != file || payloads[0]["project"] != "experts-project" || payloads[0]["project_root_count"] == nil {
		t.Fatalf("unexpected file-experts payloads: %#v", payloads)
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

func TestRunNativeReadCommandsUseExpectedEndpoints(t *testing.T) {
	responses := map[string]string{
		"/api/v1/users/current/stats":                               `{"data":{"range":"all"}}`,
		"/api/v1/users/current":                                     `{"data":{"username":"keith"}}`,
		"/api/v1/users/current/stats/last_30_days":                  `{"data":{"range":"last_30_days"}}`,
		"/api/v1/users/current/projects":                            `{"data":[{"name":"stint"}]}`,
		"/api/v1/users/current/projects/stint%20api":                `{"data":{"project":{"name":"stint api"}}}`,
		"/api/v1/users/current/goals":                               `{"data":[]}`,
		"/api/v1/users/current/goals/goal%20with%20spaces":          `{"data":{"id":"goal with spaces"}}`,
		"/api/v1/users/current/all_time_since_today":                `{"data":{"text":"123 hrs"}}`,
		"/api/v1/users/current/machine_names":                       `{"data":["handler-machine"]}`,
		"/api/v1/users/current/external_durations":                  `{"data":[{"entity":"Planning"}]}`,
		"/api/v1/users/current/custom_pricing":                      `{"data":[{"model":"gpt-5"}]}`,
		"/api/v1/users/current/pricing/sources":                     `{"data":[{"source":"litellm"}]}`,
		"/api/v1/users/current/pricing/models":                      `{"data":[{"model":"claude"}]}`,
		"/api/v1/users/current/billing_prefs":                       `{"data":[{"agent":"codex"}]}`,
		"/api/v1/users/current/ai_costs":                            `{"data":[{"provider":"openai"}]}`,
		"/api/v1/users/current/user_agents":                         `{"data":[{"value":"stint-cli/dev"}]}`,
		"/api/v1/users/current/events":                              `{"data":[{"type":"data_dumps"}]}`,
		"/api/v1/users/current/leaderboards":                        `{"data":[{"id":"board-1"}]}`,
		"/api/v1/users/current/leaderboards/board%20with%20spaces":  `{"data":{"id":"board with spaces"}}`,
		"/api/v1/users/current/data_dumps":                          `{"data":[{"id":"dump-1","type":"heartbeats"}]}`,
		"/api/v1/users/current/custom_rules":                        `{"data":[{"id":"rule-1","action":"change"}]}`,
		"/api/v1/users/current/custom_rules_progress":               `{"data":{"status":"completed"}}`,
		"/api/v1/users/current/projects/stint%20api?range=all_time": `{"data":{"project":{"name":"stint api"},"stats":{"range":"all_time"}}}`,
		"/api/v1/meta":                                       `{"data":{"api_url":"http://stint.local/api/v1"}}`,
		"/api/v1/docs":                                       `{"openapi":"3.1.0"}`,
		"/api/v1/leaders":                                    `{"data":[{"rank":1}]}`,
		"/api/v1/leaders?country=US&language=Go":             `{"data":[{"rank":1,"language":"Go","country":"US"}]}`,
		"/api/v1/editors":                                    `{"data":[{"name":"VS Code"}]}`,
		"/api/v1/program_languages":                          `{"data":[{"name":"Go"}]}`,
		"/api/v1/users/public-user":                          `{"data":{"username":"public-user"}}`,
		"/api/v1/users/public-user/stats":                    `{"data":{"range":"all_time"}}`,
		"/api/v1/users/public-user/stats?range=last_30_days": `{"data":{"range":"last_30_days"}}`,
		"/api/v1/users/public-user/stats/last_7_days":        `{"data":{"range":"last_7_days"}}`,
		"/api/v1/users/public-user/summaries":                `{"data":[{"grand_total":{"text":"1 hr"}}]}`,
		"/api/v1/users/public-user/summaries?end=2026-06-30&start=2026-06-01":                 `{"data":[{"range":"June"}]}`,
		"/api/v1/share/share%201/stats":                                                       `{"data":{"share":"stats"}}`,
		"/api/v1/share/share%201/stats?range=last_7_days":                                     `{"data":{"share":"stats","range":"last_7_days"}}`,
		"/api/v1/share/share%201/summaries":                                                   `{"data":[{"share":"summaries"}]}`,
		"/api/v1/share/share%201/summaries?end=2026-06-30&start=2026-06-01":                   `{"data":[{"share":"summaries","range":"June"}]}`,
		"/api/v1/users/public-user/share/share%201/stats":                                     `{"data":{"user_share":"stats"}}`,
		"/api/v1/users/public-user/share/share%201/stats?range=last_30_days":                  `{"data":{"user_share":"stats","range":"last_30_days"}}`,
		"/api/v1/users/public-user/share/share%201/summaries":                                 `{"data":[{"user_share":"summaries"}]}`,
		"/api/v1/users/public-user/share/share%201/summaries?end=2026-06-30&start=2026-06-01": `{"data":[{"user_share":"summaries","range":"June"}]}`,
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
		{name: "stats all ranges", args: []string{"stats"}, want: `"range":"all"`},
		{name: "account current user", args: []string{"account"}, want: `"username":"keith"`},
		{name: "account get", args: []string{"account", "get"}, want: `"username":"keith"`},
		{name: "me alias", args: []string{"me"}, want: `"username":"keith"`},
		{name: "stats positional range", args: []string{"stats", "last_30_days"}, want: `"range":"last_30_days"`},
		{name: "projects list", args: []string{"projects"}, want: `"name":"stint"`},
		{name: "projects positional detail", args: []string{"projects", "stint api"}, want: `"name":"stint api"`},
		{name: "projects range query", args: []string{"projects", "stint api", "--range", "all_time"}, want: `"range":"all_time"`},
		{name: "goals list", args: []string{"goals"}, want: `"data":[]`},
		{name: "goals positional detail", args: []string{"goals", "goal with spaces"}, want: `"id":"goal with spaces"`},
		{name: "all time alias", args: []string{"all-time"}, want: `"text":"123 hrs"`},
		{name: "all time api-shaped alias", args: []string{"all-time-since-today"}, want: `"text":"123 hrs"`},
		{name: "machine names alias", args: []string{"machine-names"}, want: `"handler-machine"`},
		{name: "machine names api-shaped alias", args: []string{"machine_names"}, want: `"handler-machine"`},
		{name: "user agents alias", args: []string{"user-agents"}, want: `"stint-cli/dev"`},
		{name: "user agents api-shaped alias", args: []string{"user_agents"}, want: `"stint-cli/dev"`},
		{name: "external durations alias", args: []string{"external-durations"}, want: `"entity":"Planning"`},
		{name: "external durations api-shaped alias", args: []string{"external_durations"}, want: `"entity":"Planning"`},
		{name: "custom pricing alias", args: []string{"custom-pricing"}, want: `"model":"gpt-5"`},
		{name: "custom pricing api-shaped alias", args: []string{"custom_pricing"}, want: `"model":"gpt-5"`},
		{name: "pricing sources alias", args: []string{"pricing-sources"}, want: `"source":"litellm"`},
		{name: "pricing models alias", args: []string{"pricing-models"}, want: `"model":"claude"`},
		{name: "billing prefs alias", args: []string{"billing-prefs"}, want: `"agent":"codex"`},
		{name: "billing prefs api-shaped alias", args: []string{"billing_prefs"}, want: `"agent":"codex"`},
		{name: "ai costs alias", args: []string{"ai-costs"}, want: `"provider":"openai"`},
		{name: "ai costs api-shaped alias", args: []string{"ai_costs"}, want: `"provider":"openai"`},
		{name: "events", args: []string{"events"}, want: `"type":"data_dumps"`},
		{name: "leaderboards list", args: []string{"leaderboards"}, want: `"id":"board-1"`},
		{name: "leaderboards detail", args: []string{"leaderboards", "board with spaces"}, want: `"id":"board with spaces"`},
		{name: "data dumps alias", args: []string{"data-dumps"}, want: `"id":"dump-1"`},
		{name: "data dumps api-shaped alias", args: []string{"data_dumps"}, want: `"type":"heartbeats"`},
		{name: "custom rules alias", args: []string{"custom-rules"}, want: `"action":"change"`},
		{name: "custom rules api-shaped alias", args: []string{"custom_rules"}, want: `"id":"rule-1"`},
		{name: "custom rules progress subcommand", args: []string{"custom-rules", "progress"}, want: `"status":"completed"`},
		{name: "custom rules progress alias", args: []string{"custom-rules-progress"}, want: `"status":"completed"`},
		{name: "custom rules progress api-shaped alias", args: []string{"custom_rules_progress"}, want: `"status":"completed"`},
		{name: "meta", args: []string{"meta"}, want: `"api_url":"http://stint.local/api/v1"`},
		{name: "api docs", args: []string{"api-docs"}, want: `"openapi":"3.1.0"`},
		{name: "leaders", args: []string{"leaders"}, want: `"rank":1`},
		{name: "leaders filters", args: []string{"leaders", "--language", "Go", "--country", "US"}, want: `"country":"US"`},
		{name: "editors", args: []string{"editors"}, want: `"name":"VS Code"`},
		{name: "program languages", args: []string{"program-languages"}, want: `"name":"Go"`},
		{name: "program languages api-shaped alias", args: []string{"program_languages"}, want: `"name":"Go"`},
		{name: "public user", args: []string{"users", "public-user"}, want: `"username":"public-user"`},
		{name: "public user stats", args: []string{"users", "public-user", "stats"}, want: `"range":"all_time"`},
		{name: "public user stats flag range", args: []string{"users", "public-user", "stats", "--range", "last_30_days"}, want: `"range":"last_30_days"`},
		{name: "public user stats range", args: []string{"users", "public-user", "stats", "last_7_days"}, want: `"range":"last_7_days"`},
		{name: "public user summaries", args: []string{"users", "public-user", "summaries"}, want: `"grand_total"`},
		{name: "public user summaries window", args: []string{"users", "public-user", "summaries", "--start", "2026-06-01", "--end", "2026-06-30"}, want: `"range":"June"`},
		{name: "share stats", args: []string{"share", "share 1", "stats"}, want: `"share":"stats"`},
		{name: "share stats range", args: []string{"share", "share 1", "stats", "--range", "last_7_days"}, want: `"range":"last_7_days"`},
		{name: "share summaries", args: []string{"share", "share 1", "summaries"}, want: `"share":"summaries"`},
		{name: "share summaries window", args: []string{"share", "share 1", "summaries", "--start", "2026-06-01", "--end", "2026-06-30"}, want: `"range":"June"`},
		{name: "user share stats", args: []string{"users", "public-user", "share", "share 1", "stats"}, want: `"user_share":"stats"`},
		{name: "user share stats range", args: []string{"users", "public-user", "share", "share 1", "stats", "--range", "last_30_days"}, want: `"range":"last_30_days"`},
		{name: "user share summaries", args: []string{"users", "public-user", "share", "share 1", "summaries"}, want: `"user_share":"summaries"`},
		{name: "user share summaries window", args: []string{"users", "public-user", "share", "share 1", "summaries", "--start", "2026-06-01", "--end", "2026-06-30"}, want: `"range":"June"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append(append([]string{}, tt.args...), "--api-url", server.URL+"/api/v1", "--key", "waka_test", "--output", "raw-json")
			var out bytes.Buffer
			if err := Run(args, nil, &out, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(out.String()) == "" || !strings.Contains(out.String(), tt.want) {
				t.Fatalf("Run(%v) output missing %q: %q", args, tt.want, out.String())
			}
		})
	}

	for _, want := range []string{
		"/api/v1/users/current/stats",
		"/api/v1/users/current",
		"/api/v1/users/current/stats/last_30_days",
		"/api/v1/users/current/projects",
		"/api/v1/users/current/projects/stint%20api",
		"/api/v1/users/current/projects/stint%20api?range=all_time",
		"/api/v1/users/current/goals",
		"/api/v1/users/current/goals/goal%20with%20spaces",
		"/api/v1/users/current/all_time_since_today",
		"/api/v1/users/current/machine_names",
		"/api/v1/users/current/user_agents",
		"/api/v1/users/current/external_durations",
		"/api/v1/users/current/custom_pricing",
		"/api/v1/users/current/pricing/sources",
		"/api/v1/users/current/pricing/models",
		"/api/v1/users/current/billing_prefs",
		"/api/v1/users/current/ai_costs",
		"/api/v1/users/current/events",
		"/api/v1/users/current/leaderboards",
		"/api/v1/users/current/leaderboards/board%20with%20spaces",
		"/api/v1/users/current/data_dumps",
		"/api/v1/users/current/custom_rules",
		"/api/v1/users/current/custom_rules_progress",
		"/api/v1/meta",
		"/api/v1/docs",
		"/api/v1/leaders",
		"/api/v1/leaders?country=US&language=Go",
		"/api/v1/editors",
		"/api/v1/program_languages",
		"/api/v1/users/public-user",
		"/api/v1/users/public-user/stats",
		"/api/v1/users/public-user/stats?range=last_30_days",
		"/api/v1/users/public-user/stats/last_7_days",
		"/api/v1/users/public-user/summaries",
		"/api/v1/users/public-user/summaries?end=2026-06-30&start=2026-06-01",
		"/api/v1/share/share%201/stats",
		"/api/v1/share/share%201/stats?range=last_7_days",
		"/api/v1/share/share%201/summaries",
		"/api/v1/share/share%201/summaries?end=2026-06-30&start=2026-06-01",
		"/api/v1/users/public-user/share/share%201/stats",
		"/api/v1/users/public-user/share/share%201/stats?range=last_30_days",
		"/api/v1/users/public-user/share/share%201/summaries",
		"/api/v1/users/public-user/share/share%201/summaries?end=2026-06-30&start=2026-06-01",
	} {
		if !slices.Contains(paths, want) {
			t.Fatalf("expected endpoint %s in %#v", want, paths)
		}
	}
}

func TestRunDataDumpsDownloadUsesExpectedEndpoint(t *testing.T) {
	responses := map[string]string{
		"/api/v1/users/current/data_dumps/dump%201/download": `[{"entity":"main.go"}]`,
	}
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		paths = append(paths, path)
		body, ok := responses[path]
		if !ok {
			t.Fatalf("unexpected endpoint path: %s", path)
		}
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"data-dumps", "download", "dump 1", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"entity":"main.go"`) {
		t.Fatalf("download output missing dump body: %q", out.String())
	}
	if !slices.Contains(paths, "/api/v1/users/current/data_dumps/dump%201/download") {
		t.Fatalf("expected data dump download endpoint in %#v", paths)
	}
}

func TestRunDataDumpsDownloadRequiresID(t *testing.T) {
	var out bytes.Buffer
	err := Run([]string{"data-dumps", "download", "--key", "waka_test"}, nil, &out, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "usage: stint data-dumps download DUMP_ID") {
		t.Fatalf("expected usage error for missing dump id, got %v", err)
	}
}

func TestRunDataDumpsCreatePostsExpectedPayload(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantType string
		wantBody string
	}{
		{name: "positional type", args: []string{"data-dumps", "create", "heartbeats"}, wantType: "heartbeats", wantBody: `"type":"heartbeats"`},
		{name: "type flag", args: []string{"data-dumps", "create", "--type", "daily"}, wantType: "daily", wantBody: `"type":"daily"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Fatalf("method = %s, want POST", r.Method)
				}
				if r.URL.EscapedPath() != "/api/v1/users/current/data_dumps" {
					t.Fatalf("path = %s, want /api/v1/users/current/data_dumps", r.URL.EscapedPath())
				}
				var payload map[string]string
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if payload["type"] != tt.wantType {
					t.Fatalf("payload type = %q, want %q", payload["type"], tt.wantType)
				}
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"data":{"id":"dump-2","type":"` + tt.wantType + `"}}`))
			}))
			defer server.Close()

			args := append(append([]string{}, tt.args...), "--api-url", server.URL+"/api/v1", "--key", "waka_test", "--output", "raw-json")
			var out bytes.Buffer
			if err := Run(args, nil, &out, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), tt.wantBody) {
				t.Fatalf("create output missing %q: %q", tt.wantBody, out.String())
			}
		})
	}
}

func TestRunDataDumpsCreateRequiresType(t *testing.T) {
	var out bytes.Buffer
	err := Run([]string{"data-dumps", "create", "--key", "waka_test"}, nil, &out, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "usage: stint data-dumps create heartbeats|daily") {
		t.Fatalf("expected usage error for missing dump type, got %v", err)
	}
}

func TestRunDataDumpsCreateRejectsUnknownTypeBeforePosting(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"data":{"id":"dump-1"}}`))
	}))
	defer server.Close()

	err := Run([]string{"data-dumps", "create", "unknown", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "usage: stint data-dumps create heartbeats|daily") {
		t.Fatalf("expected invalid dump type usage error, got %v", err)
	}
	if called {
		t.Fatal("invalid dump type should not be posted to the API")
	}
}

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

func TestRunExternalDurationsCreatePostsRawStdin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v1/users/current/external_durations" {
			t.Fatalf("path = %s, want external durations endpoint", r.URL.EscapedPath())
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["external_id"] != "manual-1" || payload["provider"] != "manual" {
			t.Fatalf("unexpected payload: %#v", payload)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"duration-1"}}`))
	}))
	defer server.Close()

	body := `{"external_id":"manual-1","provider":"manual","entity":"Planning","type":"app","start_time":1781887000,"end_time":1781887600}`
	var out bytes.Buffer
	err := Run([]string{"external-durations", "create", "--stdin", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, strings.NewReader(body), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id":"duration-1"`) {
		t.Fatalf("create output missing response: %q", out.String())
	}
}

func TestRunExternalDurationsBulkPostsFile(t *testing.T) {
	dir := t.TempDir()
	bodyPath := filepath.Join(dir, "external-durations.json")
	if err := os.WriteFile(bodyPath, []byte(`[{"external_id":"manual-2","provider":"manual","entity":"Planning","type":"app","start_time":1781887000,"end_time":1781887600}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v1/users/current/external_durations.bulk" {
			t.Fatalf("path = %s, want external durations bulk endpoint", r.URL.EscapedPath())
		}
		var payload []map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload) != 1 || payload[0]["external_id"] != "manual-2" {
			t.Fatalf("unexpected payload: %#v", payload)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"responses":[{"status":201}]}`))
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"external-durations", "bulk", bodyPath, "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"status":201`) {
		t.Fatalf("bulk output missing response: %q", out.String())
	}
}

func TestRunExternalDurationsDeletePostsIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %s, want DELETE", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v1/users/current/external_durations.bulk" {
			t.Fatalf("path = %s, want external durations bulk endpoint", r.URL.EscapedPath())
		}
		var payload map[string][]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(payload["ids"], []string{"id-1", "id-2"}) {
			t.Fatalf("ids = %#v, want id-1/id-2", payload["ids"])
		}
		_, _ = w.Write([]byte(`{"data":{"deleted":2}}`))
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"external-durations", "delete", "--ids", "id-1,id-2", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"deleted":2`) {
		t.Fatalf("delete output missing response: %q", out.String())
	}
}

func TestRunGoalsCreatePostsRawStdin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v1/users/current/goals" {
			t.Fatalf("path = %s, want goals endpoint", r.URL.EscapedPath())
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["title"] != "CLI goal" || payload["target_seconds"] != float64(3600) {
			t.Fatalf("unexpected payload: %#v", payload)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"goal-1","title":"CLI goal"}}`))
	}))
	defer server.Close()

	body := `{"title":"CLI goal","target_seconds":3600,"range":"day"}`
	var out bytes.Buffer
	err := Run([]string{"goals", "create", "--stdin", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, strings.NewReader(body), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id":"goal-1"`) {
		t.Fatalf("create output missing response: %q", out.String())
	}
}

func TestRunGoalsUpdatePutsFileToGoalEndpoint(t *testing.T) {
	dir := t.TempDir()
	bodyPath := filepath.Join(dir, "goal.json")
	if err := os.WriteFile(bodyPath, []byte(`{"title":"Updated goal","target_seconds":7200,"range":"day"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v1/users/current/goals/goal%201" {
			t.Fatalf("path = %s, want escaped goal endpoint", r.URL.EscapedPath())
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["title"] != "Updated goal" {
			t.Fatalf("unexpected payload: %#v", payload)
		}
		_, _ = w.Write([]byte(`{"data":{"id":"goal 1","title":"Updated goal"}}`))
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"goals", "update", "goal 1", bodyPath, "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"title":"Updated goal"`) {
		t.Fatalf("update output missing response: %q", out.String())
	}
}

func TestRunGoalsDeleteUsesGoalEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %s, want DELETE", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v1/users/current/goals/goal%201" {
			t.Fatalf("path = %s, want escaped goal endpoint", r.URL.EscapedPath())
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"goals", "delete", "goal 1", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("delete should not print a 204 response body, got %q", out.String())
	}
}

func TestRunCustomPricingUpsertAndDeleteUseExpectedEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			if r.URL.EscapedPath() != "/api/v1/users/current/custom_pricing" {
				t.Fatalf("put path = %s, want custom pricing endpoint", r.URL.EscapedPath())
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["model"] != "gpt-5" {
				t.Fatalf("unexpected payload: %#v", payload)
			}
			_, _ = w.Write([]byte(`{"data":[{"model":"gpt-5"}]}`))
		case http.MethodDelete:
			if r.URL.EscapedPath() != "/api/v1/users/current/custom_pricing/gpt-5%20mini" {
				t.Fatalf("delete path = %s, want escaped custom pricing model", r.URL.EscapedPath())
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("method = %s, want PUT or DELETE", r.Method)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"custom-pricing", "upsert", "--stdin", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, strings.NewReader(`{"model":"gpt-5","input_per_million_usd":1.25,"output_per_million_usd":10}`), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"model":"gpt-5"`) {
		t.Fatalf("upsert output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"custom-pricing", "delete", "gpt-5 mini", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("delete should not print a 204 response body, got %q", out.String())
	}
}

func TestRunBillingPrefsUpsertAndDeleteUseExpectedEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			if r.URL.EscapedPath() != "/api/v1/users/current/billing_prefs" {
				t.Fatalf("put path = %s, want billing prefs endpoint", r.URL.EscapedPath())
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["agent"] != "codex" || payload["billing_type"] != "subscription" {
				t.Fatalf("unexpected payload: %#v", payload)
			}
			_, _ = w.Write([]byte(`{"data":[{"agent":"codex"}]}`))
		case http.MethodDelete:
			if r.URL.EscapedPath() != "/api/v1/users/current/billing_prefs/claude%20code" {
				t.Fatalf("delete path = %s, want escaped billing agent", r.URL.EscapedPath())
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("method = %s, want PUT or DELETE", r.Method)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"billing-prefs", "upsert", "--stdin", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, strings.NewReader(`{"agent":"codex","billing_type":"subscription"}`), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"agent":"codex"`) {
		t.Fatalf("upsert output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"billing-prefs", "delete", "claude code", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("delete should not print a 204 response body, got %q", out.String())
	}
}

func TestRunAICostsReplacePutsArrayBody(t *testing.T) {
	dir := t.TempDir()
	bodyPath := filepath.Join(dir, "ai-costs.json")
	if err := os.WriteFile(bodyPath, []byte(`[{"agent":"Codex","input_cost_per_million_cents":300,"output_cost_per_million_cents":1200}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v1/users/current/ai_costs" {
			t.Fatalf("path = %s, want ai_costs endpoint", r.URL.EscapedPath())
		}
		var payload []map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload) != 1 || payload[0]["agent"] != "Codex" {
			t.Fatalf("unexpected payload: %#v", payload)
		}
		_, _ = w.Write([]byte(`{"data":[{"agent":"Codex"}]}`))
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"ai-costs", "replace", bodyPath, "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"agent":"Codex"`) {
		t.Fatalf("replace output missing response: %q", out.String())
	}
}

func TestRunCustomRulesReplacePutsArrayBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v1/users/current/custom_rules" {
			t.Fatalf("path = %s, want custom_rules endpoint", r.URL.EscapedPath())
		}
		var payload []map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload) != 1 || payload[0]["source"] != "entity" {
			t.Fatalf("unexpected payload: %#v", payload)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"rule-1","source":"entity"}]}`))
	}))
	defer server.Close()

	body := `[{"action":"change","source":"entity","operation":"contains","source_value":"tmp","destinations":[{"destination":"project","destination_value":"scratch"}]}]`
	var out bytes.Buffer
	err := Run([]string{"custom-rules", "replace", "--stdin", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, strings.NewReader(body), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id":"rule-1"`) {
		t.Fatalf("replace output missing response: %q", out.String())
	}
}

func TestRunCustomRulesDeleteUsesRuleEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %s, want DELETE", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v1/users/current/custom_rules/rule%201" {
			t.Fatalf("path = %s, want escaped custom rule endpoint", r.URL.EscapedPath())
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"custom-rules", "delete", "rule 1", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("delete should not print a 204 response body, got %q", out.String())
	}
}

func TestRunCustomRulesAbortDeletesProgressEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %s, want DELETE", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v1/users/current/custom_rules_progress" {
			t.Fatalf("path = %s, want custom_rules_progress endpoint", r.URL.EscapedPath())
		}
		_, _ = w.Write([]byte(`{"data":{"status":"Aborted"}}`))
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"custom-rules", "abort", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"status":"Aborted"`) {
		t.Fatalf("abort output missing response: %q", out.String())
	}
}

func TestRunLeaderboardsCreateUpdateDeleteAndMembers(t *testing.T) {
	seen := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.EscapedPath()
		seen[key] = true
		switch key {
		case "POST /api/v1/users/current/leaderboards":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["name"] != "CLI board" || payload["time_range"] != "last_7_days" {
				t.Fatalf("unexpected create payload: %#v", payload)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"id":"board-1","name":"CLI board"}}`))
		case "PUT /api/v1/users/current/leaderboards/board%201":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["name"] != "Updated board" {
				t.Fatalf("unexpected update payload: %#v", payload)
			}
			_, _ = w.Write([]byte(`{"data":{"id":"board 1","name":"Updated board"}}`))
		case "DELETE /api/v1/users/current/leaderboards/board%201":
			w.WriteHeader(http.StatusNoContent)
		case "POST /api/v1/users/current/leaderboards/board%201/members":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["username"] != "octocat" {
				t.Fatalf("unexpected member payload: %#v", payload)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"username":"octocat"}}`))
		case "DELETE /api/v1/users/current/leaderboards/board%201/members/user%201":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s", key)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"leaderboards", "create", "--stdin", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, strings.NewReader(`{"name":"CLI board","time_range":"last_7_days"}`), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name":"CLI board"`) {
		t.Fatalf("create output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"leaderboards", "update", "board 1", "--stdin", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, strings.NewReader(`{"name":"Updated board","time_range":"last_30_days"}`), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name":"Updated board"`) {
		t.Fatalf("update output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"leaderboards", "add-member", "board 1", "octocat", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"username":"octocat"`) {
		t.Fatalf("add-member output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"leaderboards", "remove-member", "board 1", "user 1", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	err = Run([]string{"leaderboards", "delete", "board 1", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"POST /api/v1/users/current/leaderboards",
		"PUT /api/v1/users/current/leaderboards/board%201",
		"POST /api/v1/users/current/leaderboards/board%201/members",
		"DELETE /api/v1/users/current/leaderboards/board%201/members/user%201",
		"DELETE /api/v1/users/current/leaderboards/board%201",
	} {
		if !seen[want] {
			t.Fatalf("expected request %s in %#v", want, seen)
		}
	}
}

func TestRunShareTokensUseExpectedEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.EscapedPath() {
		case "GET /api/v1/users/current/share_tokens":
			_, _ = w.Write([]byte(`{"data":[{"id":"share-1","name":"review"}]}`))
		case "POST /api/v1/users/current/share_tokens":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["name"] != "team review" {
				t.Fatalf("unexpected payload: %#v", payload)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"id":"share-2","name":"team review"}}`))
		case "DELETE /api/v1/users/current/share_tokens/share%201":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"share-tokens", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id":"share-1"`) {
		t.Fatalf("list output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"share_tokens", "create", "team review", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name":"team review"`) {
		t.Fatalf("create output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"share-tokens", "delete", "share 1", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("delete should not print a 204 response body, got %q", out.String())
	}
}

func TestRunAccountMutationsUseExpectedEndpoints(t *testing.T) {
	var requests []string
	var updateBody map[string]any
	var deleteBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.EscapedPath())
		switch r.Method + " " + r.URL.EscapedPath() {
		case "PUT /api/v1/users/current":
			if err := json.NewDecoder(r.Body).Decode(&updateBody); err != nil {
				t.Fatalf("decode update body: %v", err)
			}
			_, _ = w.Write([]byte(`{"data":{"timezone":"America/Los_Angeles","writes_only":true}}`))
		case "DELETE /api/v1/users/current":
			if err := json.NewDecoder(r.Body).Decode(&deleteBody); err != nil {
				t.Fatalf("decode delete body: %v", err)
			}
			_, _ = w.Write([]byte(`{"data":{"deleted":true}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run(
		[]string{"account", "update", "--stdin", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"},
		strings.NewReader(`{"timezone":"America/Los_Angeles","writes_only":true}`),
		&out,
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if updateBody["timezone"] != "America/Los_Angeles" || updateBody["writes_only"] != true {
		t.Fatalf("unexpected account update body: %#v", updateBody)
	}
	if !strings.Contains(out.String(), `"writes_only":true`) {
		t.Fatalf("unexpected account update output: %q", out.String())
	}

	out.Reset()
	err = Run([]string{"account", "delete", "--confirm", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if deleteBody["confirmation"] != "DELETE" {
		t.Fatalf("unexpected account delete body: %#v", deleteBody)
	}
	if !strings.Contains(out.String(), `"deleted":true`) {
		t.Fatalf("unexpected account delete output: %q", out.String())
	}
	for _, want := range []string{
		"PUT /api/v1/users/current",
		"DELETE /api/v1/users/current",
	} {
		if !slices.Contains(requests, want) {
			t.Fatalf("expected request %s in %#v", want, requests)
		}
	}
}

func TestRunAccountDeleteRequiresConfirmation(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	err := Run([]string{"account", "delete", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--confirm") {
		t.Fatalf("expected confirmation error, got %v", err)
	}
	if called {
		t.Fatal("account delete should not call the server without --confirm")
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

func TestRunAPIKeysUseExpectedEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.EscapedPath() {
		case "GET /api/v1/api_keys":
			_, _ = w.Write([]byte(`{"data":[{"id":"key-1","name":"default"}]}`))
		case "POST /api/v1/api_keys":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["name"] != "cli key" {
				t.Fatalf("unexpected key name: %#v", payload)
			}
			scopes, ok := payload["scopes"].([]any)
			if !ok || len(scopes) != 2 || scopes[0] != "write_heartbeats" || scopes[1] != "read_stats" {
				t.Fatalf("unexpected scopes: %#v", payload["scopes"])
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"api_key":"waka_new","key":{"id":"key-2","name":"cli key"}}}`))
		case "DELETE /api/v1/api_keys/key%201":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"api-keys", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id":"key-1"`) {
		t.Fatalf("list output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"api-keys", "create", "cli key", "--scope", "write_heartbeats", "--scope", "read_stats", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"api_key":"waka_new"`) {
		t.Fatalf("create output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"api-keys", "delete", "key 1", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("delete should not print a 204 response body, got %q", out.String())
	}
}

func TestRunOAuthAppsUseExpectedEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.EscapedPath() {
		case "GET /api/v1/oauth/apps":
			_, _ = w.Write([]byte(`{"data":[{"id":"app-1","name":"Local app"}]}`))
		case "POST /api/v1/oauth/apps":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["name"] != "CLI OAuth" {
				t.Fatalf("unexpected app name: %#v", payload)
			}
			redirects, ok := payload["redirect_uris"].([]any)
			if !ok || len(redirects) != 1 || redirects[0] != "http://localhost:3000/callback" {
				t.Fatalf("unexpected redirect_uris: %#v", payload["redirect_uris"])
			}
			scopes, ok := payload["scopes"].([]any)
			if !ok || len(scopes) != 1 || scopes[0] != "read_stats" {
				t.Fatalf("unexpected scopes: %#v", payload["scopes"])
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"id":"app-2","name":"CLI OAuth","client_id":"client-1"}}`))
		case "DELETE /api/v1/oauth/apps/app%201":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"oauth-apps", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id":"app-1"`) {
		t.Fatalf("list output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"oauth", "apps", "create", "CLI OAuth", "--redirect-uri", "http://localhost:3000/callback", "--scope", "read_stats", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"client_id":"client-1"`) {
		t.Fatalf("create output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"oauth-apps", "delete", "app 1", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("delete should not print a 204 response body, got %q", out.String())
	}
}

func TestRunOAuthTokenAndRevokeUseExpectedEndpoints(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.EscapedPath())
		if got := r.Header.Get("Authorization"); got != basicAuthHeader("client-1:secret-1") {
			t.Fatalf("Authorization = %q, want OAuth client basic auth", got)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/x-www-form-urlencoded") {
			t.Fatalf("Content-Type = %q, want form", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		switch r.Method + " " + r.URL.EscapedPath() {
		case "POST /oauth/token":
			switch r.Form.Get("grant_type") {
			case "authorization_code":
				if r.Form.Get("code") != "auth-code" || r.Form.Get("redirect_uri") != "http://localhost/callback" {
					t.Fatalf("unexpected authorization_code form: %#v", r.Form)
				}
				_, _ = w.Write([]byte(`{"access_token":"access-1","refresh_token":"refresh-1","token_type":"Bearer"}`))
			case "refresh_token":
				if r.Form.Get("refresh_token") != "refresh-1" {
					t.Fatalf("unexpected refresh_token form: %#v", r.Form)
				}
				_, _ = w.Write([]byte(`{"access_token":"access-2","refresh_token":"refresh-2","token_type":"Bearer"}`))
			default:
				t.Fatalf("unexpected token grant form: %#v", r.Form)
			}
		case "POST /oauth/revoke":
			if r.Form.Get("token") != "access-1" {
				t.Fatalf("unexpected revoke form: %#v", r.Form)
			}
			_, _ = w.Write([]byte(`{"revoked":true}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"oauth", "token", "--client-id", "client-1", "--client-secret", "secret-1", "--code", "auth-code", "--redirect-uri", "http://localhost/callback", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"access_token":"access-1"`) {
		t.Fatalf("authorization_code output missing response: %q", out.String())
	}

	out.Reset()
	err = Run([]string{"oauth", "token", "--client-id", "client-1", "--client-secret", "secret-1", "--refresh-token", "refresh-1", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"access_token":"access-2"`) {
		t.Fatalf("refresh_token output missing response: %q", out.String())
	}

	out.Reset()
	err = Run([]string{"oauth", "revoke", "access-1", "--client-id", "client-1", "--client-secret", "secret-1", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"revoked":true`) {
		t.Fatalf("revoke output missing response: %q", out.String())
	}

	for _, want := range []string{
		"POST /oauth/token",
		"POST /oauth/revoke",
	} {
		if !slices.Contains(seen, want) {
			t.Fatalf("expected request %s in %#v", want, seen)
		}
	}
}

func TestRunOperationalCommandsUsePublicEndpointsWithoutAPIKey(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.RequestURI())
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Fatalf("Authorization header should be empty for public operational command, got %q", auth)
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /healthz":
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "GET /healthz/ingestion":
			_, _ = w.Write([]byte(`{"ok":true,"count_last_hour":3}`))
		case "POST /api/v1/dev/seed-key":
			if r.URL.Query().Get("github_id") != "4001" || r.URL.Query().Get("username") != "dev-user" {
				t.Fatalf("unexpected seed-key query: %s", r.URL.RawQuery)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"api_key":"waka_dev"}}`))
		case "POST /api/v1/dev/jobs/heartbeats-purge":
			if r.URL.Query().Get("retention_days") != "0" {
				t.Fatalf("unexpected purge query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"data":{"queued":false,"deleted":7}}`))
		case "POST /api/v1/dev/jobs/leaderboard-update":
			if r.URL.Query().Get("range") != "last_30_days" {
				t.Fatalf("unexpected leaderboard query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"data":{"queued":false,"entries":2}}`))
		case "POST /api/v1/dev/jobs/goals-evaluate":
			if r.URL.Query().Get("now_unix") != "1780000000" {
				t.Fatalf("unexpected goals query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"data":{"queued":false,"evaluated":4}}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "health", args: []string{"health"}, want: `"ok":true`},
		{name: "ingestion health", args: []string{"health", "ingestion"}, want: `"count_last_hour":3`},
		{name: "dev seed key", args: []string{"dev", "seed-key", "--github-id", "4001", "--username", "dev-user"}, want: `"api_key":"waka_dev"`},
		{name: "dev heartbeats purge", args: []string{"dev", "heartbeats-purge", "--retention-days", "0"}, want: `"deleted":7`},
		{name: "dev leaderboard update", args: []string{"dev", "leaderboard-update", "--range", "last_30_days"}, want: `"entries":2`},
		{name: "dev goals evaluate", args: []string{"dev", "goals-evaluate", "--now-unix", "1780000000"}, want: `"evaluated":4`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append(append([]string{}, tt.args...), "--api-url", server.URL+"/api/v1", "--output", "raw-json")
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
		"GET /healthz",
		"GET /healthz/ingestion",
		"POST /api/v1/dev/seed-key?github_id=4001&username=dev-user",
		"POST /api/v1/dev/jobs/heartbeats-purge?retention_days=0",
		"POST /api/v1/dev/jobs/leaderboard-update?range=last_30_days",
		"POST /api/v1/dev/jobs/goals-evaluate?now_unix=1780000000",
	} {
		if !slices.Contains(seen, want) {
			t.Fatalf("expected request %s in %#v", want, seen)
		}
	}
}

func TestRunDurationsUsesExpectedEndpointAndQuery(t *testing.T) {
	responses := map[string]string{
		"/api/v1/users/current/durations":                                     `{"data":[{"project":"stint"}]}`,
		"/api/v1/users/current/durations?date=2026-06-28":                     `{"data":[{"date":"2026-06-28"}]}`,
		"/api/v1/users/current/durations?date=2026-06-28&slice_by=language":   `{"data":[{"language":"Go"}]}`,
		"/api/v1/users/current/durations?date=2026-06-28&slice_by=dependency": `{"data":[{"dependency":"echo"}]}`,
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
		{name: "default day", args: []string{"durations"}, want: `"project":"stint"`},
		{name: "positional date", args: []string{"durations", "2026-06-28"}, want: `"date":"2026-06-28"`},
		{name: "slice by flag", args: []string{"durations", "2026-06-28", "--slice-by", "language"}, want: `"language":"Go"`},
		{name: "slice by underscore alias", args: []string{"durations", "--date", "2026-06-28", "--slice_by", "dependency"}, want: `"dependency":"echo"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append(append([]string{}, tt.args...), "--api-url", server.URL+"/api/v1", "--key", "waka_test", "--output", "raw-json")
			var out bytes.Buffer
			if err := Run(args, nil, &out, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(out.String()) == "" || !strings.Contains(out.String(), tt.want) {
				t.Fatalf("Run(%v) output missing %q: %q", args, tt.want, out.String())
			}
		})
	}

	for _, want := range []string{
		"/api/v1/users/current/durations",
		"/api/v1/users/current/durations?date=2026-06-28",
		"/api/v1/users/current/durations?date=2026-06-28&slice_by=language",
		"/api/v1/users/current/durations?date=2026-06-28&slice_by=dependency",
	} {
		if !slices.Contains(paths, want) {
			t.Fatalf("expected endpoint %s in %#v", want, paths)
		}
	}
}

func TestRunSummariesUsesExpectedEndpointAndQuery(t *testing.T) {
	responses := map[string]string{
		"/api/v1/users/current/summaries":                                 `{"data":[{"range":"default"}]}`,
		"/api/v1/users/current/summaries?start=2026-06-01":                `{"data":[{"start":"2026-06-01"}]}`,
		"/api/v1/users/current/summaries?end=2026-06-30&start=2026-06-01": `{"data":[{"end":"2026-06-30"}]}`,
		"/api/v1/users/current/summaries?end=2026-06-29&start=2026-06-28": `{"data":[{"end":"2026-06-29"}]}`,
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
		{name: "default range", args: []string{"summaries"}, want: `"range":"default"`},
		{name: "positional start", args: []string{"summaries", "2026-06-01"}, want: `"start":"2026-06-01"`},
		{name: "positional start end", args: []string{"summaries", "2026-06-01", "2026-06-30"}, want: `"end":"2026-06-30"`},
		{name: "start end flags", args: []string{"summaries", "--start", "2026-06-28", "--end", "2026-06-29"}, want: `"end":"2026-06-29"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append(append([]string{}, tt.args...), "--api-url", server.URL+"/api/v1", "--key", "waka_test", "--output", "raw-json")
			var out bytes.Buffer
			if err := Run(args, nil, &out, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(out.String()) == "" || !strings.Contains(out.String(), tt.want) {
				t.Fatalf("Run(%v) output missing %q: %q", args, tt.want, out.String())
			}
		})
	}

	for _, want := range []string{
		"/api/v1/users/current/summaries",
		"/api/v1/users/current/summaries?start=2026-06-01",
		"/api/v1/users/current/summaries?end=2026-06-30&start=2026-06-01",
		"/api/v1/users/current/summaries?end=2026-06-29&start=2026-06-28",
	} {
		if !slices.Contains(paths, want) {
			t.Fatalf("expected endpoint %s in %#v", want, paths)
		}
	}
}

func TestRunHeartbeatsListUsesExpectedEndpointAndQuery(t *testing.T) {
	responses := map[string]string{
		"/api/v1/users/current/heartbeats":                 `{"data":[{"entity":"/tmp/default.go"}]}`,
		"/api/v1/users/current/heartbeats?date=2026-06-28": `{"data":[{"entity":"/tmp/date.go"}]}`,
		"/api/v1/users/current/heartbeats?date=2026-06-29": `{"data":[{"entity":"/tmp/flag.go"}]}`,
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
		{name: "default day", args: []string{"heartbeats"}, want: `"/tmp/default.go"`},
		{name: "positional date", args: []string{"heartbeats", "2026-06-28"}, want: `"/tmp/date.go"`},
		{name: "date flag", args: []string{"heartbeats", "--date", "2026-06-29"}, want: `"/tmp/flag.go"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append(append([]string{}, tt.args...), "--api-url", server.URL+"/api/v1", "--key", "waka_test", "--output", "raw-json")
			var out bytes.Buffer
			if err := Run(args, nil, &out, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(out.String()) == "" || !strings.Contains(out.String(), tt.want) {
				t.Fatalf("Run(%v) output missing %q: %q", args, tt.want, out.String())
			}
		})
	}

	for _, want := range []string{
		"/api/v1/users/current/heartbeats",
		"/api/v1/users/current/heartbeats?date=2026-06-28",
		"/api/v1/users/current/heartbeats?date=2026-06-29",
	} {
		if !slices.Contains(paths, want) {
			t.Fatalf("expected endpoint %s in %#v", want, paths)
		}
	}
}

func TestRunInsightsUsesExpectedEndpoint(t *testing.T) {
	responses := map[string]string{
		"/api/v1/users/current/insights/languages/last_7_days":      `{"data":[{"name":"Go"}]}`,
		"/api/v1/users/current/insights/daily_average/last_30_days": `{"data":{"seconds":123}}`,
		"/api/v1/users/current/insights/operating_systems/2026-06":  `{"data":[{"name":"Linux"}]}`,
		"/api/v1/users/current/insights/categories/last%20range":    `{"data":[{"name":"coding"}]}`,
	}
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
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
		{name: "positional", args: []string{"insights", "languages", "last_7_days"}, want: `"name":"Go"`},
		{name: "flag form", args: []string{"insights", "--type", "daily_average", "--range", "last_30_days"}, want: `"seconds":123`},
		{name: "api-shaped type flag", args: []string{"insights", "--insight-type", "operating_systems", "--range", "2026-06"}, want: `"name":"Linux"`},
		{name: "path escaped range", args: []string{"insights", "categories", "last range"}, want: `"name":"coding"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append(append([]string{}, tt.args...), "--api-url", server.URL+"/api/v1", "--key", "waka_test", "--output", "raw-json")
			var out bytes.Buffer
			if err := Run(args, nil, &out, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(out.String()) == "" || !strings.Contains(out.String(), tt.want) {
				t.Fatalf("Run(%v) output missing %q: %q", args, tt.want, out.String())
			}
		})
	}

	for _, want := range []string{
		"/api/v1/users/current/insights/languages/last_7_days",
		"/api/v1/users/current/insights/daily_average/last_30_days",
		"/api/v1/users/current/insights/operating_systems/2026-06",
		"/api/v1/users/current/insights/categories/last%20range",
	} {
		if !slices.Contains(paths, want) {
			t.Fatalf("expected endpoint %s in %#v", want, paths)
		}
	}
}

func TestRunInsightsRequiresTypeAndRange(t *testing.T) {
	err := Run([]string{"insights", "languages", "--key", "waka_test"}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "usage: stint insights TYPE RANGE") {
		t.Fatalf("expected insights usage error, got %v", err)
	}
}

func TestRunUsageEventsUsesExpectedEndpointsAndQueries(t *testing.T) {
	responses := map[string]string{
		"/api/v1/users/current/usage_events":                                                                            `{"data":[{"model":"default"}]}`,
		"/api/v1/users/current/usage_events?end=2026-06-30&start=2026-06-01":                                            `{"data":[{"model":"claude"}]}`,
		"/api/v1/users/current/usage_events/summary?agent=codex&cost_mode=calculate&range=last_30_days":                 `{"data":{"total":12}}`,
		"/api/v1/users/current/usage_events/blocks?cost_mode=display&end=2026-06-30&range=last_7_days&start=2026-06-01": `{"data":[{"block":"five-hour"}]}`,
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
		{name: "list default", args: []string{"usage-events"}, want: `"model":"default"`},
		{name: "list start end", args: []string{"usage-events", "--start", "2026-06-01", "--end", "2026-06-30"}, want: `"model":"claude"`},
		{name: "summary", args: []string{"usage-events", "summary", "--range", "last_30_days", "--cost-mode", "calculate", "--agent", "codex"}, want: `"total":12`},
		{name: "blocks", args: []string{"usage-events", "blocks", "--range", "last_7_days", "--start", "2026-06-01", "--end", "2026-06-30", "--cost_mode", "display"}, want: `"block":"five-hour"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append(append([]string{}, tt.args...), "--api-url", server.URL+"/api/v1", "--key", "waka_test", "--output", "raw-json")
			var out bytes.Buffer
			if err := Run(args, nil, &out, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(out.String()) == "" || !strings.Contains(out.String(), tt.want) {
				t.Fatalf("Run(%v) output missing %q: %q", args, tt.want, out.String())
			}
		})
	}

	for _, want := range []string{
		"/api/v1/users/current/usage_events",
		"/api/v1/users/current/usage_events?end=2026-06-30&start=2026-06-01",
		"/api/v1/users/current/usage_events/summary?agent=codex&cost_mode=calculate&range=last_30_days",
		"/api/v1/users/current/usage_events/blocks?cost_mode=display&end=2026-06-30&range=last_7_days&start=2026-06-01",
	} {
		if !slices.Contains(paths, want) {
			t.Fatalf("expected endpoint %s in %#v", want, paths)
		}
	}
}

func TestWriteFileExpertsEmptyResponseHasNoOutputForAnyFormat(t *testing.T) {
	for _, format := range []string{"", "text", "json", "raw-json"} {
		var out bytes.Buffer
		if err := writeFileExpertsOutput(&out, format, []byte(`{"data":[]}`)); err != nil {
			t.Fatalf("format %q: %v", format, err)
		}
		if out.String() != "" {
			t.Fatalf("format %q output = %q", format, out.String())
		}
	}
}

func TestRunDoctorJSONOutputStaysValidJSON(t *testing.T) {
	body := `{"data":{"api_url":"http://example.test/api/v1","version":"dev"}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/meta" {
			t.Fatalf("unexpected doctor endpoint path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	var out bytes.Buffer
	queuePath := filepath.Join(t.TempDir(), "offline_heartbeats.bdb")
	if err := Run([]string{
		"doctor",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", queuePath,
		"--output", "json",
	}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("doctor --output json produced invalid JSON %q: %v", out.String(), err)
	}
	if got["offline_queue_count"] != float64(0) {
		t.Fatalf("expected offline_queue_count=0, got %#v from %q", got, out.String())
	}
	if data, ok := got["data"].(map[string]any); !ok || data["version"] != "dev" {
		t.Fatalf("expected meta data to be preserved, got %#v", got)
	}
}

func TestOfflineQueueCountPrintAndSync(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	heartbeats := []Heartbeat{{Entity: "/tmp/a.go", EntityType: "file", Time: 1}, {Entity: "/tmp/b.go", EntityType: "file", Time: 2}}
	if err := AppendQueue(queue, heartbeats); err != nil {
		t.Fatal(err)
	}
	count, err := CountQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("count = %d", count)
	}
	var posted []Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()
	var out bytes.Buffer
	if err := Run([]string{"offline", "sync", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--offline-queue-file", queue}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 2 {
		t.Fatalf("posted = %d", len(posted))
	}
	count, err = CountQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("count after sync = %d", count)
	}
}

func TestOfflineQueueFileHomeExpansionErrorMatchesWakaTime(t *testing.T) {
	err := Run([]string{
		"--offline-count",
		"--offline-queue-file", "~missing-user/offline_heartbeats.bdb",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "failed expanding offline-queue-file param") {
		t.Fatalf("expected offline queue expansion error, got %v", err)
	}
}

func TestSSLCertsFileHomeExpansionErrorMatchesWakaTime(t *testing.T) {
	_, err := parseCommon([]string{
		"--key", "waka_test",
		"--ssl-certs-file", "~missing-user/certs.pem",
	})
	if err == nil || !strings.Contains(err.Error(), "failed expanding ssl certs file") {
		t.Fatalf("expected ssl certs expansion error, got %v", err)
	}
}

func TestLogFileHomeExpansionErrorMatchesWakaTime(t *testing.T) {
	_, err := parseCommon([]string{
		"--key", "waka_test",
		"--log-file", "~missing-user/wakatime.log",
	})
	if err == nil || !strings.Contains(err.Error(), "failed to expand log file") {
		t.Fatalf("expected log file expansion error, got %v", err)
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

func TestOfflineSyncFansOutToConfiguredAPIURLs(t *testing.T) {
	dir := t.TempDir()
	queue := filepath.Join(dir, "offline.bdb")
	heartbeats := []Heartbeat{{Entity: "/work/projects/main.go", EntityType: "file", Time: 1}}
	if err := AppendQueue(queue, heartbeats); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counts[r.Header.Get("Authorization")]++
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "api_url", server.URL+"/api/v1")
	cfg.Set("settings", "api_key", "waka_default")
	cfg.Set("api_urls", `/work/`, server.URL+"/api/v1|waka_fanout")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"offline", "sync", "--config", config, "--offline-queue-file", queue}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if counts[basicAuthHeader("waka_default")] != 1 || counts[basicAuthHeader("waka_fanout")] != 1 {
		t.Fatalf("unexpected fanout counts: %#v", counts)
	}
	count, err := CountQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("count after sync = %d", count)
	}
}

func TestPrintOfflineDefaultLimitMatchesWakaTime(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	var heartbeats []Heartbeat
	for i := 0; i < defaultPrintOfflineMax+2; i++ {
		heartbeats = append(heartbeats, Heartbeat{Entity: fmt.Sprintf("/tmp/%02d.go", i), EntityType: "file", Time: float64(i + 1)})
	}
	heartbeats[0].Entity = "/tmp/<main>&.go"
	if err := AppendQueue(queue, heartbeats); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run([]string{"offline", "print", "--offline-queue-file", queue, "--key", "waka_test"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var printed []Heartbeat
	if err := json.Unmarshal(out.Bytes(), &printed); err != nil {
		t.Fatal(err)
	}
	if len(printed) != defaultPrintOfflineMax {
		t.Fatalf("printed = %d", len(printed))
	}
	if strings.Contains(out.String(), "\n  ") || !strings.Contains(out.String(), `"/tmp/<main>&.go"`) {
		t.Fatalf("unexpected offline print format: %q", out.String())
	}
}

func TestPrintOfflineExplicitZeroPrintsNoHeartbeatsLikeWakaTime(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	if err := AppendQueue(queue, []Heartbeat{{Entity: "/tmp/queued.go", EntityType: "file", Time: 1}}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run([]string{"offline", "print", "--print-offline-heartbeats", "0", "--offline-queue-file", queue, "--key", "waka_test"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestSendHeartbeatsQueuesExtraHeartbeatsOverWakaTimeSendLimit(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	heartbeats := make([]Heartbeat, 0, defaultHeartbeatLimit+2)
	for i := 0; i < defaultHeartbeatLimit+2; i++ {
		heartbeats = append(heartbeats, Heartbeat{
			Entity:     fmt.Sprintf("/tmp/live-%02d.go", i),
			EntityType: "file",
			Time:       float64(i + 1),
		})
	}
	var posted []Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()

	err := sendHeartbeats(&bytes.Buffer{}, Options{
		APIKey:             "waka_test",
		APIURL:             server.URL + "/api/v1",
		HeartbeatRateLimit: 0,
		InternalConfigPath: filepath.Join(t.TempDir(), "wakatime-internal.cfg"),
		QueuePath:          queue,
		Timeout:            1,
	}, heartbeats, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(posted) != defaultHeartbeatLimit {
		t.Fatalf("posted = %d", len(posted))
	}
	queued, err := ReadQueue(queue, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 2 || queued[0].Entity != "/tmp/live-10.go" || queued[1].Entity != "/tmp/live-11.go" {
		t.Fatalf("queued extras = %#v", queued)
	}
}

func TestRootEntityTakesPrecedenceOverExplicitSyncFlags(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	queue := filepath.Join(dir, "offline.bdb")
	if err := AppendQueue(queue, []Heartbeat{{Entity: "/tmp/queued.go", EntityType: "file", Time: 1}}); err != nil {
		t.Fatal(err)
	}

	var calls [][]Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		calls = append(calls, posted)
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()

	if err := Run([]string{
		"--entity", file,
		"--sync-offline-activity", "0",
		"--sync-ai-activity",
		"--sync-ai-disabled",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", queue,
		"--heartbeat-rate-limit-seconds", "0",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected heartbeat then offline sync calls, got %#v", calls)
	}
	if len(calls[0]) != 1 || calls[0][0].Entity != file {
		t.Fatalf("first call should send entity heartbeat, got %#v", calls[0])
	}
	if len(calls[1]) != 1 || calls[1][0].Entity != "/tmp/queued.go" {
		t.Fatalf("second call should sync queued heartbeat, got %#v", calls[1])
	}
}

func TestRootEntityOfflineSyncHonorsExplicitSyncLimitLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	queue := filepath.Join(dir, "offline.bdb")
	queued := []Heartbeat{
		{Entity: "/tmp/queued-1.go", EntityType: "file", Time: 1},
		{Entity: "/tmp/queued-2.go", EntityType: "file", Time: 2},
	}
	if err := AppendQueue(queue, queued); err != nil {
		t.Fatal(err)
	}

	var calls [][]Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		calls = append(calls, posted)
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()

	if err := Run([]string{
		"--entity", file,
		"--sync-offline-activity", "1",
		"--sync-ai-disabled",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", queue,
		"--heartbeat-rate-limit-seconds", "0",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected live heartbeat plus one offline sync call, got %#v", calls)
	}
	if len(calls[1]) != 1 || calls[1][0].Entity != "/tmp/queued-1.go" {
		t.Fatalf("explicit sync limit should post one queued heartbeat, got %#v", calls[1])
	}
	remaining, err := ReadQueue(queue, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].Entity != "/tmp/queued-2.go" {
		t.Fatalf("expected second queued heartbeat to remain, got %#v", remaining)
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

func TestRunExtraHeartbeatsAreProcessedLikeMainHeartbeat(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".wakatime-project"), []byte("extra-project\nfeature\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mainFile := filepath.Join(dir, "main.go")
	extraFile := filepath.Join(dir, "extra.py")
	skippedFile := filepath.Join(dir, "skip.py")
	for path, data := range map[string][]byte{
		mainFile:    []byte("package main\n"),
		extraFile:   []byte("import requests\nprint('ok')\n"),
		skippedFile: []byte("print('skip')\n"),
	} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cursor := 12
	line := 2
	stdin, err := json.Marshal([]Heartbeat{
		{
			Entity:         extraFile,
			EntityType:     "file",
			Time:           123,
			CursorPosition: &cursor,
			LineNumber:     &line,
		},
		{
			Entity:     skippedFile,
			EntityType: "file",
			Time:       124,
		},
	})
	if err != nil {
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
		"--entity", mainFile,
		"--extra-heartbeats",
		"--exclude", "skip\\.py$",
		"--hide-file-names", "true",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", filepath.Join(dir, "queue.bdb"),
		"--heartbeat-rate-limit-seconds", "0",
		"--sync-ai-disabled",
	}, bytes.NewReader(stdin), &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 2 {
		t.Fatalf("posted = %#v", posted)
	}
	extra := posted[1]
	if extra.Entity != "HIDDEN.py" || extra.Project != "extra-project" || extra.Language != "Python" {
		t.Fatalf("extra heartbeat was not enriched and sanitized: %#v", extra)
	}
	if extra.Branch != "" || extra.CursorPosition != nil || extra.LineNumber != nil || extra.Lines != nil || extra.ProjectRootCount != nil || extra.Dependencies != nil {
		t.Fatalf("extra heartbeat leaked sanitized metadata: %#v", extra)
	}
}

func TestRunExtraHeartbeatNullCategoryIsUndefinedLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	mainFile := filepath.Join(dir, "main.go")
	extraFile := filepath.Join(dir, "extra.go")
	for _, file := range []string{mainFile, extraFile} {
		if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stdin := strings.NewReader(`[` +
		`{"entity":` + strconv.Quote(extraFile) + `,"type":"file","time":123,"category":"null"}` +
		`]`)
	var posted []Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()

	if err := Run([]string{
		"--entity", mainFile,
		"--category", "debugging",
		"--extra-heartbeats",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", filepath.Join(dir, "queue.bdb"),
		"--heartbeat-rate-limit-seconds", "0",
		"--sync-ai-disabled",
	}, stdin, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 2 {
		t.Fatalf("posted = %#v", posted)
	}
	if posted[1].Category != "" {
		t.Fatalf("extra heartbeat null category should be omitted, got %#v", posted[1])
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

func TestRunExtraHeartbeatsIgnoreProvidedUserAgentLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	mainFile := filepath.Join(dir, "main.go")
	extraFile := filepath.Join(dir, "extra.go")
	for _, file := range []string{mainFile, extraFile} {
		if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var posted []Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()

	stdin := strings.NewReader(`[{"entity":` + strconv.Quote(extraFile) + `,"type":"file","time":123,"user_agent":"spoofed/1.0"}]`)
	if err := Run([]string{
		"--entity", mainFile,
		"--extra-heartbeats",
		"--plugin", "plugin/0.0.1",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", filepath.Join(dir, "queue.bdb"),
		"--heartbeat-rate-limit-seconds", "0",
		"--sync-ai-disabled",
	}, stdin, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 2 {
		t.Fatalf("posted = %#v", posted)
	}
	want := userAgent("plugin/0.0.1")
	if posted[1].UserAgent != want || strings.Contains(posted[1].UserAgent, "spoofed") {
		t.Fatalf("extra user_agent = %q, want %q", posted[1].UserAgent, want)
	}
}

func TestRunExtraHeartbeatsWithEmptyStdinSendsMainHeartbeat(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
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
		"--entity", file,
		"--extra-heartbeats",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", filepath.Join(dir, "queue.bdb"),
		"--heartbeat-rate-limit-seconds", "0",
		"--sync-ai-disabled",
	}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 1 || posted[0].Entity != file {
		t.Fatalf("posted = %#v", posted)
	}
}

func TestRunExtraHeartbeatsWithMalformedStdinSendsMainHeartbeat(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
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
		"--entity", file,
		"--extra-heartbeats",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", filepath.Join(dir, "queue.bdb"),
		"--heartbeat-rate-limit-seconds", "0",
		"--sync-ai-disabled",
	}, strings.NewReader(`[{"entity":"extra.go"}]`), &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 1 || posted[0].Entity != file {
		t.Fatalf("posted = %#v", posted)
	}
}

func TestDecodeExtraHeartbeatsAcceptsWakaTimeStringValues(t *testing.T) {
	got, err := decodeExtraHeartbeats(strings.NewReader(`[
		{"entity":"main.go","entity_type":"file","time":"1585598059","cursorpos":"12","lineno":"42","lines":"45","is_unsaved_entity":"true","is_write":"true","ai_input_tokens":"100","alternate_branch":"fallback-branch","alternate_language":"Golang","alternate_project":"fallback-project","local_file":"local-main.go"},
		{"entity":"other.py","type":"file","timestamp":"1585598060","cursorpos":13,"lineno":43,"lines":46,"is_unsaved_entity":true,"is_write":true}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("heartbeats = %#v", got)
	}
	first := got[0]
	if first.Time != 1585598059 || !first.IsUnsavedEntity || !first.IsWrite {
		t.Fatalf("first scalar fields = %#v", first)
	}
	if first.CursorPosition == nil || *first.CursorPosition != 12 || first.LineNumber == nil || *first.LineNumber != 42 || first.Lines == nil || *first.Lines != 45 {
		t.Fatalf("first integer fields = %#v", first)
	}
	if first.AIInputTokens == nil || *first.AIInputTokens != 100 {
		t.Fatalf("first ai tokens = %#v", first.AIInputTokens)
	}
	if first.EntityType != "file" || first.AlternateBranch != "fallback-branch" || first.AlternateLanguage != "Golang" || first.AlternateProject != "fallback-project" || first.LocalFile != "local-main.go" {
		t.Fatalf("first internal fields = %#v", first)
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "alternate_") || strings.Contains(string(encoded), "local_file") {
		t.Fatalf("internal extra heartbeat fields leaked into JSON: %s", encoded)
	}
	second := got[1]
	if second.Time != 1585598060 || second.CursorPosition == nil || *second.CursorPosition != 13 {
		t.Fatalf("second fields = %#v", second)
	}
}

func TestRunExtraHeartbeatsPreserveExplicitFalseWriteLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	mainFile := filepath.Join(dir, "main.go")
	extraFile := filepath.Join(dir, "extra.go")
	for _, file := range []string{mainFile, extraFile} {
		if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var posted []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"responses":[]}`))
	}))
	defer server.Close()

	stdin := strings.NewReader(`[{"entity":` + strconv.Quote(extraFile) + `,"type":"file","time":123,"is_write":false}]`)
	if err := Run([]string{
		"--entity", mainFile,
		"--extra-heartbeats",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--heartbeat-rate-limit-seconds", "0",
		"--sync-ai-disabled",
	}, stdin, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 2 {
		t.Fatalf("posted = %#v", posted)
	}
	value, ok := posted[1]["is_write"]
	if !ok || value != false {
		t.Fatalf("extra is_write = %#v present=%v payload=%#v", value, ok, posted[1])
	}
}

func TestDecodeExtraHeartbeatsFallsBackFromZeroTimeToTimestampLikeWakaTime(t *testing.T) {
	got, err := decodeExtraHeartbeats(strings.NewReader(`[{"entity":"main.go","type":"file","time":0,"timestamp":1585598060}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Time != 1585598060 {
		t.Fatalf("heartbeats = %#v", got)
	}
}

func TestDecodeExtraHeartbeatsTruncatesJSONNumberIntegerFieldsLikeWakaTime(t *testing.T) {
	got, err := decodeExtraHeartbeats(strings.NewReader(`[{
		"entity":"main.go",
		"type":"file",
		"time":1,
		"cursorpos":12.9,
		"lineno":42.8,
		"lines":45.7,
		"ai_input_tokens":100.6,
		"ai_line_changes":5.9,
		"ai_output_tokens":200.4,
		"ai_prompt_length":300.2,
		"human_line_changes":3.8
	}]`))
	if err != nil {
		t.Fatal(err)
	}
	first := got[0]
	for name, got := range map[string]*int{
		"cursorpos":          first.CursorPosition,
		"lineno":             first.LineNumber,
		"lines":              first.Lines,
		"ai_input_tokens":    first.AIInputTokens,
		"ai_line_changes":    first.AILineChanges,
		"ai_output_tokens":   first.AIOutputTokens,
		"ai_prompt_length":   first.AIPromptLength,
		"human_line_changes": first.HumanLineChanges,
	} {
		if got == nil {
			t.Fatalf("%s was nil", name)
		}
	}
	if *first.CursorPosition != 12 || *first.LineNumber != 42 || *first.Lines != 45 ||
		*first.AIInputTokens != 100 || *first.AILineChanges != 5 || *first.AIOutputTokens != 200 ||
		*first.AIPromptLength != 300 || *first.HumanLineChanges != 3 {
		t.Fatalf("integer fields were not truncated like WakaTime: %#v", first)
	}
}

func TestDecodeExtraHeartbeatsRejectsInvalidWakaTimeCategoryAndEntityType(t *testing.T) {
	if _, err := decodeExtraHeartbeats(strings.NewReader(`[{"entity":"main.go","time":1,"category":"bad"}]`)); err == nil || !strings.Contains(err.Error(), `invalid category "bad"`) {
		t.Fatalf("expected invalid category error, got %v", err)
	}
	if _, err := decodeExtraHeartbeats(strings.NewReader(`[{"entity":"main.go","time":1,"entity_type":"bad"}]`)); err == nil || !strings.Contains(err.Error(), `invalid entity type "bad"`) {
		t.Fatalf("expected invalid entity type error, got %v", err)
	}
}

func TestProcessExtraHeartbeatUsesWakaTimeInternalFallbackFields(t *testing.T) {
	dir := t.TempDir()
	localFile := filepath.Join(dir, "local.py")
	if err := os.WriteFile(localFile, []byte("import requests\nprint('ok')\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hb, skip, err := processExtraHeartbeat(Heartbeat{
		Entity:            "ssh://example.test/tmp/remote",
		EntityType:        "file",
		LocalFile:         localFile,
		AlternateBranch:   "fallback-branch",
		AlternateLanguage: "FallbackLang",
		AlternateProject:  "fallback-project",
		Time:              123,
	}, Options{Category: "coding", Include: []string{`local\.py$`}})
	if err != nil {
		t.Fatal(err)
	}
	if skip {
		t.Fatal("heartbeat was unexpectedly skipped")
	}
	if hb.Entity != "ssh://example.test/tmp/remote" {
		t.Fatalf("entity = %q", hb.Entity)
	}
	if hb.Project != "fallback-project" || hb.Branch != "fallback-branch" || hb.Language != "Python" {
		t.Fatalf("fallback/enriched fields = %#v", hb)
	}
	if hb.Lines == nil || *hb.Lines != 2 || len(hb.Dependencies) == 0 {
		t.Fatalf("local-file stats were not applied: %#v", hb)
	}
}

func TestProcessExtraHeartbeatRemoteEntitySkipsProjectFileFilterLikeWakaTime(t *testing.T) {
	original := remoteFileDownload
	remoteFileDownload = func(_ remoteFileClient, localPath string) error {
		return os.WriteFile(localPath, []byte("package main\n"), 0o600)
	}
	t.Cleanup(func() { remoteFileDownload = original })
	hb, skip, err := processExtraHeartbeat(Heartbeat{
		Entity:     "ssh://example.test/home/me/main.go",
		EntityType: "file",
		Time:       123,
	}, Options{IncludeOnlyProjectFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if skip {
		t.Fatal("remote extra heartbeat was unexpectedly filtered")
	}
	if hb.Entity != "ssh://example.test/home/me/main.go" {
		t.Fatalf("entity = %q", hb.Entity)
	}
}

func TestProcessExtraHeartbeatDownloadsRemoteEntityWithoutLocalFile(t *testing.T) {
	original := remoteFileDownload
	var downloadedPath string
	remoteFileDownload = func(client remoteFileClient, localPath string) error {
		if client.Host != "example.test" || client.Path != "/tmp/remote.py" {
			t.Fatalf("unexpected remote client: %#v", client)
		}
		downloadedPath = localPath
		return os.WriteFile(localPath, []byte("import requests\nprint('ok')\n"), 0o600)
	}
	t.Cleanup(func() { remoteFileDownload = original })

	hb, skip, err := processExtraHeartbeat(Heartbeat{
		Entity:     "sftp://user:secret@example.test/tmp/remote.py",
		EntityType: "file",
		Time:       123,
	}, Options{Category: "coding"})
	if err != nil {
		t.Fatal(err)
	}
	if skip {
		t.Fatal("remote extra heartbeat was unexpectedly skipped")
	}
	if hb.Entity != "sftp://example.test/tmp/remote.py" {
		t.Fatalf("entity = %q", hb.Entity)
	}
	if hb.Language != "Python" || hb.Lines == nil || *hb.Lines != 2 || len(hb.Dependencies) == 0 {
		t.Fatalf("remote stats were not applied: %#v", hb)
	}
	if downloadedPath == "" {
		t.Fatal("expected remote downloader to run")
	}
	if _, err := os.Stat(downloadedPath); !os.IsNotExist(err) {
		t.Fatalf("remote temp file was not cleaned up: %s err=%v", downloadedPath, err)
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

func TestOfflineQueueDeleteDuplicates(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	heartbeats := []Heartbeat{
		{Entity: "/tmp/a.go", EntityType: "file", Time: 10},
		{Entity: "/tmp/a.go", EntityType: "file", Time: 10.5},
		{Entity: "/tmp/a.go", EntityType: "file", Time: 12},
		{Entity: "/tmp/b.go", EntityType: "file", Time: 10.5},
	}
	if err := AppendQueue(queue, heartbeats); err != nil {
		t.Fatal(err)
	}
	deleted, err := DeleteQueueDuplicates(queue)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d", deleted)
	}
	got, err := ReadQueue(queue, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("queue length = %d: %#v", len(got), got)
	}
	for _, hb := range got {
		if hb.Entity == "/tmp/a.go" && hb.Time == 10.5 {
			t.Fatalf("duplicate heartbeat was not removed: %#v", got)
		}
	}
}

func TestOfflineSyncDeduplicatesBeforePosting(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	if err := AppendQueue(queue, []Heartbeat{
		{Entity: "/tmp/a.go", EntityType: "file", Time: 10},
		{Entity: "/tmp/a.go", EntityType: "file", Time: 10.5},
		{Entity: "/tmp/a.go", EntityType: "file", Time: 12},
	}); err != nil {
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
	if err := Run([]string{"offline", "sync", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--offline-queue-file", queue}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 2 {
		t.Fatalf("posted = %#v", posted)
	}
	if posted[0].Time != 10 || posted[1].Time != 12 {
		t.Fatalf("unexpected posted heartbeat times: %#v", posted)
	}
}

func TestOfflineSyncRequeuesFailedAndMissingResults(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	heartbeats := []Heartbeat{
		{Entity: "/tmp/ok.go", EntityType: "file", Time: 1},
		{Entity: "/tmp/retry.go", EntityType: "file", Time: 2},
		{Entity: "/tmp/missing.go", EntityType: "file", Time: 3},
	}
	if err := AppendQueue(queue, heartbeats); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		response := map[string]any{"responses": []any{
			[]any{map[string]any{"data": posted[0]}, http.StatusCreated},
			[]any{map[string]any{"data": posted[1]}, http.StatusInternalServerError},
		}}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()
	if err := Run([]string{"offline", "sync", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--offline-queue-file", queue}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadQueue(queue, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Entity != "/tmp/retry.go" || got[1].Entity != "/tmp/missing.go" {
		t.Fatalf("unexpected requeued heartbeats: %#v", got)
	}
}

func TestOfflineSyncRequeuesMissingResultByHeartbeatAssociationLikeWakaTime(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	heartbeats := []Heartbeat{
		{Entity: "/tmp/a.go", EntityType: "file", Time: 1},
		{Entity: "/tmp/b.go", EntityType: "file", Time: 2},
		{Entity: "/tmp/c.go", EntityType: "file", Time: 3},
	}
	if err := AppendQueue(queue, heartbeats); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		response := map[string]any{"responses": []any{
			[]any{map[string]any{"data": posted[0]}, http.StatusCreated},
			[]any{map[string]any{"data": posted[2]}, http.StatusCreated},
		}}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()
	if err := Run([]string{"offline", "sync", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--offline-queue-file", queue}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadQueue(queue, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID() != heartbeats[1].ID() {
		t.Fatalf("unexpected requeued heartbeats: %#v", got)
	}
}

func TestOfflineSyncDropsBadRequestResults(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	hb := Heartbeat{Entity: "/tmp/bad.go", EntityType: "file", Time: 1}
	if err := AppendQueue(queue, []Heartbeat{hb}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		response := map[string]any{"responses": []any{[]any{map[string]any{"data": posted[0]}, http.StatusBadRequest}}}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()
	if err := Run([]string{"offline", "sync", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--offline-queue-file", queue}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	count, err := CountQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("bad request heartbeat should be dropped, count=%d", count)
	}
}

func TestOfflineSyncZeroLimitSyncsAllQueuedHeartbeats(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	heartbeats := []Heartbeat{
		{Entity: "/tmp/a.go", EntityType: "file", Time: 1},
		{Entity: "/tmp/b.go", EntityType: "file", Time: 2},
		{Entity: "/tmp/c.go", EntityType: "file", Time: 3},
	}
	if err := AppendQueue(queue, heartbeats); err != nil {
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
	if err := Run([]string{"offline", "sync", "--sync-offline-activity", "0", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--offline-queue-file", queue}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != len(heartbeats) {
		t.Fatalf("posted = %d", len(posted))
	}
}

func TestOfflineSyncPostsWakaTimeSizedChunks(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	heartbeats := make([]Heartbeat, 0, 25)
	for i := 0; i < 25; i++ {
		heartbeats = append(heartbeats, Heartbeat{
			Entity:     fmt.Sprintf("/tmp/offline-%02d.go", i),
			EntityType: "file",
			Time:       float64(i + 1),
		})
	}
	if err := AppendQueue(queue, heartbeats); err != nil {
		t.Fatal(err)
	}
	var batchSizes []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var posted []Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		batchSizes = append(batchSizes, len(posted))
		_, _ = w.Write(bulkResponseFor(posted, http.StatusCreated))
	}))
	defer server.Close()
	var out bytes.Buffer
	if err := Run([]string{"offline", "sync", "--sync-offline-activity", "25", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--offline-queue-file", queue}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "synced=25" {
		t.Fatalf("output = %q", out.String())
	}
	if got := fmt.Sprint(batchSizes); got != "[10 10 5]" {
		t.Fatalf("batch sizes = %s", got)
	}
	count, err := CountQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("count after sync = %d", count)
	}
}

func TestOfflineSyncNegativeLimitSyncsAllQueuedHeartbeatsLikeWakaTime(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	heartbeats := []Heartbeat{
		{Entity: "/tmp/a.go", EntityType: "file", Time: 1},
		{Entity: "/tmp/b.go", EntityType: "file", Time: 2},
		{Entity: "/tmp/c.go", EntityType: "file", Time: 3},
	}
	if err := AppendQueue(queue, heartbeats); err != nil {
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
	if err := Run([]string{"offline", "sync", "--sync-offline-activity", "-1", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--offline-queue-file", queue}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != len(heartbeats) {
		t.Fatalf("posted = %d", len(posted))
	}
}

func TestOfflineSyncMigratesConfiguredLegacyBoltQueueLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	queue := filepath.Join(dir, "offline_heartbeats.bdb")
	legacyQueue := filepath.Join(dir, ".wakatime.bdb")
	heartbeats := []Heartbeat{{Entity: "/tmp/legacy.go", EntityType: "file", Time: 1}}
	if err := AppendQueue(legacyQueue, heartbeats); err != nil {
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
		"offline", "sync",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", queue,
		"--offline-queue-file-legacy", legacyQueue,
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 1 || posted[0].Entity != "/tmp/legacy.go" {
		t.Fatalf("posted = %#v", posted)
	}
	if _, err := os.Stat(legacyQueue); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy queue should be removed after sync, stat err=%v", err)
	}
}

func TestOfflineSyncEmptyLegacyQueueFlagFallsBackToDefaultLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WAKATIME_HOME", dir)
	queue := filepath.Join(dir, "offline_heartbeats.bdb")
	legacyQueue := filepath.Join(dir, ".wakatime.bdb")
	heartbeats := []Heartbeat{{Entity: "/tmp/default-legacy.go", EntityType: "file", Time: 1}}
	if err := AppendQueue(legacyQueue, heartbeats); err != nil {
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
		"offline", "sync",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", queue,
		"--offline-queue-file-legacy=",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 1 || posted[0].Entity != "/tmp/default-legacy.go" {
		t.Fatalf("posted = %#v", posted)
	}
	if _, err := os.Stat(legacyQueue); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy queue should be removed after sync, stat err=%v", err)
	}
}

func TestOfflineQueueUsesWakaTimeBoltBucketAndKeys(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	hb := Heartbeat{Entity: "/tmp/a.go", EntityType: "file", Category: "coding", Project: "stint", Branch: "main", IsWrite: true, Time: 1}
	if err := AppendQueue(queue, []Heartbeat{hb}); err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(queue, 0o600, &bolt.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("heartbeats"))
		if b == nil {
			t.Fatalf("missing heartbeats bucket")
		}
		var got Heartbeat
		if err := json.Unmarshal(b.Get([]byte(hb.ID())), &got); err != nil {
			t.Fatal(err)
		}
		if got.Entity != hb.Entity || got.Project != hb.Project {
			t.Fatalf("unexpected queued heartbeat: %#v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestOfflineQueueResetsCorruptBoltDBLikeWakaTime(t *testing.T) {
	dir := t.TempDir()
	queue := filepath.Join(dir, "offline.bdb")
	if err := os.WriteFile(queue, []byte("not a bolt database"), 0o600); err != nil {
		t.Fatal(err)
	}
	count, err := CountQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("count = %d", count)
	}
	backups, err := filepath.Glob(queue + ".corrupt.*")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("corrupt backups = %v", backups)
	}
	hb := Heartbeat{Entity: "/tmp/recovered.go", EntityType: "file", Time: 1}
	if err := AppendQueue(queue, []Heartbeat{hb}); err != nil {
		t.Fatal(err)
	}
	read, err := ReadQueue(queue, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(read) != 1 || read[0].Entity != hb.Entity {
		t.Fatalf("read = %#v", read)
	}
}

func TestOfflineQueueLegacyJSONLStillWorks(t *testing.T) {
	queue := filepath.Join(t.TempDir(), "offline.jsonl")
	heartbeats := []Heartbeat{{Entity: "/tmp/a.go", EntityType: "file", Time: 1}}
	if err := AppendQueue(queue, heartbeats); err != nil {
		t.Fatal(err)
	}
	count, err := CountQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count = %d", count)
	}
	read, err := ReadQueue(queue, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(read) != 1 || read[0].Entity != "/tmp/a.go" {
		t.Fatalf("read = %#v", read)
	}
	if err := RemoveQueuePrefix(queue, 1); err != nil {
		t.Fatal(err)
	}
	count, err = CountQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("count after remove = %d", count)
	}
}

func bulkResponseFor(heartbeats []Heartbeat, status int) []byte {
	responses := make([]any, 0, len(heartbeats))
	for _, hb := range heartbeats {
		responses = append(responses, []any{map[string]any{"data": hb}, status})
	}
	body, _ := json.Marshal(map[string]any{"responses": responses})
	return body
}

func TestCollectDelegatesToStintCollectOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script PATH fixture is unix-only")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "stint-collect")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho collect:$*\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)
	var out bytes.Buffer
	if err := Run([]string{"collect", "--dry-run", "--agent", "codex"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "collect:--dry-run --agent codex" {
		t.Fatalf("unexpected collect output: %q", out.String())
	}
}

func TestCollectDelegatesToStintCollectNextToExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script executable fixture is unix-only")
	}
	dir := t.TempDir()
	stintPath := filepath.Join(dir, "stint")
	if err := os.WriteFile(stintPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	helperPath := filepath.Join(dir, "stint-collect")
	if err := os.WriteFile(helperPath, []byte("#!/bin/sh\necho sibling-collect:$*\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldExecutablePath := executablePath
	executablePath = func() (string, error) { return stintPath, nil }
	t.Cleanup(func() { executablePath = oldExecutablePath })
	t.Setenv("PATH", t.TempDir())

	var out bytes.Buffer
	if err := Run([]string{"collect", "--dry-run", "--agent", "codex"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "sibling-collect:--dry-run --agent codex" {
		t.Fatalf("unexpected collect output: %q", out.String())
	}
}

func TestSyncAIActivityPostsNativeTranscriptHeartbeats(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "sample")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(home, ".codex", "sessions", "2026", "06", "27")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-27T12:00:00Z","type":"session_meta","payload":{"id":"codex-session","cwd":"` + filepath.ToSlash(project) + `"}}`,
		`{"timestamp":"2026-06-27T12:01:00Z","payload":{"message":"Update the main file","filePath":"main.go","info":{"total_token_usage":{"input_tokens":12,"output_tokens":5}}}}`,
	}, "\n")
	transcriptPath := filepath.Join(transcriptDir, "codex-session.jsonl")
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	modTime := time.Date(2026, 6, 27, 12, 1, 0, 0, time.UTC)
	if err := os.Chtimes(transcriptPath, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	var posted []Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/users/current/heartbeats.bulk" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"responses":[]}`))
	}))
	defer server.Close()
	var out bytes.Buffer
	internalConfig := filepath.Join(t.TempDir(), "wakatime-internal.cfg")
	if err := Run([]string{
		"--sync-ai-activity",
		"--sync-ai-after", "1780000000",
		"--internal-config", internalConfig,
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
	}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 2 {
		t.Fatalf("posted = %#v", posted)
	}
	if posted[0].EntityType != "app" || posted[0].Entity != "Codex codex-session" || posted[0].Category != "ai coding" {
		t.Fatalf("unexpected app heartbeat: %#v", posted[0])
	}
	if posted[0].AIInputTokens == nil || *posted[0].AIInputTokens != 12 || posted[0].AIOutputTokens == nil || *posted[0].AIOutputTokens != 5 {
		t.Fatalf("unexpected token heartbeat: %#v", posted[0])
	}
	if posted[1].EntityType != "file" || posted[1].Entity != filepath.Join(project, "main.go") || !posted[1].IsWrite {
		t.Fatalf("unexpected file heartbeat: %#v", posted[1])
	}
	cfg, err := LoadConfig(internalConfig)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Get("internal", "ai_sync_after") == "" {
		t.Fatalf("expected ai_sync_after to be recorded")
	}
	if cfg.Get("internal", "ai_logs_last_parsed_at") == "" {
		t.Fatalf("expected ai_logs_last_parsed_at to be recorded")
	}
	if !strings.Contains(out.String(), "ai_heartbeats=2") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestRootSyncAIActivityTakesPrecedenceOverOfflineCountLikeWakaTime(t *testing.T) {
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
	transcriptDir := filepath.Join(home, ".codex", "sessions", "2026", "06", "27")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptPath := filepath.Join(transcriptDir, "codex-priority.jsonl")
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-27T12:00:00Z","type":"session_meta","payload":{"id":"codex-priority","cwd":"` + filepath.ToSlash(project) + `"}}`,
		`{"timestamp":"2026-06-27T12:01:00Z","payload":{"message":"Update the main file","filePath":"main.go"}}`,
	}, "\n")
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	modTime := time.Date(2026, 6, 27, 12, 1, 0, 0, time.UTC)
	if err := os.Chtimes(transcriptPath, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	queue := filepath.Join(t.TempDir(), "offline.bdb")
	if err := AppendQueue(queue, []Heartbeat{{Entity: "/tmp/offline.go", EntityType: "file", Time: 1}}); err != nil {
		t.Fatal(err)
	}

	var posted []Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"responses":[]}`))
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := Run([]string{
		"--sync-ai-activity",
		"--offline-count",
		"--sync-ai-after", "1780000000",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--offline-queue-file", queue,
		"--internal-config", filepath.Join(t.TempDir(), "wakatime-internal.cfg"),
	}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) == 0 {
		t.Fatalf("sync-ai-activity should take precedence over offline-count; output=%q", out.String())
	}
	if strings.TrimSpace(out.String()) == "1" {
		t.Fatalf("offline-count handled mixed command before sync-ai-activity")
	}
}

func TestSyncAIActivityFiltersExcludedFileHeartbeats(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "sample")
	if err := os.MkdirAll(project, 0o755); err != nil {
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
	transcriptPath := filepath.Join(transcriptDir, "codex-filter.jsonl")
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-27T12:00:00Z","session_id":"codex-filter","cwd":"` + filepath.ToSlash(project) + `"}`,
		`{"timestamp":"2026-06-27T12:01:00Z","message":"change main","filePath":"main.go"}`,
	}, "\n")
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	modTime := time.Date(2026, 6, 27, 12, 1, 0, 0, time.UTC)
	if err := os.Chtimes(transcriptPath, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		t.Fatalf("excluded AI activity should not be posted")
	}))
	defer server.Close()
	var out bytes.Buffer
	if err := Run([]string{
		"--sync-ai-activity",
		"--sync-ai-after", "1780000000",
		"--exclude", regexp.QuoteMeta(file),
		"--heartbeat-rate-limit-seconds", "0",
		"--internal-config", filepath.Join(t.TempDir(), "wakatime-internal.cfg"),
		"--offline-queue-file", filepath.Join(t.TempDir(), "offline.bdb"),
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
	}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if calls != 0 || strings.TrimSpace(out.String()) != "synced=0" {
		t.Fatalf("calls=%d output=%q", calls, out.String())
	}
}

func TestSyncAIActivityReadsWakaTimeAILogsLastParsedAt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "sample")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(home, ".codex", "sessions")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	after := time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Second)
	oldTime := after.Add(-30 * time.Minute)
	newTime := after.Add(30 * time.Minute)
	for _, item := range []struct {
		id string
		ts time.Time
	}{
		{id: "old", ts: oldTime},
		{id: "new", ts: newTime},
	} {
		transcript := strings.Join([]string{
			`{"timestamp":"` + item.ts.Format(time.RFC3339) + `","session_id":"codex-` + item.id + `","cwd":"` + filepath.ToSlash(project) + `"}`,
			`{"timestamp":"` + item.ts.Add(time.Minute).Format(time.RFC3339) + `","message":"change main","filePath":"main.go"}`,
		}, "\n")
		path := filepath.Join(transcriptDir, "codex-"+item.id+".jsonl")
		if err := os.WriteFile(path, []byte(transcript), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, item.ts, item.ts); err != nil {
			t.Fatal(err)
		}
	}
	internalConfig := filepath.Join(t.TempDir(), "wakatime-internal.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("internal", "ai_logs_last_parsed_at", after.Format(time.RFC3339))
	if err := cfg.Write(internalConfig); err != nil {
		t.Fatal(err)
	}
	var posted []Heartbeat
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"responses":[]}`))
	}))
	defer server.Close()

	if err := Run([]string{
		"--sync-ai-activity",
		"--internal-config", internalConfig,
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(posted) != 2 {
		t.Fatalf("posted = %#v", posted)
	}
	for _, hb := range posted {
		if hb.AISession != "codex-new" {
			t.Fatalf("old transcript was not filtered by ai_logs_last_parsed_at: %#v", posted)
		}
	}
}

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

func TestSyncAIActivityParsesClineTaskJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "cline-project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".git", "HEAD"), []byte("ref: refs/heads/ai-task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "app.ts"), []byte("export const ok = true;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	taskDir := filepath.Join(home, ".config", "Code", "User", "globalStorage", "saoudrizwan.claude-dev", "tasks", "task-123")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	millis := time.Date(2026, 6, 27, 13, 0, 0, 0, time.UTC).UnixMilli()
	messages := `[
		{"timestamp":` + fmt.Sprint(millis) + `,"sessionId":"task-123","cwd":"` + filepath.ToSlash(project) + `","text":"change app","input_tokens":20},
		{"timestamp":` + fmt.Sprint(millis+1000) + `,"filePath":"app.ts","output_tokens":7}
	]`
	path := filepath.Join(taskDir, "ui_messages.json")
	if err := os.WriteFile(path, []byte(messages), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, time.UnixMilli(millis+1000), time.UnixMilli(millis+1000)); err != nil {
		t.Fatal(err)
	}

	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var foundApp, foundFile bool
	for _, hb := range heartbeats {
		if hb.Entity == "Cline task-123" && hb.EntityType == "app" {
			foundApp = true
			if hb.AIInputTokens == nil || *hb.AIInputTokens != 20 || hb.AIOutputTokens == nil || *hb.AIOutputTokens != 7 {
				t.Fatalf("unexpected Cline app heartbeat: %#v", hb)
			}
		}
		if hb.Entity == filepath.Join(project, "app.ts") && hb.EntityType == "file" {
			foundFile = true
			if hb.Project != filepath.Base(project) {
				t.Fatalf("unexpected Cline file heartbeat: %#v", hb)
			}
		}
	}
	if !foundApp || !foundFile {
		t.Fatalf("missing Cline heartbeats: %#v", heartbeats)
	}
}

func TestSyncAIActivityMarksClineReadFileAsNonWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "cline-read-project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "notes.md")
	if err := os.WriteFile(file, []byte("# notes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	taskDir := filepath.Join(home, ".config", "Code", "User", "globalStorage", "saoudrizwan.claude-dev", "tasks", "cline-read-task")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	messageText := func(value map[string]any) string {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	millis := time.Date(2026, 6, 27, 15, 0, 0, 0, time.UTC).UnixMilli()
	messages := []map[string]any{
		{
			"ts":   millis,
			"type": "say",
			"say":  "api_req_started",
			"text": messageText(map[string]any{
				"request":   "<task>Inspect notes</task>\n# Current Working Directory (" + filepath.ToSlash(project) + ")",
				"tokensIn":  11,
				"tokensOut": 3,
			}),
		},
		{
			"ts":   millis + 1000,
			"type": "say",
			"say":  "tool",
			"text": messageText(map[string]any{
				"tool": "readFile",
				"path": "notes.md",
			}),
		},
	}
	data, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(taskDir, "ui_messages.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, time.UnixMilli(millis+1000), time.UnixMilli(millis+1000)); err != nil {
		t.Fatal(err)
	}

	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, hb := range heartbeats {
		if hb.Entity == file && hb.EntityType == "file" {
			if hb.IsWrite {
				t.Fatalf("expected Cline readFile heartbeat to be non-write, got %#v", hb)
			}
			if hb.AILineChanges != nil {
				t.Fatalf("expected Cline readFile heartbeat without ai_line_changes, got %#v", hb)
			}
			return
		}
	}
	t.Fatalf("missing Cline readFile heartbeat: %#v", heartbeats)
}

func TestSyncAIActivityParsesRooTaskJSONStrings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "roo-project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	taskDir := filepath.Join(home, ".config", "Code", "User", "globalStorage", "RooVeterinaryInc.roo-cline", "tasks", "roo-task")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	messageText := func(value map[string]any) string {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	millis := time.Date(2026, 6, 27, 14, 0, 0, 0, time.UTC).UnixMilli()
	messages := []map[string]any{
		{
			"ts":   millis,
			"type": "say",
			"say":  "api_req_started",
			"text": messageText(map[string]any{
				"request":   "<task>Refactor this function</task>\n# Current Working Directory (" + filepath.ToSlash(project) + ")",
				"tokensIn":  120,
				"tokensOut": 30,
			}),
		},
		{
			"ts":   millis + 1000,
			"type": "ask",
			"ask":  "tool",
			"text": messageText(map[string]any{
				"tool": "appliedDiff",
				"path": "main.go",
				"diff": "<<<<<<< SEARCH\nold\n=======\nnew\nextra\n>>>>>>> REPLACE",
			}),
		},
	}
	data, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(taskDir, "ui_messages.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, time.UnixMilli(millis+1000), time.UnixMilli(millis+1000)); err != nil {
		t.Fatal(err)
	}

	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var foundApp, foundFile bool
	for _, hb := range heartbeats {
		if hb.Entity == "Roo Code ui_messages" && hb.EntityType == "app" {
			foundApp = true
			if hb.AIPromptLength == nil || *hb.AIPromptLength != len("Refactor this function") {
				t.Fatalf("unexpected Roo app heartbeat: %#v", hb)
			}
			if hb.AIInputTokens == nil || *hb.AIInputTokens != 120 || hb.AIOutputTokens == nil || *hb.AIOutputTokens != 30 {
				t.Fatalf("unexpected Roo token metadata: %#v", hb)
			}
		}
		if hb.Entity == file && hb.EntityType == "file" {
			foundFile = true
			if hb.Project != filepath.Base(project) {
				t.Fatalf("unexpected Roo file heartbeat: %#v", hb)
			}
			if hb.AILineChanges == nil || *hb.AILineChanges != 1 {
				t.Fatalf("expected Roo file heartbeat to include ai_line_changes=1, got %#v", hb)
			}
		}
	}
	if !foundApp || !foundFile {
		t.Fatalf("missing Roo heartbeats: %#v", heartbeats)
	}
}

func TestAISourcesIncludeAntigravityProducts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got := aiSources()
	for _, want := range []struct {
		name string
		root string
	}{
		{name: "Antigravity Desktop", root: filepath.Join(home, ".gemini", "antigravity")},
		{name: "Antigravity IDE", root: filepath.Join(home, ".gemini", "antigravity-ide")},
		{name: "Antigravity CLI", root: filepath.Join(home, ".gemini", "antigravity-cli")},
	} {
		if !hasAISource(got, want.name, want.root) {
			t.Fatalf("missing AI source %s at %s in %#v", want.name, want.root, got)
		}
	}
}

func TestAISourcesIncludeCurrentWakaTimeTranscriptRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got := aiSources()
	for _, want := range []struct {
		name string
		root string
	}{
		{name: "Copilot", root: filepath.Join(home, "Library", "Application Support", "Code", "User", "workspaceStorage")},
		{name: "Copilot", root: filepath.Join(home, "AppData", "Roaming", "Code", "User", "workspaceStorage")},
		{name: "Copilot", root: filepath.Join(home, ".config", "Code", "User", "workspaceStorage")},
		{name: "Copilot", root: filepath.Join(home, ".config", "code", "User", "workspaceStorage")},
		{name: "Qoder", root: filepath.Join(home, ".qoder", "cache", "projects")},
		{name: "Roo Code", root: filepath.Join(home, "AppData", "Roaming", "Cursor", "User", "globalStorage", "RooVeterinaryInc.roo-cline", "tasks")},
	} {
		if !hasAISource(got, want.name, want.root) {
			t.Fatalf("missing AI transcript source %s at %s in %#v", want.name, want.root, got)
		}
	}
}

func TestAISQLiteSourcesIncludeCurrentWakaTimeDatabaseRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got := aiSQLiteSources()
	for _, want := range []aiSQLiteSource{
		{Name: "Windsurf", Kind: "cursor_disk_kv", Path: filepath.Join(home, "AppData", "Roaming", "Windsurf - Next", "User", "globalStorage", "state.vscdb")},
		{Name: "Windsurf", Kind: "cursor_disk_kv", Path: filepath.Join(home, ".config", "Windsurf - Next", "User", "globalStorage", "state.vscdb")},
		{Name: "Windsurf", Kind: "cursor_disk_kv", Path: filepath.Join(home, ".config", "windsurf-next", "User", "globalStorage", "state.vscdb")},
		{Name: "Cody", Kind: "cody_item_table", Path: filepath.Join(home, "Library", "Application Support", "Code - Insiders", "User", "globalStorage", "state.vscdb")},
		{Name: "Cody", Kind: "cody_item_table", Path: filepath.Join(home, "Library", "Application Support", "VSCodium", "User", "globalStorage", "state.vscdb")},
		{Name: "Cody", Kind: "cody_item_table", Path: filepath.Join(home, "AppData", "Roaming", "Code - Insiders", "User", "globalStorage", "state.vscdb")},
		{Name: "Cody", Kind: "cody_item_table", Path: filepath.Join(home, "AppData", "Roaming", "VSCodium", "User", "globalStorage", "state.vscdb")},
	} {
		if !hasAISQLiteSource(got, want) {
			t.Fatalf("missing AI sqlite source %#v in %#v", want, got)
		}
	}
}

func TestSyncAIActivityParsesAntigravityTranscriptJSONL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	project := filepath.Join(home, "src", "antigravity-project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "README.md")
	if err := os.WriteFile(file, []byte("# Antigravity\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sessionID := "f7da61a0-c935-43b4-9425-eb08ba98231d"
	logsDir := filepath.Join(home, ".gemini", "antigravity-cli", "brain", sessionID, ".system_generated", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcript := strings.Join([]string{
		`{"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-06-27T17:00:00Z","content":"<USER_REQUEST>\nupdate README.md\n</USER_REQUEST>"}`,
		`{"source":"MODEL","type":"CODE_ACTION","status":"DONE","created_at":"2026-06-27T17:00:02Z","content":"The following changes were made by the replace_file_content tool to: ` + filepath.ToSlash(file) + `. If relevant, proactively run tests.\n[diff_block_start]\n@@ -1 +1,2 @@\n # Antigravity\n+Updated by agent\n[diff_block_end]"}`,
	}, "\n")
	transcriptPath := filepath.Join(logsDir, "transcript.jsonl")
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	mtime := time.Date(2026, 6, 27, 17, 0, 2, 0, time.UTC)
	if err := os.Chtimes(transcriptPath, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var foundApp, foundFile bool
	for _, hb := range heartbeats {
		if hb.Entity == "Antigravity CLI "+sessionID && hb.EntityType == "app" {
			foundApp = true
			if hb.AISession != sessionID || hb.AIPromptLength == nil || *hb.AIPromptLength == 0 {
				t.Fatalf("unexpected Antigravity app heartbeat: %#v", hb)
			}
		}
		if hb.Entity == file && hb.EntityType == "file" {
			foundFile = true
			if hb.Project != filepath.Base(project) || hb.Language != "Markdown" {
				t.Fatalf("unexpected Antigravity file heartbeat: %#v", hb)
			}
		}
	}
	if !foundApp || !foundFile {
		t.Fatalf("missing Antigravity heartbeats: %#v", heartbeats)
	}
}

func TestSyncAIActivityParsesCopilotWorkspaceSessionFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	project := filepath.Join(home, "copilot project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	workspaceDir := filepath.Join(home, "Library", "Application Support", "Code", "User", "workspaceStorage", "workspace-1", "chatSessions")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(workspaceDir, "session-1.json")
	session := map[string]any{
		"lastMessageDate": int64(1770000100000),
		"sessionId":       "session-1",
		"requests": []any{
			map[string]any{
				"timestamp": int64(1770000001000),
				"message": map[string]any{
					"text": "Update the file and explain what changed",
				},
				"result": map[string]any{
					"metadata": map[string]any{
						"promptTokens": 9,
						"outputTokens": 4,
					},
				},
				"variableData": map[string]any{
					"variables": []any{
						map[string]any{
							"kind": "file",
							"id":   "file://" + strings.ReplaceAll(filepath.ToSlash(file), " ", "%20"),
							"value": map[string]any{
								"fsPath": file,
							},
						},
					},
				},
			},
		},
	}
	data, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessionPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1769999990,
	})
	if err != nil {
		t.Fatal(err)
	}

	var foundApp, foundFile bool
	for _, hb := range heartbeats {
		if hb.Entity == "Copilot session-1" && hb.EntityType == "app" {
			foundApp = true
			if hb.AIPromptLength == nil || *hb.AIPromptLength != len("Update the file and explain what changed") {
				t.Fatalf("unexpected Copilot app heartbeat: %#v", hb)
			}
			if hb.AIInputTokens == nil || *hb.AIInputTokens != 9 || hb.AIOutputTokens == nil || *hb.AIOutputTokens != 4 {
				t.Fatalf("unexpected Copilot token metadata: %#v", hb)
			}
		}
		if hb.Entity == file && hb.EntityType == "file" {
			foundFile = true
			if hb.Project != filepath.Base(project) {
				t.Fatalf("unexpected Copilot file heartbeat: %#v", hb)
			}
		}
	}
	if !foundApp || !foundFile {
		t.Fatalf("missing Copilot workspace heartbeats: %#v", heartbeats)
	}
}

func TestSyncAIActivityParsesCodexSuccessfulApplyPatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	project := filepath.Join(home, "codex-project")
	successful := filepath.Join(project, "successful.go")
	failed := filepath.Join(project, "failed.go")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(successful, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(failed, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(home, ".codex", "sessions", "2026", "06", "20")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptPath := filepath.Join(transcriptDir, "session.jsonl")
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-20T12:00:00Z","type":"session_meta","payload":{"id":"codex-session","cwd":"` + filepath.ToSlash(project) + `"}}`,
		`{"timestamp":"2026-06-20T12:00:01Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"failed-direct","name":"apply_patch","input":"*** Update File: failed.go\n+one"}}`,
		`{"timestamp":"2026-06-20T12:00:02Z","type":"event_msg","payload":{"type":"patch_apply_end","call_id":"failed-direct","success":false,"status":"failed"}}`,
		`{"timestamp":"2026-06-20T12:00:03Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"successful-direct","name":"apply_patch","input":"*** Update File: successful.go\n+one"}}`,
		`{"timestamp":"2026-06-20T12:00:04Z","type":"event_msg","payload":{"type":"patch_apply_end","call_id":"successful-direct","success":true,"status":"completed"}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(transcriptPath, time.Date(2026, 6, 20, 12, 0, 4, 0, time.UTC), time.Date(2026, 6, 20, 12, 0, 4, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, hb := range heartbeats {
		if hb.Entity == failed {
			t.Fatalf("failed Codex patch was emitted: %#v", heartbeats)
		}
		if hb.Entity == successful {
			found = true
			if !hb.IsWrite || hb.AILineChanges == nil || *hb.AILineChanges != 1 {
				t.Fatalf("unexpected Codex patch heartbeat: %#v", hb)
			}
		}
	}
	if !found {
		t.Fatalf("missing successful Codex patch heartbeat: %#v", heartbeats)
	}
}

func TestSyncAIActivityParsesCodexSuccessfulShellApplyPatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	project := filepath.Join(home, "codex-shell-project")
	successful := filepath.Join(project, "via-shell.go")
	failed := filepath.Join(project, "failed-shell.go")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(successful, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(failed, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(home, ".codex", "sessions", "2026", "06", "23")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	successCommand := "apply_patch <<'PATCH'\n*** Begin Patch\n*** Update File: via-shell.go\n+one\n+two\n*** End Patch\nPATCH"
	failedCommand := "apply_patch <<'PATCH'\n*** Begin Patch\n*** Update File: failed-shell.go\n+bad\n*** End Patch\nPATCH"
	transcriptPath := filepath.Join(transcriptDir, "session.jsonl")
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-23T12:00:00Z","type":"session_meta","payload":{"id":"codex-shell-session","cwd":"` + filepath.ToSlash(project) + `"}}`,
		`{"timestamp":"2026-06-23T12:00:01Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"failed-shell","name":"exec_command","input":` + strconv.Quote(`{"cmd":`+strconv.Quote(failedCommand)+`}`) + `}}`,
		`{"timestamp":"2026-06-23T12:00:02Z","type":"event_msg","payload":{"type":"exec_command_end","call_id":"failed-shell","exit_code":1}}`,
		`{"timestamp":"2026-06-23T12:00:03Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"successful-shell","name":"exec_command","input":` + strconv.Quote(`{"cmd":`+strconv.Quote(successCommand)+`}`) + `}}`,
		`{"timestamp":"2026-06-23T12:00:04Z","type":"event_msg","payload":{"type":"exec_command_end","call_id":"successful-shell","exit_code":0}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(transcriptPath, time.Date(2026, 6, 23, 12, 0, 4, 0, time.UTC), time.Date(2026, 6, 23, 12, 0, 4, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, hb := range heartbeats {
		if hb.Entity == failed {
			t.Fatalf("failed Codex shell patch was emitted: %#v", heartbeats)
		}
		if hb.Entity == successful {
			found = true
			if !hb.IsWrite || hb.AILineChanges == nil || *hb.AILineChanges != 2 {
				t.Fatalf("unexpected Codex shell patch heartbeat: %#v", hb)
			}
		}
	}
	if !found {
		t.Fatalf("missing successful Codex shell patch heartbeat: %#v", heartbeats)
	}
}

func TestSyncAIActivityStripsCodexPromptWrappers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	project := filepath.Join(home, "codex-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(home, ".codex", "sessions", "2026", "06", "21")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	userPrompt := "Add cache warming for the dashboard stats query"
	wrappedPrompt := "# Context from my IDE setup:\n\n## Active file: internal/api/stats.go\n\n## My request for Codex:\n" + userPrompt
	transcriptPath := filepath.Join(transcriptDir, "session.jsonl")
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-21T12:00:00Z","type":"session_meta","payload":{"id":"codex-wrapper-session","cwd":"` + filepath.ToSlash(project) + `"}}`,
		`{"timestamp":"2026-06-21T12:00:01Z","type":"event_msg","payload":{"message":` + strconv.Quote(wrappedPrompt) + `,"info":{"total_token_usage":{"input_tokens":20,"output_tokens":7}}}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(transcriptPath, time.Date(2026, 6, 21, 12, 0, 1, 0, time.UTC), time.Date(2026, 6, 21, 12, 0, 1, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(heartbeats) != 1 {
		t.Fatalf("heartbeats = %#v", heartbeats)
	}
	if heartbeats[0].AIPromptLength == nil || *heartbeats[0].AIPromptLength != len(userPrompt) {
		t.Fatalf("prompt length = %#v, want %d; heartbeat=%#v", heartbeats[0].AIPromptLength, len(userPrompt), heartbeats[0])
	}
}

func TestSyncAIActivityStripsClaudeSystemReminderFromPrompt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	project := filepath.Join(home, "claude-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(home, ".claude", "projects", "claude-project")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	userPrompt := "Please update the README with setup notes"
	wrappedPrompt := "<system-reminder>\nUse the TodoWrite tool for substantial tasks.\n</system-reminder>\n" + userPrompt
	transcriptPath := filepath.Join(transcriptDir, "session.jsonl")
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-22T12:00:00Z","sessionId":"claude-wrapper-session","cwd":"` + filepath.ToSlash(project) + `","message":{"role":"user","content":[{"type":"text","text":` + strconv.Quote(wrappedPrompt) + `}]}}`,
		`{"timestamp":"2026-06-22T12:00:01Z","sessionId":"claude-wrapper-session","message":{"role":"assistant","model":"claude-sonnet-4.5","usage":{"input_tokens":30,"output_tokens":11}}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(transcriptPath, time.Date(2026, 6, 22, 12, 0, 1, 0, time.UTC), time.Date(2026, 6, 22, 12, 0, 1, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(heartbeats) != 1 {
		t.Fatalf("heartbeats = %#v", heartbeats)
	}
	if heartbeats[0].AIPromptLength == nil || *heartbeats[0].AIPromptLength != len(userPrompt) {
		t.Fatalf("prompt length = %#v, want %d; heartbeat=%#v", heartbeats[0].AIPromptLength, len(userPrompt), heartbeats[0])
	}
	if heartbeats[0].AIModel != "claude-sonnet-4.5" || heartbeats[0].AIInputTokens == nil || *heartbeats[0].AIInputTokens != 30 || heartbeats[0].AIOutputTokens == nil || *heartbeats[0].AIOutputTokens != 11 {
		t.Fatalf("unexpected Claude metadata: %#v", heartbeats[0])
	}
}

func TestSyncAIActivityParsesClaudeSubscriptionPlan(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	project := filepath.Join(home, "claude-plan-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(home, ".claude", "projects", "claude-plan-project")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptPath := filepath.Join(transcriptDir, "session.jsonl")
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-24T12:00:00Z","sessionId":"claude-plan-session","cwd":"` + filepath.ToSlash(project) + `","message":{"role":"user","content":"Review the billing code"},"config":{"subscriptionPlan":"max"}}`,
		`{"timestamp":"2026-06-24T12:00:01Z","sessionId":"claude-plan-session","message":{"role":"assistant","model":"claude-sonnet-4.5","usage":{"input_tokens":9,"output_tokens":4}}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(transcriptPath, time.Date(2026, 6, 24, 12, 0, 1, 0, time.UTC), time.Date(2026, 6, 24, 12, 0, 1, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(heartbeats) != 1 {
		t.Fatalf("heartbeats = %#v", heartbeats)
	}
	if heartbeats[0].AISubscriptionPlan != "max" {
		t.Fatalf("subscription plan = %q; heartbeat=%#v", heartbeats[0].AISubscriptionPlan, heartbeats[0])
	}
}

func TestSyncAIActivityParsesCopilotCLISessionEvents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	project := filepath.Join(home, "copilot-cli-project")
	readme := filepath.Join(project, "README.md")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readme, []byte("# Project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sessionID := "cli-session-1"
	eventsPath := filepath.Join(home, ".copilot", "session-state", sessionID, "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(eventsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestJSONLines(t, eventsPath, []map[string]any{
		{
			"type":      "session.start",
			"timestamp": "2026-06-14T12:00:00Z",
			"data": map[string]any{
				"sessionId":      sessionID,
				"copilotVersion": "1.0.62",
				"context": map[string]any{
					"cwd":     project,
					"gitRoot": project,
				},
			},
		},
		{
			"type":      "session.model_change",
			"timestamp": "2026-06-14T12:00:00.500Z",
			"data": map[string]any{
				"newModel": "gpt-5.4",
			},
		},
		{
			"type":      "user.message",
			"timestamp": "2026-06-14T12:00:01Z",
			"data": map[string]any{
				"content": "Update README",
			},
		},
		{
			"type":      "assistant.message",
			"timestamp": "2026-06-14T12:00:02Z",
			"data": map[string]any{
				"outputTokens": 9,
			},
		},
		{
			"type":      "tool.execution_complete",
			"timestamp": "2026-06-14T12:00:03Z",
			"data": map[string]any{
				"success": true,
				"toolTelemetry": map[string]any{
					"restrictedProperties": map[string]any{
						"filePaths": testJSONString(t, []string{readme}),
					},
					"metrics": map[string]any{
						"linesAdded":   2,
						"linesRemoved": 1,
					},
				},
			},
		},
		{
			"type":      "session.shutdown",
			"timestamp": "2026-06-14T12:00:04Z",
			"data": map[string]any{
				"tokenDetails": map[string]any{
					"input": map[string]any{
						"tokenCount": 17,
					},
					"output": map[string]any{
						"tokenCount": 11,
					},
				},
			},
		},
	})
	modTime := time.Date(2026, 6, 14, 12, 4, 0, 0, time.UTC)
	if err := os.Chtimes(eventsPath, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var foundApp, foundFile bool
	for _, hb := range heartbeats {
		if hb.Entity == "Copilot "+sessionID && hb.EntityType == "app" {
			foundApp = true
			if hb.AISession != sessionID || hb.AIAgentVersion != "1.0.62" || hb.AIModel != "gpt-5.4" {
				t.Fatalf("missing Copilot CLI identity metadata: %#v", hb)
			}
			if hb.AIPromptLength == nil || *hb.AIPromptLength != len("Update README") {
				t.Fatalf("missing Copilot CLI prompt length: %#v", hb)
			}
			if hb.AIInputTokens == nil || *hb.AIInputTokens != 17 || hb.AIOutputTokens == nil || *hb.AIOutputTokens != 20 {
				t.Fatalf("unexpected Copilot CLI token metadata: %#v", hb)
			}
		}
		if hb.Entity == readme && hb.EntityType == "file" {
			foundFile = true
			if !hb.IsWrite || hb.AILineChanges == nil || *hb.AILineChanges != 1 {
				t.Fatalf("missing Copilot CLI file metadata: %#v", hb)
			}
		}
	}
	if !foundApp || !foundFile {
		t.Fatalf("missing Copilot CLI heartbeats: %#v", heartbeats)
	}
}

func TestSyncAIActivityParsesContinueDevDataWithWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	continueDir := filepath.Join(home, ".continue")
	devDataDir := filepath.Join(continueDir, "dev_data", "0.2.0")
	sessionsDir := filepath.Join(continueDir, "sessions")
	if err := os.MkdirAll(devDataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(home, "continue-project")
	readPath := filepath.Join(workspace, "README.md")
	editPath := filepath.Join(workspace, "pkg", "ai", "continue.go")
	if err := os.MkdirAll(filepath.Dir(editPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readPath, []byte("# Project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(editPath, []byte("package ai\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sessionID := "5832a36f-ea52-4eb4-b8f7-16ed2bf063dd"
	sessions := []map[string]any{{
		"sessionId":          sessionID,
		"workspaceDirectory": "file://" + filepath.ToSlash(workspace),
	}}
	if err := os.WriteFile(filepath.Join(sessionsDir, "sessions.json"), []byte(testJSONString(t, sessions)), 0o600); err != nil {
		t.Fatal(err)
	}
	writeTestJSONLines(t, filepath.Join(devDataDir, "tokensGenerated.jsonl"), []map[string]any{{
		"timestamp":       "2026-05-01T19:45:22.290Z",
		"eventName":       "tokensGenerated",
		"model":           "gpt-5.2",
		"provider":        "openai",
		"promptTokens":    151,
		"generatedTokens": 36,
	}})
	writeTestJSONLines(t, filepath.Join(devDataDir, "chatInteraction.jsonl"), []map[string]any{{
		"timestamp":     "2026-05-01T19:45:22.287Z",
		"eventName":     "chatInteraction",
		"prompt":        "<system>\nskip\n</system>\n<user>\nadd yourself to the readme in this project\n\n",
		"modelName":     "gpt-5.2",
		"modelProvider": "openai",
		"sessionId":     sessionID,
	}})
	writeTestJSONLines(t, filepath.Join(devDataDir, "toolUsage.jsonl"), []map[string]any{{
		"timestamp":    "2026-05-01T19:45:26.107Z",
		"eventName":    "toolUsage",
		"functionName": "read_file",
		"toolCallArgs": testJSONString(t, map[string]string{"filepath": "README.md"}),
		"accepted":     true,
		"succeeded":    true,
	}})
	writeTestJSONLines(t, filepath.Join(devDataDir, "editOutcome.jsonl"), []map[string]any{{
		"timestamp":     "2026-05-01T19:45:48.207Z",
		"eventName":     "editOutcome",
		"prompt":        "add yourself to the readme in this project",
		"modelName":     "gpt-5.2",
		"modelProvider": "openai",
		"accepted":      true,
		"lineChange":    2,
		"filepath":      "file://" + filepath.ToSlash(editPath),
	}})
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1777664722.281,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(heartbeats) != 3 {
		t.Fatalf("heartbeats = %#v", heartbeats)
	}
	app := heartbeats[0]
	if app.Entity != "Continue "+sessionID || app.EntityType != "app" || app.AISession != sessionID {
		t.Fatalf("unexpected Continue app heartbeat: %#v", app)
	}
	if app.Project != filepath.Base(workspace) || app.AIPromptLength == nil || *app.AIPromptLength != len("add yourself to the readme in this project") {
		t.Fatalf("missing Continue app project/prompt metadata: %#v", app)
	}
	if app.AIInputTokens == nil || *app.AIInputTokens != 151 || app.AIOutputTokens == nil || *app.AIOutputTokens != 36 {
		t.Fatalf("missing Continue token metadata: %#v", app)
	}
	read := heartbeats[1]
	if read.Entity != readPath || read.EntityType != "file" || read.IsWrite {
		t.Fatalf("unexpected Continue read heartbeat: %#v", read)
	}
	if read.AILineChanges == nil || *read.AILineChanges != 0 {
		t.Fatalf("missing Continue read line changes: %#v", read)
	}
	edit := heartbeats[2]
	if edit.Entity != editPath || edit.EntityType != "file" || !edit.IsWrite {
		t.Fatalf("unexpected Continue edit heartbeat: %#v", edit)
	}
	if edit.AILineChanges == nil || *edit.AILineChanges != 2 {
		t.Fatalf("missing Continue edit line changes: %#v", edit)
	}
}

func TestSyncAIActivityParsesAmpApplyPatchLogs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	project := filepath.Join(home, "amp-project")
	readme := filepath.Join(project, "README.md")
	testFile := filepath.Join(project, "pkg", "ai", "amp_test.go")
	if err := os.MkdirAll(filepath.Dir(testFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readme, []byte("old readme\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testFile, []byte("old test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	threadID := "T-019eea92-63e4-70dd-83d7-bfac30818089"
	transcriptPath := filepath.Join(home, ".cache", "amp", "logs", "threads", threadID+".log")
	if err := os.MkdirAll(filepath.Dir(transcriptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: README.md",
		"@@",
		"-Made by the team.",
		"+Made by the team and Amp.",
		"*** Update File: pkg/ai/amp_test.go",
		"@@",
		"-old test",
		"+new test",
		"+second new test",
		"*** End Patch",
	}, "\n")
	writeTestJSONLines(t, transcriptPath, []map[string]any{
		{
			"@timestamp": "2026-06-21T14:28:00.500Z",
			"type":       "agent_state",
			"threadId":   threadID,
		},
		{
			"@timestamp": "2026-06-21T14:28:07.006Z",
			"message":    "onToolLease",
			"threadId":   threadID,
			"data": map[string]any{
				"type":       "tool_lease",
				"toolCallId": "patch-1",
				"toolName":   "apply_patch",
				"args": map[string]any{
					"workdir":   project,
					"patchText": patch,
				},
			},
		},
		{
			"@timestamp": "2026-06-21T14:28:07.330Z",
			"type":       "executor_tool_result",
			"threadId":   threadID,
			"toolCallId": "patch-1",
			"runStatus":  "done",
		},
	})
	modTime := time.Date(2026, 6, 21, 14, 29, 0, 0, time.UTC)
	if err := os.Chtimes(transcriptPath, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1782048480,
	})
	if err != nil {
		t.Fatal(err)
	}
	var foundReadme, foundTest bool
	for _, hb := range heartbeats {
		switch hb.Entity {
		case readme:
			foundReadme = true
			if hb.EntityType != "file" || !hb.IsWrite || hb.AILineChanges == nil || *hb.AILineChanges != 0 {
				t.Fatalf("unexpected Amp README heartbeat: %#v", hb)
			}
		case testFile:
			foundTest = true
			if hb.EntityType != "file" || !hb.IsWrite || hb.AILineChanges == nil || *hb.AILineChanges != 1 {
				t.Fatalf("unexpected Amp test heartbeat: %#v", hb)
			}
		}
	}
	if !foundReadme || !foundTest {
		t.Fatalf("missing Amp patch file heartbeats: %#v", heartbeats)
	}
}

func TestSyncAIActivityParsesQwenCodeToolCalls(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("QWEN_HOME", "")
	t.Setenv("QWEN_RUNTIME_DIR", "")
	project := filepath.Join(home, "qwen-project")
	readme := filepath.Join(project, "README.md")
	notes := filepath.Join(project, "notes.txt")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readme, []byte("# Project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notes, []byte("alpha\nbeta\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sessionID := "426ee865-f7b9-4a3f-b9bc-5e0efd602bde"
	chatsDir := filepath.Join(home, ".qwen", "projects", "-project", "chats")
	if err := os.MkdirAll(chatsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	base := map[string]any{
		"cwd":       project,
		"sessionId": sessionID,
		"version":   "0.17.0",
	}
	record := func(fields map[string]any) map[string]any {
		merged := map[string]any{}
		for k, v := range base {
			merged[k] = v
		}
		for k, v := range fields {
			merged[k] = v
		}
		return merged
	}
	transcript := filepath.Join(chatsDir, sessionID+".jsonl")
	writeTestJSONLines(t, transcript, []map[string]any{
		record(map[string]any{
			"type":      "user",
			"timestamp": "2026-05-30T12:00:01Z",
			"message": map[string]any{
				"role":  "user",
				"parts": []map[string]any{{"text": "inspect the project and update the notes"}},
			},
		}),
		record(map[string]any{
			"type":      "assistant",
			"timestamp": "2026-05-30T12:00:02Z",
			"model":     "qwen3-coder-plus",
			"message": map[string]any{
				"role": "model",
				"parts": []map[string]any{{
					"functionCall": map[string]any{
						"id":   "read-1",
						"name": "read_file",
						"args": map[string]any{"file_path": "README.md"},
					},
				}},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount":     100,
				"candidatesTokenCount": 20,
			},
		}),
		record(map[string]any{
			"type":      "tool_result",
			"timestamp": "2026-05-30T12:00:03Z",
			"toolCallResult": map[string]any{
				"callId": "read-1",
				"status": "success",
			},
		}),
		record(map[string]any{
			"type":      "assistant",
			"timestamp": "2026-05-30T12:00:04Z",
			"message": map[string]any{
				"role": "model",
				"parts": []map[string]any{{
					"functionCall": map[string]any{
						"id":   "write-1",
						"name": "write_file",
						"args": map[string]any{
							"file_path": "notes.txt",
							"content":   "alpha\nbeta\ngamma",
						},
					},
				}},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount":     50,
				"candidatesTokenCount": 10,
			},
		}),
		record(map[string]any{
			"type":      "tool_result",
			"timestamp": "2026-05-30T12:00:05Z",
			"toolCallResult": map[string]any{
				"callId": "write-1",
				"status": "success",
			},
		}),
	})
	modTime := time.Date(2026, 5, 30, 12, 0, 5, 0, time.UTC)
	if err := os.Chtimes(transcript, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780142400,
	})
	if err != nil {
		t.Fatal(err)
	}
	var foundApp, foundRead, foundWrite bool
	for _, hb := range heartbeats {
		switch hb.Entity {
		case "Qwen Code " + sessionID:
			foundApp = true
			if hb.AIModel != "qwen3-coder-plus" || hb.AIAgentVersion != "0.17.0" {
				t.Fatalf("unexpected Qwen app metadata: %#v", hb)
			}
			if hb.AIPromptLength == nil || *hb.AIPromptLength != len("inspect the project and update the notes") {
				t.Fatalf("unexpected Qwen prompt metadata: %#v", hb)
			}
			if hb.AIInputTokens == nil || *hb.AIInputTokens != 150 || hb.AIOutputTokens == nil || *hb.AIOutputTokens != 30 {
				t.Fatalf("unexpected Qwen token metadata: %#v", hb)
			}
		case readme:
			foundRead = true
			if hb.IsWrite || hb.AILineChanges == nil || *hb.AILineChanges != 0 {
				t.Fatalf("unexpected Qwen read heartbeat: %#v", hb)
			}
		case notes:
			foundWrite = true
			if !hb.IsWrite || hb.AILineChanges == nil || *hb.AILineChanges != 3 {
				t.Fatalf("unexpected Qwen write heartbeat: %#v", hb)
			}
		}
	}
	if !foundApp || !foundRead || !foundWrite {
		t.Fatalf("missing Qwen Code heartbeats: %#v", heartbeats)
	}
}

func TestSyncAIActivityParsesGeminiProjectRootToolCalls(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	project := filepath.Join(home, "gemini-project")
	mainFile := filepath.Join(project, "pkg", "ai", "gemini.go")
	notes := filepath.Join(project, "notes.txt")
	if err := os.MkdirAll(filepath.Dir(mainFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainFile, []byte("old line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notes, []byte("alpha\nbeta\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	projectSlug := filepath.Join(home, ".gemini", "tmp", "gemini-project")
	sessionDir := filepath.Join(projectSlug, "chats")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectSlug, ".project_root"), []byte(project), 0o600); err != nil {
		t.Fatal(err)
	}
	sessionID := "gem-session-1"
	session := map[string]any{
		"sessionId":   sessionID,
		"startTime":   "2026-04-21T12:00:00Z",
		"lastUpdated": "2026-04-21T12:00:05Z",
		"messages": []map[string]any{
			{
				"type":      "user",
				"timestamp": "2026-04-21T12:00:01Z",
				"content": []map[string]any{
					{"text": "look for bugs and fix any you find"},
				},
			},
			{
				"type":      "gemini",
				"timestamp": "2026-04-21T12:00:02Z",
				"model":     "gemini-3-flash-preview",
				"tokens": map[string]any{
					"input":  100,
					"output": 20,
				},
				"toolCalls": []map[string]any{{
					"name":   "replace",
					"status": "success",
					"args": map[string]any{
						"file_path":  "pkg/ai/gemini.go",
						"old_string": "old line",
						"new_string": "new line\nextra line",
					},
					"resultDisplay": map[string]any{
						"filePath": mainFile,
						"diffStat": map[string]any{
							"model_added_lines":   3,
							"model_removed_lines": 1,
						},
					},
				}},
			},
			{
				"type":      "gemini",
				"timestamp": "2026-04-21T12:00:03Z",
				"model":     "gemini-3-flash-preview",
				"toolCalls": []map[string]any{{
					"name":   "write_file",
					"status": "success",
					"args": map[string]any{
						"file_path": "notes.txt",
						"content":   "alpha\nbeta",
					},
				}},
			},
		},
	}
	sessionPath := filepath.Join(sessionDir, "session.json")
	if err := os.WriteFile(sessionPath, []byte(testJSONString(t, session)), 0o600); err != nil {
		t.Fatal(err)
	}
	modTime := time.Date(2026, 4, 21, 12, 0, 5, 0, time.UTC)
	if err := os.Chtimes(sessionPath, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1776772790,
	})
	if err != nil {
		t.Fatal(err)
	}
	var foundApp, foundReplace, foundWrite bool
	for _, hb := range heartbeats {
		switch hb.Entity {
		case "Gemini " + sessionID:
			foundApp = true
			if hb.Project != filepath.Base(project) || hb.AIPromptLength == nil || *hb.AIPromptLength != len("look for bugs and fix any you find") {
				t.Fatalf("unexpected Gemini app metadata: %#v", hb)
			}
			if hb.AIModel != "gemini-3-flash-preview" || hb.AIInputTokens == nil || *hb.AIInputTokens != 100 || hb.AIOutputTokens == nil || *hb.AIOutputTokens != 20 {
				t.Fatalf("unexpected Gemini token/model metadata: %#v", hb)
			}
		case mainFile:
			foundReplace = true
			if !hb.IsWrite || hb.AILineChanges == nil || *hb.AILineChanges != 2 {
				t.Fatalf("unexpected Gemini replace heartbeat: %#v", hb)
			}
		case notes:
			foundWrite = true
			if !hb.IsWrite || hb.AILineChanges == nil || *hb.AILineChanges != 2 {
				t.Fatalf("unexpected Gemini write heartbeat: %#v", hb)
			}
		}
	}
	if !foundApp || !foundReplace || !foundWrite {
		t.Fatalf("missing Gemini heartbeats: %#v", heartbeats)
	}
}

func TestSyncAIActivityParsesKiroWorkspaceActions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	project := filepath.Join(home, "kiro-project")
	authors := filepath.Join(project, "AUTHORS")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authors, []byte("Patches\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, ".config", "Kiro", "User", "globalStorage", "kiro.kiroagent")
	sessionID := "c2618220-b591-4431-8f14-fcd7ae3e6f56"
	executionID := "4490c49d-f38f-4d24-907e-d14daa4224cc"
	sessionDir := filepath.Join(root, "workspace-sessions", "workspace-hash")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	session := map[string]any{
		"sessionId":          sessionID,
		"workspacePath":      project,
		"workspaceDirectory": project,
		"history": []map[string]any{
			{
				"message": map[string]any{
					"role": "user",
					"content": []map[string]string{{
						"type": "text",
						"text": "Add your name to the AUTHORS file in this repo",
					}},
				},
			},
			{
				"executionId": executionID,
				"message": map[string]any{
					"role":    "assistant",
					"content": "On it.",
				},
			},
		},
	}
	if err := os.WriteFile(filepath.Join(sessionDir, sessionID+".json"), []byte(testJSONString(t, session)), 0o600); err != nil {
		t.Fatal(err)
	}
	executionDir := filepath.Join(root, "project-hash", "session-hash")
	if err := os.MkdirAll(executionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	execution := map[string]any{
		"executionId":   executionID,
		"chatSessionId": sessionID,
		"startTime":     int64(1777312055796),
		"actions": []map[string]any{
			{
				"actionType":  "readFiles",
				"actionState": "Accepted",
				"emittedAt":   int64(1777312061066),
				"input": map[string]any{
					"files": []map[string]string{{"path": "AUTHORS"}},
				},
			},
			{
				"actionType":  "replace",
				"actionState": "Accepted",
				"emittedAt":   int64(1777312065416),
				"input": map[string]any{
					"file":            "AUTHORS",
					"local":           "file://" + filepath.ToSlash(authors),
					"originalContent": "Patches\n",
					"modifiedContent": "Patches\n- Kiro <kiro@kiro.dev>\n",
				},
			},
		},
	}
	executionPath := filepath.Join(executionDir, "first")
	if err := os.WriteFile(executionPath, []byte(testJSONString(t, execution)), 0o600); err != nil {
		t.Fatal(err)
	}
	modTime := time.Date(2026, 4, 27, 12, 1, 5, 0, time.UTC)
	if err := os.Chtimes(executionPath, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1777312050,
	})
	if err != nil {
		t.Fatal(err)
	}
	var foundApp, foundRead, foundWrite bool
	for _, hb := range heartbeats {
		switch {
		case hb.Entity == "Kiro "+sessionID && hb.EntityType == "app":
			foundApp = true
			if hb.Project != filepath.Base(project) || hb.AIPromptLength == nil || *hb.AIPromptLength != len("Add your name to the AUTHORS file in this repo") {
				t.Fatalf("unexpected Kiro app heartbeat: %#v", hb)
			}
		case hb.Entity == authors && hb.EntityType == "file" && !hb.IsWrite:
			foundRead = true
			if hb.AILineChanges != nil {
				t.Fatalf("unexpected Kiro read line changes: %#v", hb)
			}
		case hb.Entity == authors && hb.EntityType == "file" && hb.IsWrite:
			foundWrite = true
			if hb.AILineChanges == nil || *hb.AILineChanges != 1 {
				t.Fatalf("unexpected Kiro write heartbeat: %#v", hb)
			}
		}
	}
	if !foundApp || !foundRead || !foundWrite {
		t.Fatalf("missing Kiro heartbeats: %#v", heartbeats)
	}
}

func writeTestJSONLines(t *testing.T, path string, values []map[string]any) {
	t.Helper()
	var lines []string
	for _, value := range values {
		lines = append(lines, testJSONString(t, value))
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
}

func testJSONString(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func hasAISource(sources []aiTranscriptSource, name, root string) bool {
	for _, source := range sources {
		if source.Name == name && filepath.Clean(source.Root) == filepath.Clean(root) {
			return true
		}
	}
	return false
}

func hasAISQLiteSource(sources []aiSQLiteSource, want aiSQLiteSource) bool {
	for _, source := range sources {
		if source.Name == want.Name && source.Kind == want.Kind && filepath.Clean(source.Path) == filepath.Clean(want.Path) {
			return true
		}
	}
	return false
}

func TestSyncAIActivityParsesPiJSONL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "pi-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sessionDir := filepath.Join(home, ".pi", "agent", "sessions", "pi-project")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcript := strings.Join([]string{
		`{"type":"session","id":"pi-session","cwd":"` + filepath.ToSlash(project) + `","timestamp":"2026-06-27T16:00:00Z"}`,
		`{"type":"message","timestamp":"2026-06-27T16:00:01Z","message":{"role":"user","content":[{"type":"text","text":"fix main.go"}]}}`,
		`{"type":"message","timestamp":"2026-06-27T16:00:02Z","message":{"role":"assistant","provider":"anthropic","model":"claude-opus-4-5","usage":{"input":10,"output":5,"cacheRead":2},"content":[{"type":"toolCall","id":"tool-edit","name":"edit","arguments":{"filePath":"main.go","newText":"package main\nfunc main() {}\n"}}]}}`,
	}, "\n")
	path := filepath.Join(sessionDir, "pi-session.jsonl")
	if err := os.WriteFile(path, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, time.Date(2026, 6, 27, 16, 0, 2, 0, time.UTC), time.Date(2026, 6, 27, 16, 0, 2, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var foundApp, foundFile bool
	for _, hb := range heartbeats {
		if hb.Entity == "Pi pi-session" && hb.EntityType == "app" {
			foundApp = true
			if hb.AISession != "pi-session" || hb.AIInputTokens == nil || *hb.AIInputTokens != 12 || hb.AIOutputTokens == nil || *hb.AIOutputTokens != 5 {
				t.Fatalf("unexpected Pi app heartbeat: %#v", hb)
			}
		}
		if hb.Entity == file && hb.EntityType == "file" {
			foundFile = true
			if hb.Project != filepath.Base(project) || hb.Language != "Go" {
				t.Fatalf("unexpected Pi file heartbeat: %#v", hb)
			}
		}
	}
	if !foundApp || !foundFile {
		t.Fatalf("missing Pi heartbeats: %#v", heartbeats)
	}
}

func TestSyncAIActivityParsesCursorSQLiteState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "cursor-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "main.py"), []byte("print('ok')\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, ".config", "Cursor", "User", "globalStorage", "state.vscdb")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE cursorDiskKV (key TEXT UNIQUE ON CONFLICT REPLACE, value BLOB)`); err != nil {
		t.Fatal(err)
	}
	value := `{"createdAt":"2026-06-27T14:00:00Z","cwd":"` + filepath.ToSlash(project) + `","text":"inspect file","filePath":"main.py","tokenCount":{"inputTokens":9,"outputTokens":4}}`
	if _, err := db.Exec(`INSERT INTO cursorDiskKV(key, value) VALUES(?, ?)`, "bubbleId:cursor-session:user", value); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dbPath, time.Date(2026, 6, 27, 14, 0, 0, 0, time.UTC), time.Date(2026, 6, 27, 14, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, hb := range heartbeats {
		if hb.Entity == "Cursor cursor-session" && hb.EntityType == "app" {
			found = true
			if hb.AIInputTokens == nil || *hb.AIInputTokens != 9 || hb.AIOutputTokens == nil || *hb.AIOutputTokens != 4 {
				t.Fatalf("unexpected Cursor heartbeat: %#v", hb)
			}
		}
	}
	if !found {
		t.Fatalf("missing Cursor heartbeat: %#v", heartbeats)
	}
}

func TestSyncAIActivityParsesQoderSQLiteState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "qoder-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, ".config", "Qoder", "SharedClientCache", "cache", "db", "local.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE chat_session (session_id TEXT PRIMARY KEY, project_uri TEXT);
CREATE TABLE chat_message (
	id TEXT PRIMARY KEY,
	session_id TEXT,
	request_id TEXT,
	role TEXT,
	content TEXT,
	tool_result TEXT,
	token_info TEXT,
	gmt_create INTEGER
)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chat_session(session_id, project_uri) VALUES(?, ?)`, "qoder-session", project); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chat_message(id, session_id, request_id, role, content, tool_result, token_info, gmt_create) VALUES
		('user-1', 'qoder-session', 'req-1', 'user', 'change main', '', '', 1782576300000),
		('assistant-1', 'qoder-session', 'req-1', 'assistant', '', '', '{"prompt_tokens":13,"completion_tokens":8}', 1782576301000),
		('tool-1', 'qoder-session', 'req-1', 'tool', '', '{"sessionId":"qoder-session","toolCallName":"search_replace","results":[{"path":"` + filepath.ToSlash(file) + `","diffInfo":{"add":2,"delete":1}}]}', '', 1782576302000)
	`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dbPath, time.Date(2026, 6, 27, 17, 0, 2, 0, time.UTC), time.Date(2026, 6, 27, 17, 0, 2, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var foundApp, foundFile bool
	for _, hb := range heartbeats {
		if hb.Entity == "Qoder qoder-session" && hb.EntityType == "app" {
			foundApp = true
			if hb.Project != filepath.Base(project) || hb.AIInputTokens == nil || *hb.AIInputTokens != 13 || hb.AIOutputTokens == nil || *hb.AIOutputTokens != 8 {
				t.Fatalf("unexpected Qoder app heartbeat: %#v", hb)
			}
		}
		if hb.Entity == file && hb.EntityType == "file" {
			foundFile = true
			if hb.Project != filepath.Base(project) || hb.Language != "Go" {
				t.Fatalf("unexpected Qoder file heartbeat: %#v", hb)
			}
		}
	}
	if !foundApp || !foundFile {
		t.Fatalf("missing Qoder heartbeats: %#v", heartbeats)
	}
}

func TestSyncAIActivityUsesQoderHistoryForPromptLength(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "qoder-history")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, ".config", "Qoder", "SharedClientCache", "cache", "db", "local.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE chat_session (session_id TEXT PRIMARY KEY, project_uri TEXT);
CREATE TABLE chat_message (
	id TEXT PRIMARY KEY,
	session_id TEXT,
	request_id TEXT,
	role TEXT,
	content TEXT,
	tool_result TEXT,
	token_info TEXT,
	gmt_create INTEGER
)`); err != nil {
		t.Fatal(err)
	}
	sessionID := "99947f30-f6f8-4323-a2c1-4970f5329d9c"
	if _, err := db.Exec(`INSERT INTO chat_session(session_id, project_uri) VALUES(?, ?)`, sessionID, project); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chat_message(id, session_id, request_id, role, content, tool_result, token_info, gmt_create) VALUES
		('user-1', ?, 'req-1', 'user', '', '', '', 1782576300000),
		('assistant-1', ?, 'req-1', 'assistant', '', '', '{"prompt_tokens":13,"completion_tokens":8}', 1782576301000)
	`, sessionID, sessionID); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dbPath, time.Date(2026, 6, 27, 17, 0, 2, 0, time.UTC), time.Date(2026, 6, 27, 17, 0, 2, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	historyDir := filepath.Join(home, ".qoder", "cache", "projects", "qoder-history", "conversation-history", "99947f30")
	if err := os.MkdirAll(historyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	prompt := "Add your name to the AUTHORS file in this repo."
	historyLine, err := json.Marshal(map[string]any{
		"role": "user",
		"message": map[string]any{
			"content": []map[string]string{{
				"type": "text",
				"text": strings.Join([]string{
					"<system-reminder>",
					"ignore this wrapper",
					"</system-reminder>",
					"<user_query>",
					prompt,
					"</user_query>",
				}, "\n"),
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	historyPath := filepath.Join(historyDir, "99947f30.jsonl")
	if err := os.WriteFile(historyPath, append(historyLine, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(historyPath, time.Date(2026, 6, 27, 17, 0, 2, 0, time.UTC), time.Date(2026, 6, 27, 17, 0, 2, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, hb := range heartbeats {
		if hb.Entity == "Qoder "+sessionID && hb.EntityType == "app" {
			if hb.AIPromptLength == nil || *hb.AIPromptLength != len(prompt) {
				t.Fatalf("expected Qoder prompt length from history, got %#v", hb)
			}
			return
		}
	}
	t.Fatalf("missing Qoder app heartbeat: %#v", heartbeats)
}

func TestSyncAIActivityParsesOpenCodeSQLiteState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	project := filepath.Join(home, "src", "opencode-project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, ".local", "share", "opencode", "opencode.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE session (id TEXT, directory TEXT, version TEXT);
		CREATE TABLE message (id TEXT, session_id TEXT, data TEXT, time_created INTEGER);
		CREATE TABLE part (id TEXT, message_id TEXT, session_id TEXT, data TEXT, time_created INTEGER);
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO session(id, directory, version) VALUES (?, ?, ?)`, "opencode-session", project, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO message(id, session_id, data, time_created) VALUES (?, ?, ?, ?)`,
		"message-1", "opencode-session", `{"id":"message-1","sessionID":"opencode-session","role":"assistant","modelID":"gpt-5-codex","tokens":{"input":7,"output":3},"path":{"cwd":"`+filepath.ToSlash(project)+`"}}`, int64(1782579900000)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO part(id, message_id, session_id, data, time_created) VALUES (?, ?, ?, ?, ?)`,
		"part-1", "message-1", "opencode-session", `{"type":"tool","file_path":"main.go","text":"edited main"}`, int64(1782579901000)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dbPath, time.Date(2026, 6, 27, 18, 25, 1, 0, time.UTC), time.Date(2026, 6, 27, 18, 25, 1, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var foundApp, foundFile bool
	for _, hb := range heartbeats {
		if hb.Entity == "OpenCode opencode-session" && hb.EntityType == "app" {
			foundApp = true
			if hb.Project != filepath.Base(project) || hb.AIInputTokens == nil || *hb.AIInputTokens != 7 || hb.AIOutputTokens == nil || *hb.AIOutputTokens != 3 {
				t.Fatalf("unexpected OpenCode app heartbeat: %#v", hb)
			}
		}
		if hb.Entity == file && hb.EntityType == "file" {
			foundFile = true
			if hb.Project != filepath.Base(project) || hb.Language != "Go" {
				t.Fatalf("unexpected OpenCode file heartbeat: %#v", hb)
			}
		}
	}
	if !foundApp || !foundFile {
		t.Fatalf("missing OpenCode heartbeats: %#v", heartbeats)
	}
}

func TestSyncAIActivityParsesCodySQLiteChatHistory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	readPath := filepath.Join(home, "read.go")
	editPath := filepath.Join(home, "edit.go")
	if err := os.WriteFile(readPath, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(editPath, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, "Library", "Application Support", "Code", "User", "globalStorage", "state.vscdb")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE ItemTable (key TEXT UNIQUE, value BLOB)`); err != nil {
		t.Fatal(err)
	}
	storage := map[string]any{
		"cody-local-chatHistory-v2": map[string]any{
			"account-key": map[string]any{
				"chat": map[string]any{
					"chat-1": map[string]any{
						"id":                       "chat-1",
						"lastInteractionTimestamp": "2026-05-01T20:00:00Z",
						"interactions": []map[string]any{
							{
								"humanMessage": map[string]any{"text": "old prompt"},
							},
							{
								"humanMessage": map[string]any{
									"text": "Please inspect and edit the file",
									"contextFiles": []map[string]any{{
										"type": "file",
										"uri": map[string]any{
											"scheme": "file",
											"path":   readPath,
										},
									}},
								},
								"assistantMessage": map[string]any{
									"text":  "Done",
									"model": "claude-3.5",
									"tokenUsage": map[string]any{
										"promptTokens":     10,
										"completionTokens": 4,
									},
									"processes": []map[string]any{{
										"items": []map[string]any{{
											"type":     "tool-state",
											"toolName": "text_editor",
											"uri": map[string]any{
												"scheme": "file",
												"path":   editPath,
											},
											"metadata": []string{"old\n", "old\nnew\n"},
										}},
									}},
								},
							},
						},
					},
				},
			},
		},
	}
	if _, err := db.Exec(`INSERT INTO ItemTable(key, value) VALUES(?, ?)`, "sourcegraph.cody-ai", testJSONString(t, storage)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dbPath, time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC), time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1777662000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var foundApp, foundRead, foundEdit bool
	for _, hb := range heartbeats {
		switch hb.Entity {
		case "Cody chat-1":
			foundApp = true
			if hb.AIPromptLength == nil || *hb.AIPromptLength != len("Please inspect and edit the file") {
				t.Fatalf("unexpected Cody prompt metadata: %#v", hb)
			}
			if hb.AIModel != "claude-3.5" || hb.AIInputTokens == nil || *hb.AIInputTokens != 10 || hb.AIOutputTokens == nil || *hb.AIOutputTokens != 4 {
				t.Fatalf("unexpected Cody token/model metadata: %#v", hb)
			}
		case readPath:
			foundRead = true
			if hb.IsWrite || hb.AILineChanges == nil || *hb.AILineChanges != 0 {
				t.Fatalf("unexpected Cody read heartbeat: %#v", hb)
			}
		case editPath:
			foundEdit = true
			if !hb.IsWrite || hb.AILineChanges == nil || *hb.AILineChanges != 1 {
				t.Fatalf("unexpected Cody edit heartbeat: %#v", hb)
			}
		}
	}
	if !foundApp || !foundRead || !foundEdit {
		t.Fatalf("missing Cody heartbeats: %#v", heartbeats)
	}
}

func TestSyncAIActivityParsesGooseSQLiteSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "goose-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, ".local", "share", "goose", "sessions", "sessions.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE sessions (
		id TEXT,
		name TEXT,
		working_dir TEXT,
		updated_at TEXT,
		total_input_tokens INTEGER,
		total_output_tokens INTEGER
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sessions(id, name, working_dir, updated_at, total_input_tokens, total_output_tokens) VALUES(?, ?, ?, ?, ?, ?)`,
		"goose-session", "implement feature", project, "2026-06-27T15:00:00Z", 11, 6); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dbPath, time.Date(2026, 6, 27, 15, 0, 0, 0, time.UTC), time.Date(2026, 6, 27, 15, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, hb := range heartbeats {
		if hb.Entity == "Goose goose-session" && hb.EntityType == "app" {
			found = true
			if hb.Project != filepath.Base(project) || hb.AIInputTokens == nil || *hb.AIInputTokens != 11 || hb.AIOutputTokens == nil || *hb.AIOutputTokens != 6 {
				t.Fatalf("unexpected Goose heartbeat: %#v", hb)
			}
		}
	}
	if !found {
		t.Fatalf("missing Goose heartbeat: %#v", heartbeats)
	}
}
