package stintcli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

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
