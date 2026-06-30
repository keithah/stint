package stintcli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

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
