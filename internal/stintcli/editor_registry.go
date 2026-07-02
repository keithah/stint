package stintcli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
)

type EditorSpec struct {
	ID          string
	Name        string
	Binaries    []string
	ConfigPaths []string
	DeepInstall func(EditorSpec) error
}

type EditorEntry struct {
	Spec EditorSpec
}

type EditorRegistry map[string]EditorEntry

var (
	editorLookPath   = exec.LookPath
	editorRunCommand = func(name string, args ...string) error {
		cmd := exec.Command(name, args...) //nolint:gosec // Editor command is selected from a fixed registry row.
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			if message := strings.TrimSpace(stderr.String()); message != "" {
				return fmt.Errorf("%s: %w", message, err)
			}
			return err
		}
		return nil
	}
)

func DefaultEditorRegistry() EditorRegistry {
	r := EditorRegistry{}
	r.register(EditorSpec{ID: "vscode", Name: "VS Code", Binaries: []string{"code"}, DeepInstall: installVSCodeWakaTime})
	r.register(EditorSpec{ID: "cursor", Name: "Cursor", Binaries: []string{"cursor"}, DeepInstall: installVSCodeWakaTime})
	r.register(EditorSpec{ID: "windsurf", Name: "Windsurf", Binaries: []string{"windsurf"}, DeepInstall: installVSCodeWakaTime})
	r.register(EditorSpec{ID: "vscodium", Name: "VSCodium", Binaries: []string{"codium"}, DeepInstall: installVSCodeWakaTime})
	r.register(EditorSpec{
		ID:       "jetbrains",
		Name:     "JetBrains IDEs",
		Binaries: []string{"idea", "webstorm", "goland", "pycharm", "rubymine", "clion", "phpstorm", "rider", "datagrip", "android-studio", "studio"},
		ConfigPaths: []string{
			"~/.config/JetBrains",
			"~/Library/Application Support/JetBrains",
			"~/AppData/Roaming/JetBrains",
		},
		DeepInstall: installJetBrainsWakaTime,
	})
	r.register(EditorSpec{ID: "vim", Name: "Vim", Binaries: []string{"vim"}})
	r.register(EditorSpec{ID: "neovim", Name: "Neovim", Binaries: []string{"nvim"}})
	r.register(EditorSpec{ID: "zed", Name: "Zed", Binaries: []string{"zed"}})
	return r
}

func (r EditorRegistry) register(spec EditorSpec) {
	r[spec.ID] = EditorEntry{Spec: spec}
}

func (r EditorRegistry) IDs() []string {
	ids := make([]string, 0, len(r))
	for id := range r {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (r EditorRegistry) DetectInstalled() []string {
	var ids []string
	for _, id := range r.IDs() {
		if r[id].Detected() {
			ids = append(ids, id)
		}
	}
	return ids
}

func (e EditorEntry) Detected() bool {
	for _, binary := range e.Spec.Binaries {
		if _, err := editorLookPath(binary); err == nil {
			return true
		}
	}
	for _, path := range e.Spec.ConfigPaths {
		if info, err := os.Stat(expandHome(path)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func runConnect(args []string, stdout io.Writer) error {
	fs := newFlagSet("stint connect")
	deep := fs.Bool("deep", false, "install editor extensions when supported")
	server := fs.String("server", "", "Stint API URL")
	key := fs.String("key", "", "Stint API key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	apiURL, apiKey, err := connectCredentials(*server, *key)
	if err != nil {
		return err
	}
	reg := DefaultEditorRegistry()
	ids := reg.DetectInstalled()
	if len(ids) == 0 {
		fmt.Fprintln(stdout, "no supported editors detected")
		return nil
	}
	if err := writeSetupConfig(DefaultWakaTimeConfigPath(), apiURL, apiKey, false); err != nil {
		return err
	}
	var failures []string
	for _, id := range ids {
		entry := reg[id]
		status := "configured"
		if *deep && entry.Spec.DeepInstall != nil {
			if err := entry.Spec.DeepInstall(entry.Spec); err != nil {
				status = "configured; extension install failed"
				failures = append(failures, fmt.Sprintf("%s: %v", id, err))
			} else {
				status = "configured; extension installed"
			}
		}
		fmt.Fprintf(stdout, "%s %s\n", id, status)
	}
	if len(failures) > 0 {
		return fmt.Errorf("deep install failures: %s", strings.Join(failures, "; "))
	}
	return nil
}

func connectCredentials(server, key string) (string, string, error) {
	nativeCfg, err := loadNativeConfig()
	if err != nil {
		return "", "", err
	}
	wakaCfg, err := LoadConfigStack(DefaultWakaTimeConfigPath())
	if err != nil {
		return "", "", err
	}
	apiURL := first(server, os.Getenv("STINT_API_URL"), configFirst(nativeCfg, "api_url", "api-url", "apiurl"), configFirst(wakaCfg, "api_url", "api-url", "apiurl"))
	apiKey := first(key, os.Getenv("STINT_API_KEY"))
	if apiKey == "" {
		apiKey, err = resolveAPIKeyFromConfigs("", []Config{nativeCfg, wakaCfg}, os.Getenv("WAKATIME_API_KEY"))
		if err != nil {
			return "", "", err
		}
	}
	if apiURL == "" || apiKey == "" {
		return "", "", fmt.Errorf("missing Stint credentials; run `stint setup` first, pass --server and --key, or set STINT_API_URL and STINT_API_KEY")
	}
	return apiURL, apiKey, nil
}

func installVSCodeWakaTime(spec EditorSpec) error {
	for _, binary := range spec.Binaries {
		path, err := editorLookPath(binary)
		if err != nil {
			continue
		}
		return editorRunCommand(path, "--install-extension", "WakaTime.vscode-wakatime")
	}
	return fmt.Errorf("editor command not found")
}

func installJetBrainsWakaTime(spec EditorSpec) error {
	var paths []string
	for _, binary := range spec.Binaries {
		path, err := editorLookPath(binary)
		if err != nil {
			continue
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return fmt.Errorf("JetBrains command-line launcher not found")
	}
	var failures []string
	successes := 0
	for _, path := range paths {
		if err := editorRunCommand(path, "installPlugins", "com.wakatime.intellij.plugin"); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		successes++
	}
	if successes > 0 {
		return nil
	}
	return fmt.Errorf("all JetBrains plugin installs failed: %s", strings.Join(failures, "; "))
}
