package stintcli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

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
