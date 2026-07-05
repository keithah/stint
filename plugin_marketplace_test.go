package stint_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestClaudeCodeStintMarketplacePlugin(t *testing.T) {
	marketplace := readJSONFile(t, ".claude-plugin/marketplace.json")
	source := marketplace["plugins"].([]any)[0].(map[string]any)["source"].(string)
	if source != "./plugins/claude-code-stint" {
		t.Fatalf("claude marketplace source = %q", source)
	}

	plugin := readJSONFile(t, "plugins/claude-code-stint/.claude-plugin/plugin.json")
	if plugin["name"] != "claude-code-stint" {
		t.Fatalf("claude plugin name = %#v", plugin["name"])
	}

	hooksPath := "plugins/claude-code-stint/hooks/hooks.json"
	hooks := mustReadText(t, hooksPath)
	for _, want := range []string{"SessionEnd", "UserPromptSubmit", "CLAUDE_PLUGIN_ROOT"} {
		if !strings.Contains(hooks, want) {
			t.Fatalf("claude hooks missing %q", want)
		}
	}
	for _, noisy := range []string{"PreToolUse", "PostToolUse", "PreCompact", "SubagentStop"} {
		if strings.Contains(hooks, noisy) {
			t.Fatalf("claude hooks should not run on noisy event %q", noisy)
		}
	}

	assertPluginPathTracked(t, hooksPath)
	assertPluginPathTracked(t, "plugins/claude-code-stint/scripts/run")
	assertExecutable(t, "plugins/claude-code-stint/scripts/run")
	assertUsesSharedRunner(t, "plugins/claude-code-stint/scripts/run", "claude-code-stint", "claude")
}

func TestCodexCliStintMarketplacePlugin(t *testing.T) {
	marketplace := readJSONFile(t, ".agents/plugins/marketplace.json")
	pluginEntry := marketplace["plugins"].([]any)[0].(map[string]any)
	if pluginEntry["name"] != "codex-cli-stint" {
		t.Fatalf("codex marketplace plugin name = %#v", pluginEntry["name"])
	}
	source := pluginEntry["source"].(map[string]any)
	if source["path"] != "./plugins/codex-cli-stint" {
		t.Fatalf("codex marketplace path = %#v", source["path"])
	}

	plugin := readJSONFile(t, "plugins/codex-cli-stint/.codex-plugin/plugin.json")
	if plugin["name"] != "codex-cli-stint" {
		t.Fatalf("codex plugin name = %#v", plugin["name"])
	}

	hooksPath := "plugins/codex-cli-stint/hooks/hooks.json"
	hooks := mustReadText(t, hooksPath)
	for _, want := range []string{"SessionEnd", "UserPromptSubmit", "PLUGIN_ROOT"} {
		if !strings.Contains(hooks, want) {
			t.Fatalf("codex hooks missing %q", want)
		}
	}
	for _, noisy := range []string{"SessionStart", "PostToolUse", "PreToolUse"} {
		if strings.Contains(hooks, noisy) {
			t.Fatalf("codex hooks should not run on noisy event %q", noisy)
		}
	}

	assertPluginPathTracked(t, hooksPath)
	assertPluginPathTracked(t, "plugins/codex-cli-stint/scripts/run")
	assertExecutable(t, "plugins/codex-cli-stint/scripts/run")
	assertUsesSharedRunner(t, "plugins/codex-cli-stint/scripts/run", "codex-cli-stint", "codex")
}

func TestSharedMarketplacePluginRunner(t *testing.T) {
	runnerPath := "plugins/shared/stint-plugin-runner.js"
	runner := mustReadText(t, runnerPath)
	assertPluginPathTracked(t, runnerPath)
	for _, want := range []string{"--sync-ai-activity", "--ai-agent", "STINT_PLUGIN_AGENT", "STINT_PLUGIN_AUTO_INSTALL", "STINT_PLUGIN_MIN_SYNC_SECONDS"} {
		if !strings.Contains(runner, want) {
			t.Fatalf("shared plugin runner missing %q", want)
		}
	}
	if strings.Contains(runner, "curl -fsSL https://stint.fyi/install.sh | sh") {
		t.Fatalf("shared plugin runner must not auto-install Stint without an explicit opt-in")
	}
}

func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	body := []byte(mustReadText(t, path))
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return out
}

func mustReadText(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

func assertPluginPathTracked(t *testing.T, path string) {
	t.Helper()
	if out, err := exec.Command("git", "check-ignore", path).CombinedOutput(); err == nil {
		t.Fatalf("%s is ignored by git:\n%s", path, out)
	}
	if out, err := exec.Command("git", "ls-files", "--error-unmatch", path).CombinedOutput(); err != nil {
		t.Fatalf("%s is not tracked by git:\n%s", path, out)
	}
}

func assertExecutable(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("%s must be executable; mode is %v", path, info.Mode().Perm())
	}
}

func assertUsesSharedRunner(t *testing.T, path, pluginName, agent string) {
	t.Helper()
	body := mustReadText(t, path)
	for _, want := range []string{"shared/stint-plugin-runner.js", "STINT_PLUGIN_NAME=" + pluginName, "STINT_PLUGIN_AGENT=" + agent} {
		if !strings.Contains(body, want) {
			t.Fatalf("%s missing %q", path, want)
		}
	}
}
