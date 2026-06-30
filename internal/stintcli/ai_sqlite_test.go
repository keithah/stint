package stintcli

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestSyncAIActivityParsesCursorSQLiteState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "cursor-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "main.py"), []byte("print('ok')\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, ".config", "Cursor", "User", "globalStorage", "state.vscdb")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE cursorDiskKV (key TEXT UNIQUE ON CONFLICT REPLACE, value BLOB)`); err != nil {
		t.Fatal(err)
	}
	value := `{"createdAt":"2026-06-27T14:00:00Z","cwd":"` + filepath.ToSlash(project) + `","text":"inspect file","filePath":"main.py","tokenCount":{"inputTokens":9,"outputTokens":4}}`
	if _, err := db.Exec(`INSERT INTO cursorDiskKV(key, value) VALUES(?, ?)`, "bubbleId:cursor-session:user", value); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dbPath, time.Date(2026, 6, 27, 14, 0, 0, 0, time.UTC), time.Date(2026, 6, 27, 14, 0, 0, 0, time.UTC)); err != nil {
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
		if hb.Entity == "Cursor cursor-session" && hb.EntityType == "app" {
			found = true
			if hb.AIInputTokens == nil || *hb.AIInputTokens != 9 || hb.AIOutputTokens == nil || *hb.AIOutputTokens != 4 {
				t.Fatalf("unexpected Cursor heartbeat: %#v", hb)
			}
		}
	}
	if !found {
		t.Fatalf("missing Cursor heartbeat: %#v", heartbeats)
	}
}

func TestSyncAIActivityParsesQoderSQLiteState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "qoder-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, ".config", "Qoder", "SharedClientCache", "cache", "db", "local.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE chat_session (session_id TEXT PRIMARY KEY, project_uri TEXT);
CREATE TABLE chat_message (
	id TEXT PRIMARY KEY,
	session_id TEXT,
	request_id TEXT,
	role TEXT,
	content TEXT,
	tool_result TEXT,
	token_info TEXT,
	gmt_create INTEGER
)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chat_session(session_id, project_uri) VALUES(?, ?)`, "qoder-session", project); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chat_message(id, session_id, request_id, role, content, tool_result, token_info, gmt_create) VALUES
		('user-1', 'qoder-session', 'req-1', 'user', 'change main', '', '', 1782576300000),
		('assistant-1', 'qoder-session', 'req-1', 'assistant', '', '', '{"prompt_tokens":13,"completion_tokens":8}', 1782576301000),
		('tool-1', 'qoder-session', 'req-1', 'tool', '', '{"sessionId":"qoder-session","toolCallName":"search_replace","results":[{"path":"` + filepath.ToSlash(file) + `","diffInfo":{"add":2,"delete":1}}]}', '', 1782576302000)
	`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dbPath, time.Date(2026, 6, 27, 17, 0, 2, 0, time.UTC), time.Date(2026, 6, 27, 17, 0, 2, 0, time.UTC)); err != nil {
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
		if hb.Entity == "Qoder qoder-session" && hb.EntityType == "app" {
			foundApp = true
			if hb.Project != filepath.Base(project) || hb.AIInputTokens == nil || *hb.AIInputTokens != 13 || hb.AIOutputTokens == nil || *hb.AIOutputTokens != 8 {
				t.Fatalf("unexpected Qoder app heartbeat: %#v", hb)
			}
		}
		if hb.Entity == file && hb.EntityType == "file" {
			foundFile = true
			if hb.Project != filepath.Base(project) || hb.Language != "Go" {
				t.Fatalf("unexpected Qoder file heartbeat: %#v", hb)
			}
		}
	}
	if !foundApp || !foundFile {
		t.Fatalf("missing Qoder heartbeats: %#v", heartbeats)
	}
}

func TestSyncAIActivityUsesQoderHistoryForPromptLength(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "qoder-history")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, ".config", "Qoder", "SharedClientCache", "cache", "db", "local.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE chat_session (session_id TEXT PRIMARY KEY, project_uri TEXT);
CREATE TABLE chat_message (
	id TEXT PRIMARY KEY,
	session_id TEXT,
	request_id TEXT,
	role TEXT,
	content TEXT,
	tool_result TEXT,
	token_info TEXT,
	gmt_create INTEGER
)`); err != nil {
		t.Fatal(err)
	}
	sessionID := "99947f30-f6f8-4323-a2c1-4970f5329d9c"
	if _, err := db.Exec(`INSERT INTO chat_session(session_id, project_uri) VALUES(?, ?)`, sessionID, project); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO chat_message(id, session_id, request_id, role, content, tool_result, token_info, gmt_create) VALUES
		('user-1', ?, 'req-1', 'user', '', '', '', 1782576300000),
		('assistant-1', ?, 'req-1', 'assistant', '', '', '{"prompt_tokens":13,"completion_tokens":8}', 1782576301000)
	`, sessionID, sessionID); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dbPath, time.Date(2026, 6, 27, 17, 0, 2, 0, time.UTC), time.Date(2026, 6, 27, 17, 0, 2, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	historyDir := filepath.Join(home, ".qoder", "cache", "projects", "qoder-history", "conversation-history", "99947f30")
	if err := os.MkdirAll(historyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	prompt := "Add your name to the AUTHORS file in this repo."
	historyLine, err := json.Marshal(map[string]any{
		"role": "user",
		"message": map[string]any{
			"content": []map[string]string{{
				"type": "text",
				"text": strings.Join([]string{
					"<system-reminder>",
					"ignore this wrapper",
					"</system-reminder>",
					"<user_query>",
					prompt,
					"</user_query>",
				}, "\n"),
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	historyPath := filepath.Join(historyDir, "99947f30.jsonl")
	if err := os.WriteFile(historyPath, append(historyLine, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(historyPath, time.Date(2026, 6, 27, 17, 0, 2, 0, time.UTC), time.Date(2026, 6, 27, 17, 0, 2, 0, time.UTC)); err != nil {
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
		if hb.Entity == "Qoder "+sessionID && hb.EntityType == "app" {
			if hb.AIPromptLength == nil || *hb.AIPromptLength != len(prompt) {
				t.Fatalf("expected Qoder prompt length from history, got %#v", hb)
			}
			return
		}
	}
	t.Fatalf("missing Qoder app heartbeat: %#v", heartbeats)
}

func TestSyncAIActivityParsesOpenCodeSQLiteState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	project := filepath.Join(home, "src", "opencode-project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(project, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, ".local", "share", "opencode", "opencode.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE session (id TEXT, directory TEXT, version TEXT);
		CREATE TABLE message (id TEXT, session_id TEXT, data TEXT, time_created INTEGER);
		CREATE TABLE part (id TEXT, message_id TEXT, session_id TEXT, data TEXT, time_created INTEGER);
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO session(id, directory, version) VALUES (?, ?, ?)`, "opencode-session", project, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO message(id, session_id, data, time_created) VALUES (?, ?, ?, ?)`,
		"message-1", "opencode-session", `{"id":"message-1","sessionID":"opencode-session","role":"assistant","modelID":"gpt-5-codex","tokens":{"input":7,"output":3},"path":{"cwd":"`+filepath.ToSlash(project)+`"}}`, int64(1782579900000)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO part(id, message_id, session_id, data, time_created) VALUES (?, ?, ?, ?, ?)`,
		"part-1", "message-1", "opencode-session", `{"type":"tool","file_path":"main.go","text":"edited main"}`, int64(1782579901000)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dbPath, time.Date(2026, 6, 27, 18, 25, 1, 0, time.UTC), time.Date(2026, 6, 27, 18, 25, 1, 0, time.UTC)); err != nil {
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
		if hb.Entity == "OpenCode opencode-session" && hb.EntityType == "app" {
			foundApp = true
			if hb.Project != filepath.Base(project) || hb.AIInputTokens == nil || *hb.AIInputTokens != 7 || hb.AIOutputTokens == nil || *hb.AIOutputTokens != 3 {
				t.Fatalf("unexpected OpenCode app heartbeat: %#v", hb)
			}
		}
		if hb.Entity == file && hb.EntityType == "file" {
			foundFile = true
			if hb.Project != filepath.Base(project) || hb.Language != "Go" {
				t.Fatalf("unexpected OpenCode file heartbeat: %#v", hb)
			}
		}
	}
	if !foundApp || !foundFile {
		t.Fatalf("missing OpenCode heartbeats: %#v", heartbeats)
	}
}

func TestSyncAIActivityParsesCodySQLiteChatHistory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	readPath := filepath.Join(home, "read.go")
	editPath := filepath.Join(home, "edit.go")
	if err := os.WriteFile(readPath, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(editPath, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, "Library", "Application Support", "Code", "User", "globalStorage", "state.vscdb")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE ItemTable (key TEXT UNIQUE, value BLOB)`); err != nil {
		t.Fatal(err)
	}
	storage := map[string]any{
		"cody-local-chatHistory-v2": map[string]any{
			"account-key": map[string]any{
				"chat": map[string]any{
					"chat-1": map[string]any{
						"id":                       "chat-1",
						"lastInteractionTimestamp": "2026-05-01T20:00:00Z",
						"interactions": []map[string]any{
							{
								"humanMessage": map[string]any{"text": "old prompt"},
							},
							{
								"humanMessage": map[string]any{
									"text": "Please inspect and edit the file",
									"contextFiles": []map[string]any{{
										"type": "file",
										"uri": map[string]any{
											"scheme": "file",
											"path":   readPath,
										},
									}},
								},
								"assistantMessage": map[string]any{
									"text":  "Done",
									"model": "claude-3.5",
									"tokenUsage": map[string]any{
										"promptTokens":     10,
										"completionTokens": 4,
									},
									"processes": []map[string]any{{
										"items": []map[string]any{{
											"type":     "tool-state",
											"toolName": "text_editor",
											"uri": map[string]any{
												"scheme": "file",
												"path":   editPath,
											},
											"metadata": []string{"old\n", "old\nnew\n"},
										}},
									}},
								},
							},
						},
					},
				},
			},
		},
	}
	if _, err := db.Exec(`INSERT INTO ItemTable(key, value) VALUES(?, ?)`, "sourcegraph.cody-ai", testJSONString(t, storage)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dbPath, time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC), time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	heartbeats, _, err := collectAIHeartbeats(Options{
		InternalConfig: Config{Sections: map[string]map[string]string{}},
		SyncAIAfter:    1777662000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var foundApp, foundRead, foundEdit bool
	for _, hb := range heartbeats {
		switch hb.Entity {
		case "Cody chat-1":
			foundApp = true
			if hb.AIPromptLength == nil || *hb.AIPromptLength != len("Please inspect and edit the file") {
				t.Fatalf("unexpected Cody prompt metadata: %#v", hb)
			}
			if hb.AIModel != "claude-3.5" || hb.AIInputTokens == nil || *hb.AIInputTokens != 10 || hb.AIOutputTokens == nil || *hb.AIOutputTokens != 4 {
				t.Fatalf("unexpected Cody token/model metadata: %#v", hb)
			}
		case readPath:
			foundRead = true
			if hb.IsWrite || hb.AILineChanges == nil || *hb.AILineChanges != 0 {
				t.Fatalf("unexpected Cody read heartbeat: %#v", hb)
			}
		case editPath:
			foundEdit = true
			if !hb.IsWrite || hb.AILineChanges == nil || *hb.AILineChanges != 1 {
				t.Fatalf("unexpected Cody edit heartbeat: %#v", hb)
			}
		}
	}
	if !foundApp || !foundRead || !foundEdit {
		t.Fatalf("missing Cody heartbeats: %#v", heartbeats)
	}
}

func TestSyncAIActivityParsesGooseSQLiteSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "src", "goose-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(home, ".local", "share", "goose", "sessions", "sessions.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE sessions (
		id TEXT,
		name TEXT,
		working_dir TEXT,
		updated_at TEXT,
		total_input_tokens INTEGER,
		total_output_tokens INTEGER
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sessions(id, name, working_dir, updated_at, total_input_tokens, total_output_tokens) VALUES(?, ?, ?, ?, ?, ?)`,
		"goose-session", "implement feature", project, "2026-06-27T15:00:00Z", 11, 6); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(dbPath, time.Date(2026, 6, 27, 15, 0, 0, 0, time.UTC), time.Date(2026, 6, 27, 15, 0, 0, 0, time.UTC)); err != nil {
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
		if hb.Entity == "Goose goose-session" && hb.EntityType == "app" {
			found = true
			if hb.Project != filepath.Base(project) || hb.AIInputTokens == nil || *hb.AIInputTokens != 11 || hb.AIOutputTokens == nil || *hb.AIOutputTokens != 6 {
				t.Fatalf("unexpected Goose heartbeat: %#v", hb)
			}
		}
	}
	if !found {
		t.Fatalf("missing Goose heartbeat: %#v", heartbeats)
	}
}
