package stintcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPluginInstallClaudeRunsMarketplaceCommandsAndWritesConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)
	t.Setenv("STINT_API_URL", "https://stint.example.com/api/v1")
	t.Setenv("STINT_API_KEY", "waka_plugin")

	oldLookPath := pluginLookPath
	pluginLookPath = func(name string) (string, error) {
		if name == "claude" {
			return "/usr/bin/claude", nil
		}
		return "", os.ErrNotExist
	}
	var commands [][]string
	oldRun := pluginRunCommand
	pluginRunCommand = func(name string, args ...string) error {
		commands = append(commands, append([]string{name}, args...))
		return nil
	}
	t.Cleanup(func() {
		pluginLookPath = oldLookPath
		pluginRunCommand = oldRun
	})

	var out bytes.Buffer
	if err := Run([]string{"plugin", "install", "claude-code"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 2 {
		t.Fatalf("commands = %#v", commands)
	}
	if strings.Join(commands[0], " ") != "/usr/bin/claude plugin marketplace add https://github.com/wakatime/claude-code-wakatime.git" {
		t.Fatalf("unexpected marketplace command: %#v", commands[0])
	}
	if strings.Join(commands[1], " ") != "/usr/bin/claude plugin i claude-code-wakatime@wakatime" {
		t.Fatalf("unexpected install command: %#v", commands[1])
	}
	cfg, err := LoadConfig(DefaultWakaTimeConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Get("settings", "api_url") != "https://stint.example.com/api/v1" || cfg.Get("settings", "api_key") != "waka_plugin" {
		t.Fatalf("unexpected config: %#v", cfg.Section("settings"))
	}
	if !strings.Contains(out.String(), "claude-code plugin installed") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestPluginInstallCodexAndCopilotUseMarketplaceRows(t *testing.T) {
	for _, tc := range []struct {
		agent   string
		binary  string
		command string
	}{
		{"codex", "codex", "/usr/bin/codex plugin marketplace add wakatime/codex-cli-wakatime|/usr/bin/codex plugin add codex-cli-wakatime@wakatime"},
		{"copilot", "copilot", "/usr/bin/copilot plugin marketplace add wakatime/copilot-cli-wakatime|/usr/bin/copilot plugin install copilot-cli-wakatime@wakatime"},
	} {
		t.Run(tc.agent, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("WAKATIME_HOME", home)
			t.Setenv("STINT_API_URL", "https://stint.example.com/api/v1")
			t.Setenv("STINT_API_KEY", "waka_plugin")

			oldLookPath := pluginLookPath
			pluginLookPath = func(name string) (string, error) {
				if name == tc.binary {
					return "/usr/bin/" + name, nil
				}
				return "", os.ErrNotExist
			}
			var commands []string
			oldRun := pluginRunCommand
			pluginRunCommand = func(name string, args ...string) error {
				commands = append(commands, strings.Join(append([]string{name}, args...), " "))
				return nil
			}
			t.Cleanup(func() {
				pluginLookPath = oldLookPath
				pluginRunCommand = oldRun
			})

			if err := Run([]string{"plugin", "install", tc.agent}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			if strings.Join(commands, "|") != tc.command {
				t.Fatalf("commands = %#v", commands)
			}
		})
	}
}

func TestPluginInstallAntigravityUsesAgyPluginInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)
	t.Setenv("STINT_API_URL", "https://stint.example.com/api/v1")
	t.Setenv("STINT_API_KEY", "waka_plugin")

	oldLookPath := pluginLookPath
	pluginLookPath = func(name string) (string, error) {
		if name == "agy" {
			return "/usr/bin/agy", nil
		}
		return "", os.ErrNotExist
	}
	var command string
	oldRun := pluginRunCommand
	pluginRunCommand = func(name string, args ...string) error {
		command = strings.Join(append([]string{name}, args...), " ")
		return nil
	}
	t.Cleanup(func() {
		pluginLookPath = oldLookPath
		pluginRunCommand = oldRun
	})

	if err := Run([]string{"plugin", "install", "antigravity"}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if command != "/usr/bin/agy plugin install https://github.com/wakatime/antigravity-cli-wakatime" {
		t.Fatalf("command = %q", command)
	}
}

func TestPluginInstallAmpDownloadsPluginFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)
	t.Setenv("STINT_API_URL", "https://stint.example.com/api/v1")
	t.Setenv("STINT_API_KEY", "waka_plugin")

	oldLookPath := pluginLookPath
	pluginLookPath = func(name string) (string, error) {
		if name == "amp" {
			return "/usr/bin/amp", nil
		}
		return "", os.ErrNotExist
	}
	oldDownload := downloadFile
	oldAmpPluginURL := ampPluginURL
	oldAmpPluginSHA256 := ampPluginSHA256
	pluginData := []byte("// amp plugin\n")
	ampPluginURL = "https://example.test/amp-cli-wakatime.ts"
	ampPluginSHA256 = testSHA256(pluginData)
	downloadFile = func(url string) ([]byte, error) {
		if url != ampPluginURL {
			t.Fatalf("url = %q", url)
		}
		return pluginData, nil
	}
	t.Cleanup(func() {
		pluginLookPath = oldLookPath
		downloadFile = oldDownload
		ampPluginURL = oldAmpPluginURL
		ampPluginSHA256 = oldAmpPluginSHA256
	})

	if err := Run([]string{"plugin", "install", "amp"}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(home, ".config", "amp", "plugins", "amp-cli-wakatime.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "// amp plugin\n" {
		t.Fatalf("plugin file = %q", got)
	}
}

func TestPluginInstallMissingHostFailsClearly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)
	t.Setenv("STINT_API_URL", "https://stint.example.com/api/v1")
	t.Setenv("STINT_API_KEY", "waka_plugin")
	oldLookPath := pluginLookPath
	pluginLookPath = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { pluginLookPath = oldLookPath })

	err := Run([]string{"plugin", "install", "codex"}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "codex is not installed or not on PATH") {
		t.Fatalf("expected missing host error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".wakatime.cfg")); !os.IsNotExist(err) {
		t.Fatalf("missing host should not write config, stat err=%v", err)
	}
}

func TestPluginInstallCommandFailureDoesNotWriteConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)
	t.Setenv("STINT_API_URL", "https://stint.example.com/api/v1")
	t.Setenv("STINT_API_KEY", "waka_plugin")

	oldLookPath := pluginLookPath
	pluginLookPath = func(name string) (string, error) {
		if name == "claude" {
			return "/usr/bin/claude", nil
		}
		return "", os.ErrNotExist
	}
	oldRun := pluginRunCommand
	pluginRunCommand = func(string, ...string) error {
		return os.ErrPermission
	}
	t.Cleanup(func() {
		pluginLookPath = oldLookPath
		pluginRunCommand = oldRun
	})

	err := Run([]string{"plugin", "install", "claude-code"}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "claude-code plugin command failed") {
		t.Fatalf("expected plugin command failure, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".wakatime.cfg")); !os.IsNotExist(err) {
		t.Fatalf("failed plugin install should not write config, stat err=%v", err)
	}
}

func TestPluginInstallUnknownAgentListsSupportedAgents(t *testing.T) {
	err := Run([]string{"plugin", "install", "unknown"}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "supported agents") {
		t.Fatalf("expected supported agent list, got %v", err)
	}
}

func TestPluginRunCommandIncludesStderrOnFailure(t *testing.T) {
	name := "sh"
	args := []string{"-c", "printf 'plugin command rejected' >&2; exit 42"}
	if runtime.GOOS == "windows" {
		name = "cmd"
		args = []string{"/C", "echo plugin command rejected 1>&2 && exit /b 42"}
	}
	err := pluginRunCommand(name, args...)
	if err == nil || !strings.Contains(err.Error(), "plugin command rejected") {
		t.Fatalf("expected stderr in plugin command failure, got %v", err)
	}
}

func TestInstallAmpPluginRejectsChecksumMismatch(t *testing.T) {
	oldDownload := downloadFile
	oldAmpPluginURL := ampPluginURL
	oldAmpPluginSHA256 := ampPluginSHA256
	ampPluginURL = "https://example.test/amp-cli-wakatime.ts"
	ampPluginSHA256 = strings.Repeat("0", 64)
	downloadFile = func(url string) ([]byte, error) {
		if url != ampPluginURL {
			t.Fatalf("url = %q", url)
		}
		return []byte("// tampered plugin\n"), nil
	}
	t.Cleanup(func() {
		downloadFile = oldDownload
		ampPluginURL = oldAmpPluginURL
		ampPluginSHA256 = oldAmpPluginSHA256
	})

	err := installAmpPlugin(PluginSpec{}, "/usr/bin/amp")
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func testSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
