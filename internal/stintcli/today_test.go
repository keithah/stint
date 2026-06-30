package stintcli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

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
