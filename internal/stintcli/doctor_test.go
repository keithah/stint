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

func TestDoctorReportsLocalIntegrationStatus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)
	wakaCLI := filepath.Join(home, "wakatime-cli")
	if err := os.WriteFile(wakaCLI, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STINT_WAKATIME_CLI", wakaCLI)

	if err := Run([]string{"setup", "--server", "http://stint.example/api/v1", "--key", "waka_doctor"}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	oldEditorLookPath := editorLookPath
	editorLookPath = func(name string) (string, error) {
		if name == "code" {
			return "/usr/bin/code", nil
		}
		return "", os.ErrNotExist
	}
	oldPluginLookPath := pluginLookPath
	pluginLookPath = func(name string) (string, error) {
		if name == "codex" {
			return "/usr/bin/codex", nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() {
		editorLookPath = oldEditorLookPath
		pluginLookPath = oldPluginLookPath
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/meta":
			_, _ = w.Write([]byte(`{"data":{"version":"dev"}}`))
		case "/api/v1/users/current/heartbeats":
			_, _ = w.Write([]byte(`[{"entity":"/tmp/main.go","time":123}]`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := Run([]string{"doctor", "--api-url", server.URL + "/api/v1", "--key", "waka_doctor", "--output", "json"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["ok"] != true || got["api_reachable"] != true {
		t.Fatalf("unexpected doctor health: %#v", got)
	}
	if got["wakatime_cli_status"] != "override" {
		t.Fatalf("unexpected wakatime status: %#v", got)
	}
	if got["api_url_source"] != "flag" {
		t.Fatalf("unexpected api_url_source: %#v", got)
	}
	if !strings.Contains(out.String(), "vscode") || !strings.Contains(out.String(), "codex") {
		t.Fatalf("expected detected tools in output: %s", out.String())
	}
}

func TestDoctorReportsOfflineQueueErrorInTextOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)
	wakaCLI := filepath.Join(home, "wakatime-cli")
	if err := os.WriteFile(wakaCLI, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STINT_WAKATIME_CLI", wakaCLI)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/meta":
			_, _ = w.Write([]byte(`{"data":{"version":"dev"}}`))
		case "/api/v1/users/current/heartbeats":
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()

	queuePath := filepath.Join(home, "offline_heartbeats.jsonl")
	if err := os.Mkdir(queuePath, 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := Run([]string{
		"doctor",
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_doctor",
		"--offline-queue-file", queuePath,
	}, nil, &out, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "offline queue unreadable") {
		t.Fatalf("expected offline queue problem, got %v", err)
	}
	if !strings.Contains(out.String(), "offline_queue_error=") || !strings.Contains(out.String(), "problems=offline queue unreadable") {
		t.Fatalf("expected offline queue error in text output, got %q", out.String())
	}
}

func TestDoctorFailsWhenWakaTimeCLIIsMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/meta" {
			_, _ = w.Write([]byte(`{"data":{"version":"dev"}}`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	err := Run([]string{"doctor", "--api-url", server.URL + "/api/v1", "--key", "waka_doctor"}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "wakatime-cli is not installed") {
		t.Fatalf("expected missing wakatime-cli problem, got %v", err)
	}
}
