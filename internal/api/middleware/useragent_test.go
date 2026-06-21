package middleware

import "testing"

func TestParseWakaTimeUserAgent(t *testing.T) {
	ua := "wakatime/v1.102.1 (darwin-arm64) go1.22.0 vscode/1.89.0 vscode-wakatime/24.3.0"

	got := ParseUserAgent(ua)

	if got.Plugin != "wakatime" {
		t.Fatalf("expected plugin wakatime, got %q", got.Plugin)
	}
	if got.PluginVersion != "v1.102.1" {
		t.Fatalf("expected plugin version v1.102.1, got %q", got.PluginVersion)
	}
	if got.OperatingSystem != "darwin" {
		t.Fatalf("expected OS darwin, got %q", got.OperatingSystem)
	}
	if got.Architecture != "arm64" {
		t.Fatalf("expected arch arm64, got %q", got.Architecture)
	}
	if got.Editor != "vscode" {
		t.Fatalf("expected editor vscode, got %q", got.Editor)
	}
	if got.EditorVersion != "1.89.0" {
		t.Fatalf("expected editor version 1.89.0, got %q", got.EditorVersion)
	}
}

func TestParseUserAgentHandlesUnknown(t *testing.T) {
	got := ParseUserAgent("")
	if got.Editor != "Unknown" {
		t.Fatalf("expected unknown editor fallback, got %q", got.Editor)
	}
	if got.OperatingSystem != "Unknown" {
		t.Fatalf("expected unknown OS fallback, got %q", got.OperatingSystem)
	}
}

func TestParseUserAgentExtractsArchitectureFromKernelRichLinuxToken(t *testing.T) {
	ua := "wakatime/v1.104.0 (linux-6.8.0-110-generic-x86_64) go1.25.0 cursor/1.2.3 cursor-wakatime/1.0.0"

	got := ParseUserAgent(ua)

	if got.OperatingSystem != "linux" {
		t.Fatalf("expected OS linux, got %q", got.OperatingSystem)
	}
	if got.Architecture != "x86_64" {
		t.Fatalf("expected architecture x86_64, got %q", got.Architecture)
	}
	if got.Editor != "cursor" {
		t.Fatalf("expected editor cursor, got %q", got.Editor)
	}
}

func TestParseUserAgentRecognizesCodexPlugin(t *testing.T) {
	ua := "wakatime/v2.20.3 (darwin-25.3.0-arm64) go1.26.4 codex/1.0.0 codex-wakatime/1.3.1"

	got := ParseUserAgent(ua)

	if got.Editor != "codex" {
		t.Fatalf("expected editor codex, got %q", got.Editor)
	}
	if got.EditorVersion != "1.0.0" {
		t.Fatalf("expected codex editor version 1.0.0, got %q", got.EditorVersion)
	}
	if got.OperatingSystem != "darwin" {
		t.Fatalf("expected OS darwin, got %q", got.OperatingSystem)
	}
	if got.Architecture != "arm64" {
		t.Fatalf("expected architecture arm64, got %q", got.Architecture)
	}
}

func TestParseUserAgentExtractsAIAgentToken(t *testing.T) {
	ua := "wakatime/v2.20.4 (darwin-25.3.0-arm64) go1.26.4 gpt/5.5 codex-cli/0.141.0 codex/1.0.0 codex-wakatime/1.3.1"

	got := ParseUserAgent(ua)

	if got.AIAgent != "gpt" {
		t.Fatalf("expected AI agent gpt, got %q", got.AIAgent)
	}
	if got.AIAgentVersion != "5.5" {
		t.Fatalf("expected AI agent version 5.5, got %q", got.AIAgentVersion)
	}
	if got.Editor != "codex" {
		t.Fatalf("expected editor codex, got %q", got.Editor)
	}
}
