package stintcli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

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
