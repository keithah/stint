package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrepareHeartbeatFillsDefaultsAndCommitAliases(t *testing.T) {
	heartbeat := Heartbeat{Entity: "/tmp/main.go", Time: 123, Revision: "abc123"}

	PrepareHeartbeat(&heartbeat, HeartbeatDefaults{
		Plugin:          "wakatime",
		PluginVersion:   "v1.102.1",
		Editor:          "vscode",
		EditorVersion:   "1.89.0",
		OperatingSystem: "linux",
		Architecture:    "amd64",
		AIAgent:         "gpt",
		AIAgentVersion:  "5.5",
	})

	if heartbeat.Type != "file" {
		t.Fatalf("expected default type file, got %q", heartbeat.Type)
	}
	if heartbeat.CommitHash != "abc123" {
		t.Fatalf("expected commit_hash from revision, got %q", heartbeat.CommitHash)
	}
	if heartbeat.Editor != "vscode" {
		t.Fatalf("expected editor default, got %q", heartbeat.Editor)
	}
	if heartbeat.EditorVersion != "1.89.0" {
		t.Fatalf("expected editor version default, got %q", heartbeat.EditorVersion)
	}
	if heartbeat.OperatingSystem != "linux" {
		t.Fatalf("expected operating system default, got %q", heartbeat.OperatingSystem)
	}
	if heartbeat.Plugin != "wakatime" {
		t.Fatalf("expected plugin default, got %q", heartbeat.Plugin)
	}
	if heartbeat.PluginVersion != "v1.102.1" {
		t.Fatalf("expected plugin version default, got %q", heartbeat.PluginVersion)
	}
	if heartbeat.Architecture != "amd64" {
		t.Fatalf("expected architecture default, got %q", heartbeat.Architecture)
	}
	if heartbeat.AIAgent != "gpt" || heartbeat.AIAgentVersion != "5.5" {
		t.Fatalf("expected AI agent defaults, got agent=%q version=%q", heartbeat.AIAgent, heartbeat.AIAgentVersion)
	}
	if heartbeat.MachineName != "vscode-linux" {
		t.Fatalf("expected derived machine fallback, got %q", heartbeat.MachineName)
	}
}

func TestPrepareHeartbeatKeepsExplicitMachineName(t *testing.T) {
	heartbeat := Heartbeat{Entity: "/tmp/main.go", Time: 123, MachineName: "workstation"}

	PrepareHeartbeat(&heartbeat, HeartbeatDefaults{Editor: "codex", OperatingSystem: "darwin"})

	if heartbeat.MachineName != "workstation" {
		t.Fatalf("expected explicit machine name to be preserved, got %q", heartbeat.MachineName)
	}
}

func TestPrepareHeartbeatInfersCodexAIAgentWhenAIMetricsPresent(t *testing.T) {
	input := 100
	heartbeat := Heartbeat{Entity: "/tmp/main.go", Time: 123, Editor: "codex", AIInputTokens: &input}

	PrepareHeartbeat(&heartbeat, HeartbeatDefaults{})

	if heartbeat.AIAgent != "gpt" {
		t.Fatalf("expected Codex AI heartbeat to default to gpt, got %q", heartbeat.AIAgent)
	}
}

func TestPrepareHeartbeatInfersLanguageFromEntityExtension(t *testing.T) {
	heartbeat := Heartbeat{Entity: "/tmp/main.swift", Time: 123}

	PrepareHeartbeat(&heartbeat, HeartbeatDefaults{})

	if heartbeat.Language != "Swift" {
		t.Fatalf("expected Swift language inference, got %q", heartbeat.Language)
	}
}

func TestPrepareHeartbeatDetectsWakaTimeProjectFileProjectAndBranch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".wakatime-project"), []byte("cli-project\nfeature/parity\n"), 0o644); err != nil {
		t.Fatalf("write .wakatime-project: %v", err)
	}
	entity := filepath.Join(dir, "src", "main.go")
	if err := os.MkdirAll(filepath.Dir(entity), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}

	heartbeat := Heartbeat{Entity: entity, Time: 123}
	PrepareHeartbeat(&heartbeat, HeartbeatDefaults{})

	if heartbeat.Project != "cli-project" {
		t.Fatalf("expected .wakatime-project project, got %q", heartbeat.Project)
	}
	if heartbeat.Branch != "feature/parity" {
		t.Fatalf("expected .wakatime-project branch, got %q", heartbeat.Branch)
	}
}

func TestPrepareHeartbeatDetectsGitProjectAndBranch(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	entity := filepath.Join(dir, "cmd", "server", "main.go")
	if err := os.MkdirAll(filepath.Dir(entity), 0o755); err != nil {
		t.Fatalf("mkdir entity dir: %v", err)
	}

	heartbeat := Heartbeat{Entity: entity, Time: 123}
	PrepareHeartbeat(&heartbeat, HeartbeatDefaults{})

	if heartbeat.Project != filepath.Base(dir) {
		t.Fatalf("expected git folder project, got %q", heartbeat.Project)
	}
	if heartbeat.Branch != "main" {
		t.Fatalf("expected git branch, got %q", heartbeat.Branch)
	}
}

func TestPrepareHeartbeatDetectsWakaTimeSpecialLanguages(t *testing.T) {
	for _, test := range []struct {
		entity string
		want   string
	}{
		{entity: "/tmp/project/go.mod", want: "Go"},
		{entity: "/tmp/project/CMakeLists.txt", want: "CMake"},
		{entity: "/tmp/project/component.vue", want: "Vue"},
	} {
		heartbeat := Heartbeat{Entity: test.entity, Time: 123}
		PrepareHeartbeat(&heartbeat, HeartbeatDefaults{})
		if heartbeat.Language != test.want {
			t.Fatalf("expected %s language for %s, got %q", test.want, test.entity, heartbeat.Language)
		}
	}
}

func TestValidateHeartbeatAtRejectsOutOfRangeTime(t *testing.T) {
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	heartbeat := Heartbeat{Entity: "/tmp/main.go", Type: "file", Time: float64(now.AddDate(-1, 0, -1).Unix())}

	if err := ValidateHeartbeatAt(heartbeat, now); err == nil {
		t.Fatal("expected old heartbeat to be rejected")
	}
}

func TestHeartbeatAcceptsWakaTimeDependencyArray(t *testing.T) {
	var heartbeat Heartbeat

	if err := json.Unmarshal([]byte(`{"entity":"/tmp/main.go","type":"file","time":1781887600,"dependencies":["pgx","echo"]}`), &heartbeat); err != nil {
		t.Fatalf("expected WakaTime dependency array to decode, got %v", err)
	}

	if heartbeat.Dependencies != "pgx,echo" {
		t.Fatalf("expected dependencies to be normalized, got %q", heartbeat.Dependencies)
	}
}

func TestHeartbeatUsesWakaTimeMachineIDAsFallbackName(t *testing.T) {
	var heartbeat Heartbeat

	if err := json.Unmarshal([]byte(`{"entity":"/tmp/main.go","type":"file","time":1781887600,"machine_name_id":"564c7884-b78e-4005-a975-c87c991a6fdc"}`), &heartbeat); err != nil {
		t.Fatalf("expected WakaTime machine id heartbeat to decode, got %v", err)
	}

	if heartbeat.MachineName != "wakatime-564c7884" {
		t.Fatalf("expected machine fallback label, got %q", heartbeat.MachineName)
	}
}

func TestHeartbeatUsesAlternateProjectWhenProjectMissing(t *testing.T) {
	var heartbeat Heartbeat

	if err := json.Unmarshal([]byte(`{"entity":"/tmp/main.go","type":"file","time":1781887600,"alternate_project":"cli-project"}`), &heartbeat); err != nil {
		t.Fatalf("expected alternate project heartbeat to decode, got %v", err)
	}

	if heartbeat.Project != "cli-project" {
		t.Fatalf("expected alternate_project to fill project, got %q", heartbeat.Project)
	}
}

func TestHeartbeatAcceptsExtendedAITelemetryAliasesAndRawPayload(t *testing.T) {
	var heartbeat Heartbeat

	if err := json.Unmarshal([]byte(`{
		"entity":"/tmp/main.go",
		"type":"file",
		"time":1781887600,
		"model_name":"gpt-5.5-codex",
		"llm_provider":"openai",
		"metadata":{"request_id":"req_123","source":"stint-cli"},
		"ai_input_tokens":1200
	}`), &heartbeat); err != nil {
		t.Fatalf("expected extended AI telemetry heartbeat to decode, got %v", err)
	}

	if heartbeat.AIModel != "gpt-5.5-codex" {
		t.Fatalf("expected model_name alias to fill ai_model, got %q", heartbeat.AIModel)
	}
	if heartbeat.AIProvider != "openai" {
		t.Fatalf("expected llm_provider alias to fill ai_provider, got %q", heartbeat.AIProvider)
	}
	if heartbeat.Metadata["request_id"] != "req_123" || heartbeat.Metadata["source"] != "stint-cli" {
		t.Fatalf("expected metadata to be preserved, got %#v", heartbeat.Metadata)
	}
	if heartbeat.RawPayload["model_name"] != "gpt-5.5-codex" || heartbeat.RawPayload["llm_provider"] != "openai" {
		t.Fatalf("expected raw payload to preserve original aliases, got %#v", heartbeat.RawPayload)
	}
}

func TestPrepareHeartbeatInfersAIProviderFromAgentOrModel(t *testing.T) {
	heartbeat := Heartbeat{Entity: "/tmp/main.go", Time: 123, AIModel: "claude-sonnet-4.5", AIAgent: "claude"}

	PrepareHeartbeat(&heartbeat, HeartbeatDefaults{})

	if heartbeat.AIProvider != "anthropic" {
		t.Fatalf("expected Anthropic provider inference, got %q", heartbeat.AIProvider)
	}
}
