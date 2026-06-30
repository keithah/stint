package stintcli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestSyncAIActivityParsesCopilotWorkspaceSessionFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	project := filepath.Join(home, "copilot project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	workspaceDir := filepath.Join(home, "Library", "Application Support", "Code", "User", "workspaceStorage", "workspace-1", "chatSessions")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(workspaceDir, "session-1.json")
	session := map[string]any{
		"lastMessageDate": int64(1770000100000),
		"sessionId":       "session-1",
		"requests": []any{
			map[string]any{
				"timestamp": int64(1770000001000),
				"message": map[string]any{
					"text": "Update the file and explain what changed",
				},
				"result": map[string]any{
					"metadata": map[string]any{
						"promptTokens": 9,
						"outputTokens": 4,
					},
				},
				"variableData": map[string]any{
					"variables": []any{
						map[string]any{
							"kind": "file",
							"id":   "file://" + strings.ReplaceAll(filepath.ToSlash(file), " ", "%20"),
							"value": map[string]any{
								"fsPath": file,
							},
						},
					},
				},
			},
		},
	}
	data, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessionPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1769999990,
	})
	if err != nil {
		t.Fatal(err)
	}

	var foundApp, foundFile bool
	for _, hb := range heartbeats {
		if hb.Entity == "Copilot session-1" && hb.EntityType == "app" {
			foundApp = true
			if hb.AIPromptLength == nil || *hb.AIPromptLength != len("Update the file and explain what changed") {
				t.Fatalf("unexpected Copilot app heartbeat: %#v", hb)
			}
			if hb.AIInputTokens == nil || *hb.AIInputTokens != 9 || hb.AIOutputTokens == nil || *hb.AIOutputTokens != 4 {
				t.Fatalf("unexpected Copilot token metadata: %#v", hb)
			}
		}
		if hb.Entity == file && hb.EntityType == "file" {
			foundFile = true
			if hb.Project != filepath.Base(project) {
				t.Fatalf("unexpected Copilot file heartbeat: %#v", hb)
			}
		}
	}
	if !foundApp || !foundFile {
		t.Fatalf("missing Copilot workspace heartbeats: %#v", heartbeats)
	}
}

func TestSyncAIActivityParsesCodexSuccessfulApplyPatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	project := filepath.Join(home, "codex-project")
	successful := filepath.Join(project, "successful.go")
	failed := filepath.Join(project, "failed.go")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(successful, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(failed, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(home, ".codex", "sessions", "2026", "06", "20")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptPath := filepath.Join(transcriptDir, "session.jsonl")
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-20T12:00:00Z","type":"session_meta","payload":{"id":"codex-session","cwd":"` + filepath.ToSlash(project) + `"}}`,
		`{"timestamp":"2026-06-20T12:00:01Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"failed-direct","name":"apply_patch","input":"*** Update File: failed.go\n+one"}}`,
		`{"timestamp":"2026-06-20T12:00:02Z","type":"event_msg","payload":{"type":"patch_apply_end","call_id":"failed-direct","success":false,"status":"failed"}}`,
		`{"timestamp":"2026-06-20T12:00:03Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"successful-direct","name":"apply_patch","input":"*** Update File: successful.go\n+one"}}`,
		`{"timestamp":"2026-06-20T12:00:04Z","type":"event_msg","payload":{"type":"patch_apply_end","call_id":"successful-direct","success":true,"status":"completed"}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(transcriptPath, time.Date(2026, 6, 20, 12, 0, 4, 0, time.UTC), time.Date(2026, 6, 20, 12, 0, 4, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, hb := range heartbeats {
		if hb.Entity == failed {
			t.Fatalf("failed Codex patch was emitted: %#v", heartbeats)
		}
		if hb.Entity == successful {
			found = true
			if !hb.IsWrite || hb.AILineChanges == nil || *hb.AILineChanges != 1 {
				t.Fatalf("unexpected Codex patch heartbeat: %#v", hb)
			}
		}
	}
	if !found {
		t.Fatalf("missing successful Codex patch heartbeat: %#v", heartbeats)
	}
}

func TestSyncAIActivityParsesCodexSuccessfulShellApplyPatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	project := filepath.Join(home, "codex-shell-project")
	successful := filepath.Join(project, "via-shell.go")
	failed := filepath.Join(project, "failed-shell.go")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(successful, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(failed, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(home, ".codex", "sessions", "2026", "06", "23")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	successCommand := "apply_patch <<'PATCH'\n*** Begin Patch\n*** Update File: via-shell.go\n+one\n+two\n*** End Patch\nPATCH"
	failedCommand := "apply_patch <<'PATCH'\n*** Begin Patch\n*** Update File: failed-shell.go\n+bad\n*** End Patch\nPATCH"
	transcriptPath := filepath.Join(transcriptDir, "session.jsonl")
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-23T12:00:00Z","type":"session_meta","payload":{"id":"codex-shell-session","cwd":"` + filepath.ToSlash(project) + `"}}`,
		`{"timestamp":"2026-06-23T12:00:01Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"failed-shell","name":"exec_command","input":` + strconv.Quote(`{"cmd":`+strconv.Quote(failedCommand)+`}`) + `}}`,
		`{"timestamp":"2026-06-23T12:00:02Z","type":"event_msg","payload":{"type":"exec_command_end","call_id":"failed-shell","exit_code":1}}`,
		`{"timestamp":"2026-06-23T12:00:03Z","type":"response_item","payload":{"type":"custom_tool_call","call_id":"successful-shell","name":"exec_command","input":` + strconv.Quote(`{"cmd":`+strconv.Quote(successCommand)+`}`) + `}}`,
		`{"timestamp":"2026-06-23T12:00:04Z","type":"event_msg","payload":{"type":"exec_command_end","call_id":"successful-shell","exit_code":0}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(transcriptPath, time.Date(2026, 6, 23, 12, 0, 4, 0, time.UTC), time.Date(2026, 6, 23, 12, 0, 4, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, hb := range heartbeats {
		if hb.Entity == failed {
			t.Fatalf("failed Codex shell patch was emitted: %#v", heartbeats)
		}
		if hb.Entity == successful {
			found = true
			if !hb.IsWrite || hb.AILineChanges == nil || *hb.AILineChanges != 2 {
				t.Fatalf("unexpected Codex shell patch heartbeat: %#v", hb)
			}
		}
	}
	if !found {
		t.Fatalf("missing successful Codex shell patch heartbeat: %#v", heartbeats)
	}
}

func TestSyncAIActivityStripsCodexPromptWrappers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	project := filepath.Join(home, "codex-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(home, ".codex", "sessions", "2026", "06", "21")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	userPrompt := "Add cache warming for the dashboard stats query"
	wrappedPrompt := "# Context from my IDE setup:\n\n## Active file: internal/api/stats.go\n\n## My request for Codex:\n" + userPrompt
	transcriptPath := filepath.Join(transcriptDir, "session.jsonl")
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-21T12:00:00Z","type":"session_meta","payload":{"id":"codex-wrapper-session","cwd":"` + filepath.ToSlash(project) + `"}}`,
		`{"timestamp":"2026-06-21T12:00:01Z","type":"event_msg","payload":{"message":` + strconv.Quote(wrappedPrompt) + `,"info":{"total_token_usage":{"input_tokens":20,"output_tokens":7}}}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(transcriptPath, time.Date(2026, 6, 21, 12, 0, 1, 0, time.UTC), time.Date(2026, 6, 21, 12, 0, 1, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(heartbeats) != 1 {
		t.Fatalf("heartbeats = %#v", heartbeats)
	}
	if heartbeats[0].AIPromptLength == nil || *heartbeats[0].AIPromptLength != len(userPrompt) {
		t.Fatalf("prompt length = %#v, want %d; heartbeat=%#v", heartbeats[0].AIPromptLength, len(userPrompt), heartbeats[0])
	}
}

func TestSyncAIActivityStripsClaudeSystemReminderFromPrompt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	project := filepath.Join(home, "claude-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(home, ".claude", "projects", "claude-project")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	userPrompt := "Please update the README with setup notes"
	wrappedPrompt := "<system-reminder>\nUse the TodoWrite tool for substantial tasks.\n</system-reminder>\n" + userPrompt
	transcriptPath := filepath.Join(transcriptDir, "session.jsonl")
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-22T12:00:00Z","sessionId":"claude-wrapper-session","cwd":"` + filepath.ToSlash(project) + `","message":{"role":"user","content":[{"type":"text","text":` + strconv.Quote(wrappedPrompt) + `}]}}`,
		`{"timestamp":"2026-06-22T12:00:01Z","sessionId":"claude-wrapper-session","message":{"role":"assistant","model":"claude-sonnet-4.5","usage":{"input_tokens":30,"output_tokens":11}}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(transcriptPath, time.Date(2026, 6, 22, 12, 0, 1, 0, time.UTC), time.Date(2026, 6, 22, 12, 0, 1, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(heartbeats) != 1 {
		t.Fatalf("heartbeats = %#v", heartbeats)
	}
	if heartbeats[0].AIPromptLength == nil || *heartbeats[0].AIPromptLength != len(userPrompt) {
		t.Fatalf("prompt length = %#v, want %d; heartbeat=%#v", heartbeats[0].AIPromptLength, len(userPrompt), heartbeats[0])
	}
	if heartbeats[0].AIModel != "claude-sonnet-4.5" || heartbeats[0].AIInputTokens == nil || *heartbeats[0].AIInputTokens != 30 || heartbeats[0].AIOutputTokens == nil || *heartbeats[0].AIOutputTokens != 11 {
		t.Fatalf("unexpected Claude metadata: %#v", heartbeats[0])
	}
}

func TestSyncAIActivityParsesClaudeSubscriptionPlan(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	project := filepath.Join(home, "claude-plan-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(home, ".claude", "projects", "claude-plan-project")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcriptPath := filepath.Join(transcriptDir, "session.jsonl")
	transcript := strings.Join([]string{
		`{"timestamp":"2026-06-24T12:00:00Z","sessionId":"claude-plan-session","cwd":"` + filepath.ToSlash(project) + `","message":{"role":"user","content":"Review the billing code"},"config":{"subscriptionPlan":"max"}}`,
		`{"timestamp":"2026-06-24T12:00:01Z","sessionId":"claude-plan-session","message":{"role":"assistant","model":"claude-sonnet-4.5","usage":{"input_tokens":9,"output_tokens":4}}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(transcriptPath, time.Date(2026, 6, 24, 12, 0, 1, 0, time.UTC), time.Date(2026, 6, 24, 12, 0, 1, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1780000000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(heartbeats) != 1 {
		t.Fatalf("heartbeats = %#v", heartbeats)
	}
	if heartbeats[0].AISubscriptionPlan != "max" {
		t.Fatalf("subscription plan = %q; heartbeat=%#v", heartbeats[0].AISubscriptionPlan, heartbeats[0])
	}
}

func TestSyncAIActivityParsesCopilotCLISessionEvents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	project := filepath.Join(home, "copilot-cli-project")
	readme := filepath.Join(project, "README.md")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readme, []byte("# Project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sessionID := "cli-session-1"
	eventsPath := filepath.Join(home, ".copilot", "session-state", sessionID, "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(eventsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestJSONLines(t, eventsPath, []map[string]any{
		{
			"type":      "session.start",
			"timestamp": "2026-06-14T12:00:00Z",
			"data": map[string]any{
				"sessionId":      sessionID,
				"copilotVersion": "1.0.62",
				"context": map[string]any{
					"cwd":     project,
					"gitRoot": project,
				},
			},
		},
		{
			"type":      "session.model_change",
			"timestamp": "2026-06-14T12:00:00.500Z",
			"data": map[string]any{
				"newModel": "gpt-5.4",
			},
		},
		{
			"type":      "user.message",
			"timestamp": "2026-06-14T12:00:01Z",
			"data": map[string]any{
				"content": "Update README",
			},
		},
		{
			"type":      "assistant.message",
			"timestamp": "2026-06-14T12:00:02Z",
			"data": map[string]any{
				"outputTokens": 9,
			},
		},
		{
			"type":      "tool.execution_complete",
			"timestamp": "2026-06-14T12:00:03Z",
			"data": map[string]any{
				"success": true,
				"toolTelemetry": map[string]any{
					"restrictedProperties": map[string]any{
						"filePaths": testJSONString(t, []string{readme}),
					},
					"metrics": map[string]any{
						"linesAdded":   2,
						"linesRemoved": 1,
					},
				},
			},
		},
		{
			"type":      "session.shutdown",
			"timestamp": "2026-06-14T12:00:04Z",
			"data": map[string]any{
				"tokenDetails": map[string]any{
					"input": map[string]any{
						"tokenCount": 17,
					},
					"output": map[string]any{
						"tokenCount": 11,
					},
				},
			},
		},
	})
	modTime := time.Date(2026, 6, 14, 12, 4, 0, 0, time.UTC)
	if err := os.Chtimes(eventsPath, modTime, modTime); err != nil {
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
		if hb.Entity == "Copilot "+sessionID && hb.EntityType == "app" {
			foundApp = true
			if hb.AISession != sessionID || hb.AIAgentVersion != "1.0.62" || hb.AIModel != "gpt-5.4" {
				t.Fatalf("missing Copilot CLI identity metadata: %#v", hb)
			}
			if hb.AIPromptLength == nil || *hb.AIPromptLength != len("Update README") {
				t.Fatalf("missing Copilot CLI prompt length: %#v", hb)
			}
			if hb.AIInputTokens == nil || *hb.AIInputTokens != 17 || hb.AIOutputTokens == nil || *hb.AIOutputTokens != 20 {
				t.Fatalf("unexpected Copilot CLI token metadata: %#v", hb)
			}
		}
		if hb.Entity == readme && hb.EntityType == "file" {
			foundFile = true
			if !hb.IsWrite || hb.AILineChanges == nil || *hb.AILineChanges != 1 {
				t.Fatalf("missing Copilot CLI file metadata: %#v", hb)
			}
		}
	}
	if !foundApp || !foundFile {
		t.Fatalf("missing Copilot CLI heartbeats: %#v", heartbeats)
	}
}

func TestSyncAIActivityParsesContinueDevDataWithWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	continueDir := filepath.Join(home, ".continue")
	devDataDir := filepath.Join(continueDir, "dev_data", "0.2.0")
	sessionsDir := filepath.Join(continueDir, "sessions")
	if err := os.MkdirAll(devDataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(home, "continue-project")
	readPath := filepath.Join(workspace, "README.md")
	editPath := filepath.Join(workspace, "pkg", "ai", "continue.go")
	if err := os.MkdirAll(filepath.Dir(editPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readPath, []byte("# Project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(editPath, []byte("package ai\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sessionID := "5832a36f-ea52-4eb4-b8f7-16ed2bf063dd"
	sessions := []map[string]any{{
		"sessionId":          sessionID,
		"workspaceDirectory": "file://" + filepath.ToSlash(workspace),
	}}
	if err := os.WriteFile(filepath.Join(sessionsDir, "sessions.json"), []byte(testJSONString(t, sessions)), 0o600); err != nil {
		t.Fatal(err)
	}
	writeTestJSONLines(t, filepath.Join(devDataDir, "tokensGenerated.jsonl"), []map[string]any{{
		"timestamp":       "2026-05-01T19:45:22.290Z",
		"eventName":       "tokensGenerated",
		"model":           "gpt-5.2",
		"provider":        "openai",
		"promptTokens":    151,
		"generatedTokens": 36,
	}})
	writeTestJSONLines(t, filepath.Join(devDataDir, "chatInteraction.jsonl"), []map[string]any{{
		"timestamp":     "2026-05-01T19:45:22.287Z",
		"eventName":     "chatInteraction",
		"prompt":        "<system>\nskip\n</system>\n<user>\nadd yourself to the readme in this project\n\n",
		"modelName":     "gpt-5.2",
		"modelProvider": "openai",
		"sessionId":     sessionID,
	}})
	writeTestJSONLines(t, filepath.Join(devDataDir, "toolUsage.jsonl"), []map[string]any{{
		"timestamp":    "2026-05-01T19:45:26.107Z",
		"eventName":    "toolUsage",
		"functionName": "read_file",
		"toolCallArgs": testJSONString(t, map[string]string{"filepath": "README.md"}),
		"accepted":     true,
		"succeeded":    true,
	}})
	writeTestJSONLines(t, filepath.Join(devDataDir, "editOutcome.jsonl"), []map[string]any{{
		"timestamp":     "2026-05-01T19:45:48.207Z",
		"eventName":     "editOutcome",
		"prompt":        "add yourself to the readme in this project",
		"modelName":     "gpt-5.2",
		"modelProvider": "openai",
		"accepted":      true,
		"lineChange":    2,
		"filepath":      "file://" + filepath.ToSlash(editPath),
	}})
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1777664722.281,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(heartbeats) != 3 {
		t.Fatalf("heartbeats = %#v", heartbeats)
	}
	app := heartbeats[0]
	if app.Entity != "Continue "+sessionID || app.EntityType != "app" || app.AISession != sessionID {
		t.Fatalf("unexpected Continue app heartbeat: %#v", app)
	}
	if app.Project != filepath.Base(workspace) || app.AIPromptLength == nil || *app.AIPromptLength != len("add yourself to the readme in this project") {
		t.Fatalf("missing Continue app project/prompt metadata: %#v", app)
	}
	if app.AIInputTokens == nil || *app.AIInputTokens != 151 || app.AIOutputTokens == nil || *app.AIOutputTokens != 36 {
		t.Fatalf("missing Continue token metadata: %#v", app)
	}
	read := heartbeats[1]
	if read.Entity != readPath || read.EntityType != "file" || read.IsWrite {
		t.Fatalf("unexpected Continue read heartbeat: %#v", read)
	}
	if read.AILineChanges == nil || *read.AILineChanges != 0 {
		t.Fatalf("missing Continue read line changes: %#v", read)
	}
	edit := heartbeats[2]
	if edit.Entity != editPath || edit.EntityType != "file" || !edit.IsWrite {
		t.Fatalf("unexpected Continue edit heartbeat: %#v", edit)
	}
	if edit.AILineChanges == nil || *edit.AILineChanges != 2 {
		t.Fatalf("missing Continue edit line changes: %#v", edit)
	}
}

func TestSyncAIActivityParsesAmpApplyPatchLogs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	project := filepath.Join(home, "amp-project")
	readme := filepath.Join(project, "README.md")
	testFile := filepath.Join(project, "pkg", "ai", "amp_test.go")
	if err := os.MkdirAll(filepath.Dir(testFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readme, []byte("old readme\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testFile, []byte("old test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	threadID := "T-019eea92-63e4-70dd-83d7-bfac30818089"
	transcriptPath := filepath.Join(home, ".cache", "amp", "logs", "threads", threadID+".log")
	if err := os.MkdirAll(filepath.Dir(transcriptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: README.md",
		"@@",
		"-Made by the team.",
		"+Made by the team and Amp.",
		"*** Update File: pkg/ai/amp_test.go",
		"@@",
		"-old test",
		"+new test",
		"+second new test",
		"*** End Patch",
	}, "\n")
	writeTestJSONLines(t, transcriptPath, []map[string]any{
		{
			"@timestamp": "2026-06-21T14:28:00.500Z",
			"type":       "agent_state",
			"threadId":   threadID,
		},
		{
			"@timestamp": "2026-06-21T14:28:07.006Z",
			"message":    "onToolLease",
			"threadId":   threadID,
			"data": map[string]any{
				"type":       "tool_lease",
				"toolCallId": "patch-1",
				"toolName":   "apply_patch",
				"args": map[string]any{
					"workdir":   project,
					"patchText": patch,
				},
			},
		},
		{
			"@timestamp": "2026-06-21T14:28:07.330Z",
			"type":       "executor_tool_result",
			"threadId":   threadID,
			"toolCallId": "patch-1",
			"runStatus":  "done",
		},
	})
	modTime := time.Date(2026, 6, 21, 14, 29, 0, 0, time.UTC)
	if err := os.Chtimes(transcriptPath, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1782048480,
	})
	if err != nil {
		t.Fatal(err)
	}
	var foundReadme, foundTest bool
	for _, hb := range heartbeats {
		switch hb.Entity {
		case readme:
			foundReadme = true
			if hb.EntityType != "file" || !hb.IsWrite || hb.AILineChanges == nil || *hb.AILineChanges != 0 {
				t.Fatalf("unexpected Amp README heartbeat: %#v", hb)
			}
		case testFile:
			foundTest = true
			if hb.EntityType != "file" || !hb.IsWrite || hb.AILineChanges == nil || *hb.AILineChanges != 1 {
				t.Fatalf("unexpected Amp test heartbeat: %#v", hb)
			}
		}
	}
	if !foundReadme || !foundTest {
		t.Fatalf("missing Amp patch file heartbeats: %#v", heartbeats)
	}
}
