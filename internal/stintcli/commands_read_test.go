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

func TestRunDoctorJSONOutputStaysValidJSON(t *testing.T) {
	body := `{"data":{"api_url":"http://example.test/api/v1","version":"dev"}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/meta":
			_, _ = w.Write([]byte(body))
		case "/api/v1/users/current/heartbeats":
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Fatalf("unexpected doctor endpoint path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	dir := t.TempDir()
	queuePath := filepath.Join(dir, "offline_heartbeats.bdb")
	wakaCLI := filepath.Join(dir, "wakatime-cli")
	if err := os.WriteFile(wakaCLI, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STINT_WAKATIME_CLI", wakaCLI)
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
