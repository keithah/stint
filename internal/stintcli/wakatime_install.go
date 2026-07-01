package stintcli

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const pinnedWakaTimeCLIVersion = "v2.9.3"
const maxWakaTimeCLIDownloadBytes int64 = 50 << 20

type WakaTimeCLISpec struct {
	Version  string
	GOOS     string
	GOARCH   string
	SHA256   string
	BaseURL  string
	FileName string
}

var (
	pinnedWakaTimeCLISpec = defaultPinnedWakaTimeCLISpec
	downloadFile          = defaultDownloadFile
)

func runCLICommand(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: stint cli install")
	}
	switch args[0] {
	case "install":
		return runWakaTimeCLIInstall(args[1:], stdout)
	default:
		return fmt.Errorf("unknown cli command %q", args[0])
	}
}

func runWakaTimeCLIInstall(args []string, stdout io.Writer) error {
	fs := newFlagSet("stint cli install")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if override := strings.TrimSpace(os.Getenv("STINT_WAKATIME_CLI")); override != "" {
		path := expandHome(override)
		if !fileExists(path) {
			return fmt.Errorf("STINT_WAKATIME_CLI points to missing binary: %s", path)
		}
		fmt.Fprintf(stdout, "using STINT_WAKATIME_CLI=%s\n", path)
		return nil
	}
	spec, err := pinnedWakaTimeCLISpec()
	if err != nil {
		return err
	}
	dir := wakaResourcesDir()
	target := filepath.Join(dir, executableName("wakatime-cli"))
	versionPath := filepath.Join(dir, "wakatime-cli.version")
	if installedVersion(versionPath) == spec.Version && fileExists(target) {
		fmt.Fprintf(stdout, "wakatime-cli %s already installed: %s\n", spec.Version, target)
		return nil
	}
	data, err := downloadFile(strings.TrimRight(spec.BaseURL, "/") + "/" + spec.FileName)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, spec.SHA256) {
		return fmt.Errorf("checksum mismatch for %s: got %s want %s", spec.FileName, got, spec.SHA256)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := extractWakaTimeCLI(data, target); err != nil {
		return err
	}
	if err := os.WriteFile(versionPath, []byte(spec.Version+"\n"), 0o600); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "installed wakatime-cli %s: %s\n", spec.Version, target)
	return nil
}

func defaultPinnedWakaTimeCLISpec() (WakaTimeCLISpec, error) {
	fileName := "wakatime-cli-" + runtime.GOOS + "-" + runtime.GOARCH + ".zip"
	checksums := map[string]string{
		"wakatime-cli-darwin-amd64.zip":  "7d7d5f00c8aa561d0c34c9a0183a3512cac0116ece1ff752f512bc4d8a20c875",
		"wakatime-cli-darwin-arm64.zip":  "ef49d908a7b059486c4ad9a3c989f024f3eb0e1b306f67744747f87d90d8d3e3",
		"wakatime-cli-linux-386.zip":     "d20daf7b82f475c654f521d176bb39a5600f409ae0133541edfd87ca02330dde",
		"wakatime-cli-linux-amd64.zip":   "673b552541d4cc8f47974bafcca649ef521c7fd7109002ea5adaa3be5c1f4c27",
		"wakatime-cli-linux-arm.zip":     "aba065faf7f8e9fffdfc9e43d6a379baeb422a9c876a60c1db39d53dc96421c6",
		"wakatime-cli-linux-arm64.zip":   "fe3ed4e33382440fbfa4e4d9669d217577826f16646aa83985eb343dfc2dc5cd",
		"wakatime-cli-windows-386.zip":   "8a4e8a8758fae1aa19f69de1264df342a1e7670a88ef7e19dce72409831217a2",
		"wakatime-cli-windows-amd64.zip": "ee6ceece9594bf68f01e605e379f0c186ab831188a2f5b082633416e6e1ce193",
		"wakatime-cli-windows-arm64.zip": "9443d2f3cced086de0d6c9ff32f2b85e696c8c7a17463feec864fedc3e341760",
	}
	sum := checksums[fileName]
	if sum == "" {
		return WakaTimeCLISpec{}, fmt.Errorf("unsupported wakatime-cli platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return WakaTimeCLISpec{
		Version:  pinnedWakaTimeCLIVersion,
		GOOS:     runtime.GOOS,
		GOARCH:   runtime.GOARCH,
		SHA256:   sum,
		BaseURL:  "https://github.com/wakatime/wakatime-cli/releases/download/" + pinnedWakaTimeCLIVersion,
		FileName: fileName,
	}, nil
}

func defaultDownloadFile(url string) ([]byte, error) {
	client := http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url) //nolint:gosec // URL is a pinned release asset.
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	limited := io.LimitReader(resp.Body, maxWakaTimeCLIDownloadBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxWakaTimeCLIDownloadBytes {
		return nil, fmt.Errorf("download %s too large: exceeds %d bytes", url, maxWakaTimeCLIDownloadBytes)
	}
	return data, nil
}

func extractWakaTimeCLI(data []byte, target string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	want := executableName("wakatime-cli")
	for _, f := range zr.File {
		if filepath.Base(f.Name) != want && filepath.Base(f.Name) != "wakatime-cli" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		content, err := io.ReadAll(rc)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, 0o755)
	}
	return fmt.Errorf("archive does not contain wakatime-cli")
}

func installedVersion(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
