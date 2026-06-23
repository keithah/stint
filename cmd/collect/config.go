package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/keithah/stint/internal/collector"
)

// Config is the resolved collector configuration. It is assembled from, in
// increasing precedence: built-in defaults, a JSON config file, environment
// variables, then explicit command-line flags.
//
// The JSON shape is the on-disk config file. Durations are stored as strings
// (e.g. "5m") so the file stays human-friendly; Interval is the parsed value.
type Config struct {
	APIURL     string              `json:"api_url"`
	APIKey     string              `json:"api_key"`
	CostMode   string              `json:"cost_mode"`
	StatePath  string              `json:"state_path"`
	Watch      bool                `json:"watch"`
	Interval   string              `json:"interval"`
	Agents     []string            `json:"agents,omitempty"`
	AgentPaths map[string][]string `json:"agent_paths,omitempty"`
}

// DefaultConfigPath is the default config-file location.
func DefaultConfigPath() string {
	return collector.ExpandHome("~/.stint/collect.json")
}

// defaultConfig returns the built-in defaults (lowest precedence).
func defaultConfig() Config {
	return Config{
		CostMode:  "calculate",
		StatePath: collector.DefaultStatePath(),
		Watch:     false,
		Interval:  "5m",
	}
}

// loadConfigFile reads and parses a config file. A missing file at the default
// path is not an error (returns zero Config, found=false); a missing file at an
// explicitly requested path is an error.
func loadConfigFile(path string, explicit bool) (Config, bool, error) {
	var c Config
	expanded := collector.ExpandHome(path)
	b, err := os.ReadFile(expanded)
	if err != nil {
		if os.IsNotExist(err) && !explicit {
			return c, false, nil
		}
		return c, false, fmt.Errorf("read config %s: %w", expanded, err)
	}
	if len(b) == 0 {
		return c, true, nil
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, false, fmt.Errorf("parse config %s: %w", expanded, err)
	}
	return c, true, nil
}

// resolved holds the fully-resolved runtime settings the runner consumes.
type resolved struct {
	APIURL     string
	APIKey     string
	CostMode   string
	StatePath  string
	Watch      bool
	Once       bool
	Interval   time.Duration
	DryRun     bool
	Agents     []string // explicit allowlist; empty means all registered
	AgentPaths map[string][]string
	ConfigPath string
}

// resolveConfig merges the four layers with correct precedence. flagSet reports
// which flags the user set explicitly so flag defaults don't clobber lower
// layers. file is the parsed config file (or zero if absent).
func resolveConfig(file Config, fileFound bool, fl *flags, flagSet map[string]bool) (resolved, error) {
	d := defaultConfig()
	r := resolved{}

	// api_url: file -> env -> flag.
	r.APIURL = firstNonEmpty(fl.apiURL, envIfSet("STINT_API_URL"), file.APIURL, d.APIURL)
	r.APIKey = firstNonEmpty(fl.apiKey, envIfSet("STINT_API_KEY"), file.APIKey, d.APIKey)
	r.CostMode = firstNonEmpty(fl.costMode, envIfSet("STINT_COST_MODE"), file.CostMode, d.CostMode)
	r.StatePath = firstNonEmpty(fl.statePath, envIfSet("STINT_STATE_PATH"), file.StatePath, d.StatePath)

	// watch (bool): flag (if set) -> env (if set) -> file (if found) -> default.
	switch {
	case flagSet["watch"]:
		r.Watch = fl.watch
	case envIfSet("STINT_WATCH") != "":
		r.Watch = parseBool(os.Getenv("STINT_WATCH"))
	case fileFound:
		r.Watch = file.Watch
	default:
		r.Watch = d.Watch
	}

	// interval (duration string): flag -> env -> file -> default.
	intervalStr := firstNonEmpty(fl.interval, envIfSet("STINT_INTERVAL"), file.Interval, d.Interval)
	iv, err := time.ParseDuration(intervalStr)
	if err != nil {
		return r, fmt.Errorf("invalid interval %q: %w", intervalStr, err)
	}
	r.Interval = iv

	// once defaults true; --watch implies a loop. An explicit --once flag wins.
	r.Once = fl.once
	if r.Watch && !flagSet["once"] {
		r.Once = false
	}

	r.DryRun = fl.dryRun

	// agents allowlist: explicit --agent flag wins; else config file list.
	if fl.agent != "" {
		r.Agents = []string{fl.agent}
	} else if len(file.Agents) > 0 {
		r.Agents = append([]string(nil), file.Agents...)
	}

	// per-agent path overrides come only from the config file.
	if len(file.AgentPaths) > 0 {
		r.AgentPaths = make(map[string][]string, len(file.AgentPaths))
		for id, paths := range file.AgentPaths {
			exp := make([]string, 0, len(paths))
			for _, p := range paths {
				if p = strings.TrimSpace(p); p != "" {
					exp = append(exp, collector.ExpandHome(p))
				}
			}
			if len(exp) > 0 {
				r.AgentPaths[id] = exp
			}
		}
	}

	return r, nil
}

// flags carries raw flag values plus the explicit env defaults already folded
// in by flag.String. resolveConfig re-derives precedence from the raw user
// input, so flags here hold only the user-provided values (see parseFlags).
type flags struct {
	apiURL      string
	apiKey      string
	costMode    string
	statePath   string
	interval    string
	agent       string
	once        bool
	watch       bool
	dryRun      bool
	configPath  string
	printConfig bool
	initConfig  bool
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func envIfSet(key string) string {
	return os.Getenv(key)
}

func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on", "y":
		return true
	default:
		return false
	}
}

// writeStarterConfig writes a starter config to path if it does not already
// exist. It reports whether it wrote a new file.
func writeStarterConfig(path string) (bool, error) {
	expanded := collector.ExpandHome(path)
	if _, err := os.Stat(expanded); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(expanded), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(expanded, []byte(starterConfigJSON), 0o600); err != nil {
		return false, err
	}
	return true, nil
}

// starterConfigJSON is the content written by `config init` / --init-config.
// Kept in sync with cmd/collect/collect.example.json.
const starterConfigJSON = `{
  "api_url": "https://stint.example.com/api/v1",
  "api_key": "",
  "cost_mode": "calculate",
  "state_path": "~/.stint/collector-state.json",
  "watch": false,
  "interval": "5m"
}
`

// configJSON renders a Config back to indented JSON for --print-config.
func configJSON(r resolved) string {
	out := Config{
		APIURL:     r.APIURL,
		APIKey:     redact(r.APIKey),
		CostMode:   r.CostMode,
		StatePath:  r.StatePath,
		Watch:      r.Watch,
		Interval:   r.Interval.String(),
		Agents:     r.Agents,
		AgentPaths: r.AgentPaths,
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	return string(b)
}

// redact masks all but the last 4 chars of a secret for safe display.
func redact(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(s)-4) + s[len(s)-4:]
}
