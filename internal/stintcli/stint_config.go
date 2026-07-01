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
	stintWrite, err := prepareSetupConfig(*stintConfig, apiURL, apiKey, true)
	if err != nil {
		return fmt.Errorf("prepare stint config: %w", err)
	}
	defer stintWrite.cleanup()
	wakaWrite, err := prepareSetupConfig(*wakaConfig, apiURL, apiKey, false)
	if err != nil {
		return fmt.Errorf("prepare wakatime config: %w", err)
	}
	defer wakaWrite.cleanup()
	if err := commitSetupConfigs(&stintWrite, &wakaWrite); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s and %s\n", expandHome(*stintConfig), expandHome(*wakaConfig))
	return nil
}

func writeSetupConfig(path, apiURL, apiKey string, native bool) error {
	write, err := prepareSetupConfig(path, apiURL, apiKey, native)
	if err != nil {
		return err
	}
	defer write.cleanup()
	return write.commit()
}

type preparedSetupConfig struct {
	path       string
	tmpPath    string
	backupPath string
	oldBytes   []byte
	hadOld     bool
	committed  bool
}

func prepareSetupConfig(path, apiURL, apiKey string, native bool) (preparedSetupConfig, error) {
	path = expandHome(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return preparedSetupConfig{}, err
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		return preparedSetupConfig{}, err
	}
	cfg.Set("settings", "api_url", apiURL)
	cfg.Set("settings", "api_key", apiKey)
	if native {
		cfg.Set("settings", "offline", "true")
	}
	prepared := preparedSetupConfig{path: path}
	if oldBytes, err := os.ReadFile(path); err == nil {
		prepared.oldBytes = oldBytes
		prepared.hadOld = true
	} else if !os.IsNotExist(err) {
		return preparedSetupConfig{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return preparedSetupConfig{}, err
	}
	prepared.tmpPath = tmp.Name()
	if err := tmp.Close(); err != nil {
		prepared.cleanup()
		return preparedSetupConfig{}, err
	}
	if err := cfg.Write(prepared.tmpPath); err != nil {
		prepared.cleanup()
		return preparedSetupConfig{}, err
	}
	return prepared, nil
}

func commitSetupConfigs(configs ...*preparedSetupConfig) error {
	var committed []*preparedSetupConfig
	for _, config := range configs {
		if err := config.commit(); err != nil {
			for i := len(committed) - 1; i >= 0; i-- {
				_ = committed[i].restore()
			}
			return err
		}
		committed = append(committed, config)
	}
	return nil
}

func (p *preparedSetupConfig) commit() error {
	if p.hadOld {
		backup, err := os.CreateTemp(filepath.Dir(p.path), filepath.Base(p.path)+".bak-*")
		if err != nil {
			return err
		}
		p.backupPath = backup.Name()
		if err := backup.Close(); err != nil {
			return err
		}
		if err := os.Remove(p.backupPath); err != nil {
			return err
		}
		if err := os.Rename(p.path, p.backupPath); err != nil {
			return err
		}
		if err := os.Rename(p.tmpPath, p.path); err != nil {
			_ = os.Rename(p.backupPath, p.path)
			return err
		}
		p.committed = true
		return nil
	}
	if err := os.Rename(p.tmpPath, p.path); err != nil {
		return err
	}
	p.committed = true
	return nil
}

func (p preparedSetupConfig) restore() error {
	if p.hadOld {
		if p.backupPath != "" && fileExists(p.backupPath) {
			if err := os.Remove(p.path); err != nil && !os.IsNotExist(err) {
				return err
			}
			return os.Rename(p.backupPath, p.path)
		}
		return os.WriteFile(p.path, p.oldBytes, 0o600)
	}
	if err := os.Remove(p.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (p *preparedSetupConfig) cleanup() {
	if !p.committed && p.tmpPath != "" {
		_ = os.Remove(p.tmpPath)
	}
	if p.committed && p.backupPath != "" {
		_ = os.Remove(p.backupPath)
	}
}

func loadNativeConfig() (Config, error) {
	return LoadConfig(DefaultStintConfigPath())
}
