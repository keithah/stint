package stintcli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSetupWritesStintAndWakaTimeConfigsPreservingExistingKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)

	wakaPath := filepath.Join(home, ".wakatime.cfg")
	existing := Config{Sections: map[string]map[string]string{}}
	existing.Set("settings", "debug", "true")
	existing.Set("settings", "exclude", "vendor")
	if err := existing.Write(wakaPath); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := Run([]string{"setup", "--server", "https://stint.example.com/api/v1", "--key", "waka_new"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	stintCfg, err := LoadConfig(DefaultStintConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if stintCfg.Get("settings", "api_url") != "https://stint.example.com/api/v1" || stintCfg.Get("settings", "api_key") != "waka_new" {
		t.Fatalf("unexpected stint config: %#v", stintCfg.Section("settings"))
	}
	wakaCfg, err := LoadConfig(wakaPath)
	if err != nil {
		t.Fatal(err)
	}
	settings := wakaCfg.Section("settings")
	if settings["api_url"] != "https://stint.example.com/api/v1" || settings["api_key"] != "waka_new" {
		t.Fatalf("unexpected wakatime config: %#v", settings)
	}
	if settings["debug"] != "true" || settings["exclude"] != "vendor" {
		t.Fatalf("setup did not preserve existing keys: %#v", settings)
	}
	backups, err := filepath.Glob(filepath.Join(home, ".wakatime.cfg.bak-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 0 {
		t.Fatalf("setup left backup files behind: %#v", backups)
	}
	if !strings.Contains(out.String(), "wrote") {
		t.Fatalf("expected setup summary, got %q", out.String())
	}
}

func TestParseCommonUsesStintConfigBeforeWakaTimeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)

	stintCfg := Config{Sections: map[string]map[string]string{}}
	stintCfg.Set("settings", "api_url", "https://native.example.com/api/v1")
	stintCfg.Set("settings", "api_key", "waka_native")
	if err := stintCfg.Write(DefaultStintConfigPath()); err != nil {
		t.Fatal(err)
	}
	wakaCfg := Config{Sections: map[string]map[string]string{}}
	wakaCfg.Set("settings", "api_url", "https://waka.example.com/api/v1")
	wakaCfg.Set("settings", "api_key", "waka_fallback")
	if err := wakaCfg.Write(DefaultWakaTimeConfigPath()); err != nil {
		t.Fatal(err)
	}

	opts, err := parseCommon(nil)
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIURL != "https://native.example.com/api/v1" || opts.APIKey != "waka_native" {
		t.Fatalf("expected native config to win, got api_url=%q api_key=%q", opts.APIURL, opts.APIKey)
	}
}

func TestParseCommonUsesEnvBeforeStintConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)
	t.Setenv("STINT_API_URL", "https://env.example.com/api/v1")
	t.Setenv("STINT_API_KEY", "waka_env")

	stintCfg := Config{Sections: map[string]map[string]string{}}
	stintCfg.Set("settings", "api_url", "https://native.example.com/api/v1")
	stintCfg.Set("settings", "api_key", "waka_native")
	if err := stintCfg.Write(DefaultStintConfigPath()); err != nil {
		t.Fatal(err)
	}

	opts, err := parseCommon(nil)
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIURL != "https://env.example.com/api/v1" || opts.APIKey != "waka_env" {
		t.Fatalf("expected env to win, got api_url=%q api_key=%q", opts.APIURL, opts.APIKey)
	}
}

func TestParseCommonErrorsWhenNativeConfigCannotBeRead(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission fixture is not meaningful as root")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)
	if err := os.WriteFile(DefaultStintConfigPath(), []byte("[settings]\napi_key = waka_native\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(DefaultStintConfigPath(), 0o600) })

	_, err := parseCommon(nil)
	if err == nil || !strings.Contains(err.Error(), "load native config") {
		t.Fatalf("expected native config load error, got %v", err)
	}
}

func TestParseCommonFallsBackToWakaTimeConfigWhenNativeVaultFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)

	native := Config{Sections: map[string]map[string]string{}}
	native.Set("settings", "api_key_vault_cmd", "exit 9")
	if err := native.Write(DefaultStintConfigPath()); err != nil {
		t.Fatal(err)
	}
	waka := Config{Sections: map[string]map[string]string{}}
	waka.Set("settings", "api_key", "waka_fallback")
	if err := waka.Write(DefaultWakaTimeConfigPath()); err != nil {
		t.Fatal(err)
	}

	opts, err := parseCommon(nil)
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_fallback" {
		t.Fatalf("expected wakatime fallback key, got %q", opts.APIKey)
	}
}

func TestSetupDoesNotUpdateNativeConfigWhenWakaTimeWriteCannotBePrepared(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission fixture is not meaningful as root")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)

	original := Config{Sections: map[string]map[string]string{}}
	original.Set("settings", "api_url", "https://old.example.com/api/v1")
	original.Set("settings", "api_key", "waka_old")
	if err := original.Write(DefaultStintConfigPath()); err != nil {
		t.Fatal(err)
	}
	blockedDir := filepath.Join(home, "blocked")
	if err := os.Mkdir(blockedDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blockedDir, 0o700) })

	err := Run([]string{
		"setup",
		"--server", "https://new.example.com/api/v1",
		"--key", "waka_new",
		"--wakatime-config", filepath.Join(blockedDir, ".wakatime.cfg"),
	}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected wakatime config write error")
	}
	cfg, loadErr := LoadConfig(DefaultStintConfigPath())
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if cfg.Get("settings", "api_url") != "https://old.example.com/api/v1" || cfg.Get("settings", "api_key") != "waka_old" {
		t.Fatalf("native config was partially updated: %#v", cfg.Section("settings"))
	}
}

func TestSetupHonorsEnvironmentFallbacks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)
	t.Setenv("STINT_API_URL", "https://env.example.com/api/v1")
	t.Setenv("STINT_API_KEY", "waka_env")

	if err := Run([]string{"setup"}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(DefaultStintConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Get("settings", "api_url") != "https://env.example.com/api/v1" || cfg.Get("settings", "api_key") != "waka_env" {
		t.Fatalf("unexpected config from env: %#v", cfg.Section("settings"))
	}
}

func TestSetupRequiresServerAndKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)

	err := Run([]string{"setup"}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "server and key are required") {
		t.Fatalf("expected missing credentials error, got %v", err)
	}
	if _, statErr := os.Stat(DefaultStintConfigPath()); !os.IsNotExist(statErr) {
		t.Fatalf("setup should not write incomplete config, stat err=%v", statErr)
	}
}
