package stintcli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestDiagnosticOptionsParsesCollectFlags(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "wakatime.log")
	opts, err := diagnosticOptions([]string{
		"collect",
		"--api-url", "http://collector.example/api/v1",
		"--key", "waka_collect",
		"--log-file", logFile,
		"--verbose",
		"--send-diagnostics-on-errors",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.APIURL != "http://collector.example/api/v1" || opts.APIKey != "waka_collect" || opts.LogFile != logFile {
		t.Fatalf("unexpected collect diagnostic options: %#v", opts)
	}
	if !opts.Verbose || !opts.SendDiagnosticsOnError {
		t.Fatalf("collect diagnostic flags were not parsed: %#v", opts)
	}
}

func TestCollectDelegatesToStintCollectOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script PATH fixture is unix-only")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "stint-collect")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho collect:$*\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)
	var out bytes.Buffer
	if err := Run([]string{"collect", "--dry-run", "--agent", "codex"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "collect:--dry-run --agent codex" {
		t.Fatalf("unexpected collect output: %q", out.String())
	}
}

func TestCollectDelegatesToStintCollectNextToExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script executable fixture is unix-only")
	}
	dir := t.TempDir()
	stintPath := filepath.Join(dir, "stint")
	if err := os.WriteFile(stintPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	helperPath := filepath.Join(dir, "stint-collect")
	if err := os.WriteFile(helperPath, []byte("#!/bin/sh\necho sibling-collect:$*\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldExecutablePath := executablePath
	executablePath = func() (string, error) { return stintPath, nil }
	t.Cleanup(func() { executablePath = oldExecutablePath })
	t.Setenv("PATH", t.TempDir())

	var out bytes.Buffer
	if err := Run([]string{"collect", "--dry-run", "--agent", "codex"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "sibling-collect:--dry-run --agent codex" {
		t.Fatalf("unexpected collect output: %q", out.String())
	}
}
