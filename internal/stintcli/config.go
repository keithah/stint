package stintcli

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Config is a small INI reader/writer compatible with the WakaTime settings
// files editor plugins already create.
type Config struct {
	Sections     map[string]map[string]string
	SectionOrder map[string][]string
}

type ConfigEntry struct {
	Key   string
	Value string
}

func DefaultWakaTimeConfigPath() string {
	if home := strings.TrimSpace(os.Getenv("WAKATIME_HOME")); home != "" {
		return filepath.Join(expandHome(home), ".wakatime.cfg")
	}
	return expandHome("~/.wakatime.cfg")
}

func DefaultInternalConfigPath() string {
	return filepath.Join(wakaResourcesDir(), "wakatime-internal.cfg")
}

func defaultConfigPathIfEmpty(path string) string {
	if strings.TrimSpace(path) == "" {
		return DefaultWakaTimeConfigPath()
	}
	return path
}

func defaultInternalConfigPathIfEmpty(path string) string {
	if strings.TrimSpace(path) == "" {
		return DefaultInternalConfigPath()
	}
	return path
}

func LoadConfig(path string) (Config, error) {
	cfg := Config{Sections: map[string]map[string]string{}, SectionOrder: map[string][]string{}}
	path = expandHome(path)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	defer f.Close()
	section := "settings"
	cfg.ensure(section)
	scanner := bufio.NewScanner(f)
	var multilineKey string
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]"))
			cfg.ensure(section)
			multilineKey = ""
			continue
		}
		if multilineKey != "" && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			current := cfg.Get(section, multilineKey)
			if current != "" {
				current += "\n"
			}
			cfg.Set(section, multilineKey, current+trimmed)
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		cfg.Set(section, key, strings.TrimSpace(value))
		multilineKey = key
	}
	return cfg, scanner.Err()
}

func LoadConfigStack(path string) (Config, error) {
	cfg, err := LoadConfig(path)
	if err != nil {
		return cfg, err
	}
	if importPath := strings.TrimSpace(cfg.Get("settings", "import_cfg")); importPath != "" {
		importPath, err = expandHomeStrict(importPath)
		if err != nil {
			return cfg, fmt.Errorf("failed to expand settings.import_cfg param: %w", err)
		}
		imported, err := LoadConfig(importPath)
		if err != nil {
			return cfg, err
		}
		cfg.Merge(imported)
	}
	return cfg, nil
}

func LoadConfigForEntity(path, entity, entityType string, isUnsaved bool) (Config, Config, bool, error) {
	cfg, err := LoadConfigStack(path)
	if err != nil {
		return cfg, Config{}, false, err
	}
	projectPath := ProjectConfigPath(entity, entityType, isUnsaved)
	if projectPath == "" {
		return cfg, Config{}, false, nil
	}
	projectCfg, err := LoadConfig(projectPath)
	if err != nil {
		return cfg, Config{}, false, err
	}
	cfg.Merge(projectCfg)
	return cfg, projectCfg, true, nil
}

func ProjectConfigPath(entity, entityType string, isUnsaved bool) string {
	if entityType != "file" || entity == "" || isUnsaved {
		return ""
	}
	entity = expandHome(entity)
	if !filepath.IsAbs(entity) {
		if abs, err := filepath.Abs(entity); err == nil {
			entity = abs
		}
	}
	dir := entity
	if info, err := os.Stat(entity); err != nil || !info.IsDir() {
		dir = filepath.Dir(entity)
	}
	for dir != "" && dir != "." && dir != string(filepath.Separator) {
		candidate := filepath.Join(dir, ".wakatime")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	return ""
}

func InitConfig(path, apiURL, apiKey string) error {
	path = expandHome(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	cfg := Config{Sections: map[string]map[string]string{}, SectionOrder: map[string][]string{}}
	cfg.Set("settings", "api_url", apiURL)
	cfg.Set("settings", "api_key", apiKey)
	cfg.Set("settings", "offline", "true")
	cfg.Set("settings", "heartbeat_rate_limit_seconds", "120")
	return cfg.Write(path)
}

func WriteConfigValue(path, section, key, value string) error {
	return WriteConfigValues(path, section, map[string]string{key: value})
}

func WriteConfigValues(path, section string, values map[string]string) error {
	cfg, err := LoadConfig(path)
	if err != nil {
		return err
	}
	for key, value := range values {
		cfg.Set(section, key, value)
	}
	return cfg.Write(path)
}

func (c Config) Get(section, key string) string {
	if c.Sections == nil {
		return ""
	}
	section = canonicalConfigSection(section)
	key = canonicalConfigKey(section, key)
	keys := c.Sections[section]
	if keys != nil {
		if value := keys[key]; value != "" {
			return trimConfigValue(value)
		}
	}
	if section == "settings" || section == "" {
		if defaults := c.Sections["DEFAULT"]; defaults != nil {
			return trimConfigValue(defaults[key])
		}
	}
	return ""
}

func (c Config) Section(section string) map[string]string {
	section = canonicalConfigSection(section)
	values := c.Sections[section]
	if (section == "settings" || section == "") && len(c.Sections["DEFAULT"]) > 0 {
		merged := make(map[string]string, len(values)+len(c.Sections["DEFAULT"]))
		for key, value := range c.Sections["DEFAULT"] {
			merged[key] = value
		}
		for key, value := range values {
			merged[key] = value
		}
		values = merged
	}
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = trimConfigValue(value)
	}
	return out
}

func (c Config) OrderedSection(section string) []ConfigEntry {
	section = canonicalConfigSection(section)
	values := c.Sections[section]
	if len(values) == 0 {
		return nil
	}
	keys := c.orderedKeys(section, values)
	out := make([]ConfigEntry, 0, len(keys))
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		out = append(out, ConfigEntry{Key: key, Value: trimConfigValue(value)})
	}
	return out
}

func (c Config) Has(section, key string) bool {
	if c.Sections == nil {
		return false
	}
	section = canonicalConfigSection(section)
	key = canonicalConfigKey(section, key)
	values := c.Sections[section]
	if values != nil {
		if _, ok := values[key]; ok {
			return true
		}
	}
	if section == "settings" || section == "" {
		if defaults := c.Sections["DEFAULT"]; defaults != nil {
			_, ok := defaults[key]
			return ok
		}
	}
	return false
}

func (c *Config) Merge(other Config) {
	if c.Sections == nil {
		c.Sections = map[string]map[string]string{}
	}
	if c.SectionOrder == nil {
		c.SectionOrder = map[string][]string{}
	}
	for section := range other.Sections {
		c.ensure(section)
		for _, entry := range other.OrderedSection(section) {
			c.Set(section, entry.Key, entry.Value)
		}
	}
}

func (c Config) Bool(section, key string) bool {
	switch strings.ToLower(strings.TrimSpace(c.Get(section, key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (c Config) Int(section, key string) int {
	value, err := strconv.Atoi(strings.TrimSpace(c.Get(section, key)))
	if err != nil {
		return 0
	}
	return value
}

func trimConfigValue(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"'`)
}

func (c *Config) Set(section, key, value string) {
	if c.Sections == nil {
		c.Sections = map[string]map[string]string{}
	}
	if c.SectionOrder == nil {
		c.SectionOrder = map[string][]string{}
	}
	section = canonicalConfigSection(section)
	key = canonicalConfigKey(section, key)
	key = strings.ReplaceAll(key, "\x00", "")
	value = strings.ReplaceAll(value, "\x00", "")
	c.ensure(section)
	if _, exists := c.Sections[section][key]; !exists {
		c.SectionOrder[section] = append(c.SectionOrder[section], key)
	}
	c.Sections[section][key] = value
}

func (c *Config) ensure(section string) {
	if c.Sections == nil {
		c.Sections = map[string]map[string]string{}
	}
	if c.SectionOrder == nil {
		c.SectionOrder = map[string][]string{}
	}
	section = canonicalConfigSection(section)
	if c.Sections[section] == nil {
		c.Sections[section] = map[string]string{}
	}
	if c.SectionOrder[section] == nil {
		c.SectionOrder[section] = []string{}
	}
}

func (c Config) orderedKeys(section string, values map[string]string) []string {
	if order := c.SectionOrder[section]; len(order) > 0 {
		keys := make([]string, 0, len(order))
		seen := map[string]bool{}
		for _, key := range order {
			if _, ok := values[key]; ok {
				keys = append(keys, key)
				seen[key] = true
			}
		}
		var extras []string
		for key := range values {
			if !seen[key] {
				extras = append(extras, key)
			}
		}
		sort.Strings(extras)
		return append(keys, extras...)
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func canonicalConfigSection(section string) string {
	section = strings.TrimSpace(section)
	if strings.EqualFold(section, "DEFAULT") {
		return "DEFAULT"
	}
	return strings.ToLower(section)
}

func canonicalConfigKey(section, key string) string {
	key = strings.TrimSpace(key)
	switch canonicalConfigSection(section) {
	case "settings", "DEFAULT", "internal", "git":
		return strings.ToLower(key)
	default:
		return key
	}
}

func (c Config) Write(path string) error {
	path = expandHome(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	if len(c.Sections) == 0 {
		c.ensure("settings")
	}
	writeSection := func(section string) {
		values := c.Sections[section]
		if values == nil {
			return
		}
		fmt.Fprintf(&b, "[%s]\n", section)
		for _, key := range c.orderedKeys(section, values) {
			value := values[key]
			if strings.Contains(value, "\n") {
				fmt.Fprintf(&b, "%s =\n", key)
				for _, line := range strings.Split(value, "\n") {
					if strings.TrimSpace(line) != "" {
						fmt.Fprintf(&b, "    %s\n", line)
					}
				}
			} else {
				fmt.Fprintf(&b, "%s = %s\n", key, value)
			}
		}
		b.WriteByte('\n')
	}
	writeSection("settings")
	for section := range c.Sections {
		if section != "settings" {
			writeSection(section)
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

func expandHome(path string) string {
	expanded, err := expandHomeStrict(path)
	if err != nil {
		return path
	}
	return expanded
}

func expandHomeStrict(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home, nil
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
		}
	}
	if strings.HasPrefix(path, "~") {
		rest := strings.TrimPrefix(path, "~")
		sep := strings.IndexAny(rest, `/\`)
		if sep > 0 {
			name := rest[:sep]
			u, err := user.Lookup(name)
			if err != nil {
				return "", err
			}
			return filepath.Join(u.HomeDir, rest[sep+1:]), nil
		}
	}
	return path, nil
}
