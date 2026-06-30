package stintcli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestAPIKeyVaultCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-only")
	}
	dir := t.TempDir()
	vault := filepath.Join(dir, "vault.sh")
	if err := os.WriteFile(vault, []byte("#!/bin/sh\necho waka_vault\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "api_key_vault_cmd", vault)
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{"--config", config})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_vault" {
		t.Fatalf("APIKey = %q", opts.APIKey)
	}
}

func TestAPIKeyVaultCommandRunsThroughShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command fixture is unix-only")
	}
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "api_key_vault_cmd", `printf 'waka_%s' shell`)
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{"--config", config})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_shell" {
		t.Fatalf("APIKey = %q", opts.APIKey)
	}
}

func TestAPIKeyConfigWinsOverVaultCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command fixture is unix-only")
	}
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "api_key", "waka_config")
	cfg.Set("settings", "api_key_vault_cmd", "exit 7")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	opts, err := parseCommon([]string{"--config", config})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIKey != "waka_config" {
		t.Fatalf("APIKey = %q", opts.APIKey)
	}
}

func TestAPIKeyVaultCommandFailureReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command fixture is unix-only")
	}
	dir := t.TempDir()
	config := filepath.Join(dir, ".wakatime.cfg")
	cfg := Config{Sections: map[string]map[string]string{}}
	cfg.Set("settings", "api_key_vault_cmd", "exit 7")
	if err := cfg.Write(config); err != nil {
		t.Fatal(err)
	}
	_, err := parseCommon([]string{"--config", config})
	if err == nil || !strings.Contains(err.Error(), "failed to read api key from vault") {
		t.Fatalf("expected vault command error, got %v", err)
	}
}
