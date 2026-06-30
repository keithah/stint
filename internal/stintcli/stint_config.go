package stintcli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// DefaultStintConfigPath returns Stint's native config path. This is separate
// from ~/.wakatime.cfg because upstream plugins only read the WakaTime file.
func DefaultStintConfigPath() string {
	return expandHome("~/.stint.cfg")
}

func runSetup(args []string, stdout io.Writer) error {
	fs := newFlagSet("stint setup")
	server := fs.String("server", "", "Stint API URL")
	key := fs.String("key", "", "Stint API key")
	stintConfig := fs.String("stint-config", DefaultStintConfigPath(), "native Stint config path")
	wakaConfig := fs.String("wakatime-config", DefaultWakaTimeConfigPath(), "WakaTime compatibility config path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	apiURL := first(*server, os.Getenv("STINT_API_URL"))
	apiKey := first(*key, os.Getenv("STINT_API_KEY"))
	if apiURL == "" || apiKey == "" {
		return fmt.Errorf("server and key are required (use --server/--key or STINT_API_URL/STINT_API_KEY)")
	}
	if err := writeSetupConfig(*stintConfig, apiURL, apiKey, true); err != nil {
		return fmt.Errorf("write stint config: %w", err)
	}
	if err := writeSetupConfig(*wakaConfig, apiURL, apiKey, false); err != nil {
		return fmt.Errorf("write wakatime config: %w", err)
	}
	fmt.Fprintf(stdout, "wrote %s and %s\n", expandHome(*stintConfig), expandHome(*wakaConfig))
	return nil
}

func writeSetupConfig(path, apiURL, apiKey string, native bool) error {
	path = expandHome(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		return err
	}
	cfg.Set("settings", "api_url", apiURL)
	cfg.Set("settings", "api_key", apiKey)
	if native {
		cfg.Set("settings", "offline", "true")
	}
	return cfg.Write(path)
}

func loadNativeConfig() Config {
	cfg, _ := LoadConfig(DefaultStintConfigPath())
	return cfg
}
