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
