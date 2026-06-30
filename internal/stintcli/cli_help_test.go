package stintcli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

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
