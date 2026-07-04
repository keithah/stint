package stintcli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type PluginSpec struct {
	ID       string
	Host     string
	Binary   string
	Commands [][]string
	Install  func(PluginSpec, string) error
}

type PluginRegistry map[string]PluginSpec

var (
	pluginLookPath   = exec.LookPath
	pluginRunCommand = runCommandDiscardingOutput
)

var (
	ampPluginURL    = "https://raw.githubusercontent.com/wakatime/amp-cli-wakatime/b0a6b7639b57a255cae2866cf686b2bc47eedf66/amp-cli-wakatime.ts"
	ampPluginSHA256 = "9d2714df7a20cdb1d5fef0859587c4524f13fae3373e527a9e625a1cec887e76"
)

func DefaultPluginRegistry() PluginRegistry {
	return PluginRegistry{
		"claude-code": {
			ID:     "claude-code",
			Host:   "Claude Code",
			Binary: "claude",
			Commands: [][]string{
				{"plugin", "marketplace", "add", "https://github.com/wakatime/claude-code-wakatime.git"},
				{"plugin", "i", "claude-code-wakatime@wakatime"},
			},
		},
		"codex": {
			ID:     "codex",
			Host:   "Codex CLI",
			Binary: "codex",
			Commands: [][]string{
				{"plugin", "marketplace", "add", "wakatime/codex-cli-wakatime"},
				{"plugin", "add", "codex-cli-wakatime@wakatime"},
			},
		},
		"antigravity": {
			ID:     "antigravity",
			Host:   "Antigravity CLI",
			Binary: "agy",
			Commands: [][]string{
				{"plugin", "install", "https://github.com/wakatime/antigravity-cli-wakatime"},
			},
		},
		"amp": {
			ID:      "amp",
			Host:    "Amp CLI",
			Binary:  "amp",
			Install: installAmpPlugin,
		},
		"copilot": {
			ID:     "copilot",
			Host:   "GitHub Copilot CLI",
			Binary: "copilot",
			Commands: [][]string{
				{"plugin", "marketplace", "add", "wakatime/copilot-cli-wakatime"},
				{"plugin", "install", "copilot-cli-wakatime@wakatime"},
			},
		},
	}
}

func (r PluginRegistry) IDs() []string {
	ids := make([]string, 0, len(r))
	for id := range r {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func runPluginInstall(args []string, stdout io.Writer) error {
	fs := newFlagSet("stint plugin install")
	server := fs.String("server", "", "Stint API URL")
	key := fs.String("key", "", "Stint API key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: stint plugin install <agent>")
	}
	reg := DefaultPluginRegistry()
	agent := strings.TrimSpace(fs.Arg(0))
	spec, ok := reg[agent]
	if !ok {
		return fmt.Errorf("unknown plugin agent %q (supported agents: %s)", agent, strings.Join(reg.IDs(), ", "))
	}
	apiURL, apiKey, err := connectCredentials(*server, *key)
	if err != nil {
		return err
	}
	binary, err := pluginLookPath(spec.Binary)
	if err != nil {
		return fmt.Errorf("%s is not installed or not on PATH; install %s first", spec.Binary, spec.Host)
	}
	if spec.Install != nil {
		if err := spec.Install(spec, binary); err != nil {
			return err
		}
	} else {
		for _, command := range spec.Commands {
			if err := pluginRunCommand(binary, command...); err != nil {
				return fmt.Errorf("%s plugin command failed: %w", spec.ID, err)
			}
		}
	}
	if err := writeSetupConfig(DefaultWakaTimeConfigPath(), apiURL, apiKey, false); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s plugin installed\n", spec.ID)
	return nil
}

func installAmpPlugin(_ PluginSpec, _ string) error {
	data, err := downloadFile(ampPluginURL)
	if err != nil {
		return err
	}
	if err := verifySHA256(data, ampPluginSHA256); err != nil {
		return fmt.Errorf("verify amp plugin: %w", err)
	}
	path := filepath.Join(expandHome("~/.config/amp/plugins"), "amp-cli-wakatime.ts")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func verifySHA256(data []byte, want string) error {
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != want {
		return fmt.Errorf("sha256 mismatch: got %s, want %s", got, want)
	}
	return nil
}
