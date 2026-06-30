package stintcli

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestWakaTimeCLIInstallRespectsOverride(t *testing.T) {
	t.Setenv("STINT_WAKATIME_CLI", filepath.Join(t.TempDir(), "custom-wakatime-cli"))
	var out bytes.Buffer
	if err := Run([]string{"cli", "install"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "using STINT_WAKATIME_CLI") {
		t.Fatalf("expected override summary, got %q", out.String())
	}
}

func TestWakaTimeCLIInstallVerifiesChecksumAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)

	archive := buildZip(t, "wakatime-cli", []byte("#!/bin/sh\necho wakatime\n"))
	sum := sha256.Sum256(archive)
	spec := WakaTimeCLISpec{
		Version:  "vtest",
		GOOS:     runtime.GOOS,
		GOARCH:   runtime.GOARCH,
		SHA256:   hex.EncodeToString(sum[:]),
		BaseURL:  "https://example.invalid/wakatime",
		FileName: "wakatime-cli-" + runtime.GOOS + "-" + runtime.GOARCH + ".zip",
	}
	oldSpec := pinnedWakaTimeCLISpec
	oldDownload := downloadFile
	pinnedWakaTimeCLISpec = func() (WakaTimeCLISpec, error) { return spec, nil }
	downloadFile = func(url string) ([]byte, error) {
		if !strings.HasSuffix(url, spec.FileName) {
			t.Fatalf("unexpected download URL: %s", url)
		}
		return archive, nil
	}
	t.Cleanup(func() {
		pinnedWakaTimeCLISpec = oldSpec
		downloadFile = oldDownload
	})

	var out bytes.Buffer
	if err := Run([]string{"cli", "install"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(wakaResourcesDir(), "wakatime-cli")
	if info, err := os.Stat(installed); err != nil {
		t.Fatal(err)
	} else if info.Mode()&0o111 == 0 {
		t.Fatalf("installed binary is not executable: %v", info.Mode())
	}
	versionFile, err := os.ReadFile(filepath.Join(wakaResourcesDir(), "wakatime-cli.version"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(versionFile)) != "vtest" {
		t.Fatalf("version file = %q", versionFile)
	}
	if !strings.Contains(out.String(), "installed wakatime-cli vtest") {
		t.Fatalf("expected install summary, got %q", out.String())
	}

	out.Reset()
	if err := Run([]string{"cli", "install"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "already installed") {
		t.Fatalf("expected idempotent summary, got %q", out.String())
	}
}

func TestWakaTimeCLIInstallRejectsChecksumMismatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("WAKATIME_HOME", home)

	oldSpec := pinnedWakaTimeCLISpec
	oldDownload := downloadFile
	pinnedWakaTimeCLISpec = func() (WakaTimeCLISpec, error) {
		return WakaTimeCLISpec{
			Version:  "vtest",
			GOOS:     runtime.GOOS,
			GOARCH:   runtime.GOARCH,
			SHA256:   strings.Repeat("0", 64),
			BaseURL:  "https://example.invalid/wakatime",
			FileName: "wakatime-cli-" + runtime.GOOS + "-" + runtime.GOARCH + ".zip",
		}, nil
	}
	downloadFile = func(string) ([]byte, error) {
		return buildZip(t, "wakatime-cli", []byte("bad")), nil
	}
	t.Cleanup(func() {
		pinnedWakaTimeCLISpec = oldSpec
		downloadFile = oldDownload
	})

	err := Run([]string{"cli", "install"}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func buildZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0o755)
	w, err := zw.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
