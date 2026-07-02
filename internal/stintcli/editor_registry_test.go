package stintcli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEditorRegistryDetectsInstalledEditorsFromPath(t *testing.T) {
	oldLookPath := editorLookPath
	editorLookPath = func(name string) (string, error) {
		if name == "code" || name == "nvim" {
			return "/usr/bin/" + name, nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { editorLookPath = oldLookPath })

	reg := DefaultEditorRegistry()
	ids := reg.DetectInstalled()
	if !containsString(ids, "vscode") || !containsString(ids, "neovim") {
		t.Fatalf("expected vscode and neovim, got %#v", ids)
	}
	if containsString(ids, "zed") {
		t.Fatalf("did not expect zed without binary, got %#v", ids)
	}
}

func TestConnectConfiguresDetectedEditorsByWritingWakaTimeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)
	t.Setenv("STINT_API_URL", "https://stint.example.com/api/v1")
	t.Setenv("STINT_API_KEY", "waka_connect")

	oldLookPath := editorLookPath
	editorLookPath = func(name string) (string, error) {
		if name == "code" {
			return "/usr/bin/code", nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { editorLookPath = oldLookPath })

	existing := Config{Sections: map[string]map[string]string{}}
	existing.Set("settings", "debug", "true")
	if err := existing.Write(DefaultWakaTimeConfigPath()); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := Run([]string{"connect"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(DefaultWakaTimeConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Get("settings", "api_url") != "https://stint.example.com/api/v1" || cfg.Get("settings", "api_key") != "waka_connect" {
		t.Fatalf("unexpected wakatime config: %#v", cfg.Section("settings"))
	}
	if cfg.Get("settings", "debug") != "true" {
		t.Fatalf("connect should preserve existing keys: %#v", cfg.Section("settings"))
	}
	if !strings.Contains(out.String(), "vscode configured") {
		t.Fatalf("expected connect summary, got %q", out.String())
	}
}

func TestConnectReadsCredentialsFromImportedWakaTimeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)

	oldLookPath := editorLookPath
	editorLookPath = func(name string) (string, error) {
		if name == "code" {
			return "/usr/bin/code", nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { editorLookPath = oldLookPath })

	importedPath := filepath.Join(home, "imported.cfg")
	imported := Config{Sections: map[string]map[string]string{}}
	imported.Set("settings", "api_url", "https://imported.example.com/api/v1")
	imported.Set("settings", "api_key", "waka_imported")
	if err := imported.Write(importedPath); err != nil {
		t.Fatal(err)
	}
	main := Config{Sections: map[string]map[string]string{}}
	main.Set("settings", "import_cfg", importedPath)
	if err := main.Write(DefaultWakaTimeConfigPath()); err != nil {
		t.Fatal(err)
	}

	if err := Run([]string{"connect"}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(DefaultWakaTimeConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Get("settings", "api_url") != "https://imported.example.com/api/v1" || cfg.Get("settings", "api_key") != "waka_imported" {
		t.Fatalf("unexpected wakatime config after connect: %#v", cfg.Section("settings"))
	}
	if cfg.Get("settings", "import_cfg") != importedPath {
		t.Fatalf("connect should preserve import_cfg, got %#v", cfg.Section("settings"))
	}
}

func TestConnectFailsClearlyWhenNoCredentialsAreAvailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)

	oldLookPath := editorLookPath
	editorLookPath = func(name string) (string, error) {
		if name == "code" {
			return "/usr/bin/code", nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { editorLookPath = oldLookPath })

	err := Run([]string{"connect"}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "STINT_API_URL and STINT_API_KEY") {
		t.Fatalf("expected setup guidance, got %v", err)
	}
}

func TestConnectDeepInstallsVSCodeExtensionOnlyForSupportedEditors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)
	t.Setenv("STINT_API_URL", "https://stint.example.com/api/v1")
	t.Setenv("STINT_API_KEY", "waka_connect")

	oldLookPath := editorLookPath
	editorLookPath = func(name string) (string, error) {
		switch name {
		case "code":
			return "/usr/bin/code", nil
		case "nvim":
			return "/usr/bin/nvim", nil
		default:
			return "", os.ErrNotExist
		}
	}
	var commands [][]string
	oldRun := editorRunCommand
	editorRunCommand = func(name string, args ...string) error {
		commands = append(commands, append([]string{name}, args...))
		return nil
	}
	t.Cleanup(func() {
		editorLookPath = oldLookPath
		editorRunCommand = oldRun
	})

	var out bytes.Buffer
	if err := Run([]string{"connect", "--deep"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || commands[0][0] != "/usr/bin/code" || !containsString(commands[0], "--install-extension") {
		t.Fatalf("unexpected deep install commands: %#v", commands)
	}
	if !strings.Contains(out.String(), "neovim configured") {
		t.Fatalf("expected neovim default configure summary, got %q", out.String())
	}
}

func TestConnectDeepInstallsJetBrainsPlugin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)
	t.Setenv("STINT_API_URL", "https://stint.example.com/api/v1")
	t.Setenv("STINT_API_KEY", "waka_connect")

	oldLookPath := editorLookPath
	editorLookPath = func(name string) (string, error) {
		if name == "idea" {
			return "/usr/bin/idea", nil
		}
		return "", os.ErrNotExist
	}
	var commands [][]string
	oldRun := editorRunCommand
	editorRunCommand = func(name string, args ...string) error {
		commands = append(commands, append([]string{name}, args...))
		return nil
	}
	t.Cleanup(func() {
		editorLookPath = oldLookPath
		editorRunCommand = oldRun
	})

	if err := Run([]string{"connect", "--deep"}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	want := []string{"/usr/bin/idea", "installPlugins", "com.wakatime.intellij.plugin"}
	if len(commands) != 1 || strings.Join(commands[0], " ") != strings.Join(want, " ") {
		t.Fatalf("unexpected JetBrains command: %#v", commands)
	}
}

func TestConnectDeepContinuesAfterExtensionInstallFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)
	t.Setenv("STINT_API_URL", "https://stint.example.com/api/v1")
	t.Setenv("STINT_API_KEY", "waka_connect")

	oldLookPath := editorLookPath
	editorLookPath = func(name string) (string, error) {
		switch name {
		case "code":
			return "/usr/bin/code", nil
		case "cursor":
			return "/usr/bin/cursor", nil
		case "nvim":
			return "/usr/bin/nvim", nil
		default:
			return "", os.ErrNotExist
		}
	}
	var commands [][]string
	oldRun := editorRunCommand
	editorRunCommand = func(name string, args ...string) error {
		commands = append(commands, append([]string{name}, args...))
		if name == "/usr/bin/code" {
			return fmt.Errorf("marketplace unavailable")
		}
		return nil
	}
	t.Cleanup(func() {
		editorLookPath = oldLookPath
		editorRunCommand = oldRun
	})

	var out bytes.Buffer
	err := Run([]string{"connect", "--deep"}, nil, &out, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "vscode: marketplace unavailable") {
		t.Fatalf("expected aggregated deep install failure, got %v", err)
	}
	if len(commands) != 2 {
		t.Fatalf("expected connect to continue after the first deep install failure, commands=%#v", commands)
	}
	output := out.String()
	if !strings.Contains(output, "vscode configured; extension install failed") ||
		!strings.Contains(output, "cursor configured; extension installed") ||
		!strings.Contains(output, "neovim configured") {
		t.Fatalf("unexpected connect summary: %q", output)
	}
}

func TestInstallJetBrainsWakaTimeAttemptsEveryDetectedLauncher(t *testing.T) {
	oldLookPath := editorLookPath
	editorLookPath = func(name string) (string, error) {
		switch name {
		case "idea", "pycharm", "studio":
			return "/usr/bin/" + name, nil
		default:
			return "", os.ErrNotExist
		}
	}
	var commands [][]string
	oldRun := editorRunCommand
	editorRunCommand = func(name string, args ...string) error {
		commands = append(commands, append([]string{name}, args...))
		if name == "/usr/bin/idea" {
			return fmt.Errorf("install failed")
		}
		return nil
	}
	t.Cleanup(func() {
		editorLookPath = oldLookPath
		editorRunCommand = oldRun
	})

	spec := DefaultEditorRegistry()["jetbrains"].Spec
	if err := installJetBrainsWakaTime(spec); err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(commands))
	for _, command := range commands {
		got = append(got, command[0])
	}
	for _, want := range []string{"/usr/bin/idea", "/usr/bin/pycharm", "/usr/bin/studio"} {
		if !containsString(got, want) {
			t.Fatalf("expected JetBrains install command for %s, got %#v", want, commands)
		}
	}
}

func TestEditorRunCommandIncludesStderrOnFailure(t *testing.T) {
	name := "sh"
	args := []string{"-c", "printf 'extension not found' >&2; exit 42"}
	if runtime.GOOS == "windows" {
		name = "cmd"
		args = []string{"/C", "echo extension not found 1>&2 && exit /b 42"}
	}
	err := editorRunCommand(name, args...)
	if err == nil || !strings.Contains(err.Error(), "extension not found") {
		t.Fatalf("expected stderr in command failure, got %v", err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestConnectAllHandlesNoDetectedEditors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)
	t.Setenv("STINT_API_URL", "https://stint.example.com/api/v1")
	t.Setenv("STINT_API_KEY", "waka_connect")
	oldLookPath := editorLookPath
	editorLookPath = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { editorLookPath = oldLookPath })

	var out bytes.Buffer
	if err := Run([]string{"connect"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no supported editors detected") {
		t.Fatalf("unexpected output: %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".wakatime.cfg")); !os.IsNotExist(err) {
		t.Fatalf("connect should not write config when no editors are detected, stat err=%v", err)
	}
}
