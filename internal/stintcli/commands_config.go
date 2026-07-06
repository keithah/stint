package stintcli

import (
	"fmt"
	"io"
	"strings"
)

func runConfig(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: stint config init|read|write")
	}
	switch args[0] {
	case "init":
		fs := newFlagSet("stint config init")
		configPath := fs.String("config", DefaultStintConfigPath(), "config path")
		apiURL := fs.String("api-url", defaultAPIURL, "Stint API URL")
		apiKey := fs.String("api-key", "", "Stint API key")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		*configPath = defaultConfigPathIfEmpty(*configPath)
		if err := InitConfig(*configPath, *apiURL, *apiKey); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "wrote config: %s\n", expandHome(*configPath))
		return nil
	case "read":
		fs := newFlagSet("stint config read")
		configPath := fs.String("config", DefaultStintConfigPath(), "config path")
		section := fs.String("section", "settings", "config section")
		configSection := fs.String("config-section", "", "config section")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return fmt.Errorf("usage: stint config read KEY")
		}
		*configPath = defaultConfigPathIfEmpty(*configPath)
		cfg, err := LoadConfigStack(*configPath)
		if err != nil {
			return err
		}
		if *configSection != "" {
			*section = *configSection
		}
		return writeConfigRead(stdout, cfg, *section, fs.Arg(0))
	case "write":
		fs := newFlagSet("stint config write")
		configPath := fs.String("config", DefaultStintConfigPath(), "config path")
		section := fs.String("section", "settings", "config section")
		configSection := fs.String("config-section", "", "config section")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 2 {
			return fmt.Errorf("usage: stint config write KEY VALUE")
		}
		if *configSection != "" {
			*section = *configSection
		}
		*configPath = defaultConfigPathIfEmpty(*configPath)
		return WriteConfigValue(*configPath, *section, fs.Arg(0), fs.Arg(1))
	default:
		return fmt.Errorf("unknown config command %q", args[0])
	}
}

func writeConfigRead(stdout io.Writer, cfg Config, section, key string) error {
	if strings.TrimSpace(section) == "" || strings.TrimSpace(key) == "" {
		return fmt.Errorf("failed reading wakatime config file. neither section nor key can be empty")
	}
	value := strings.TrimSpace(cfg.Get(section, key))
	if value == "" {
		return fmt.Errorf("given section and key %q returned an empty string", strings.TrimSpace(section)+"."+strings.TrimSpace(key))
	}
	fmt.Fprintln(stdout, value)
	return nil
}
