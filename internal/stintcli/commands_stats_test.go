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
