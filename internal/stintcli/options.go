package stintcli

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Options is the resolved CLI option set.
type Options struct {
	AIInputTokens          int
	AIInputTokensSet       bool
	AILineChanges          int
	AILineChangesSet       bool
	AIAgent                string
	AIAgentComplexity      string
	AIAgentVersion         string
	AIModel                string
	AIOutputTokens         int
	AIOutputTokensSet      bool
	AIProvider             string
	AIPromptLength         int
	AIPromptLengthSet      bool
	AISession              string
	AISubscriptionPlan     string
	AlternateBranch        string
	AlternateLanguage      string
	AlternateProject       string
	APIKey                 string
	APIURL                 string
	Branch                 string
	Category               string
	CategorySet            bool
	CommitHash             string
	ConfigPath             string
	ConfigRead             string
	ConfigReadSet          bool
	ConfigSection          string
	ConfigWrite            map[string]string
	Config                 Config
	CursorPosition         int
	CursorPositionSet      bool
	DisableOffline         bool
	Entity                 string
	EntitySet              bool
	EntityType             string
	Editor                 string
	EditorVersion          string
	Exclude                []string
	ExcludeUnknownProject  bool
	ExtraHeartbeats        bool
	FileExperts            bool
	Goal                   string
	GuessLanguage          bool
	HeartbeatRateLimit     int
	HideBranchNames        string
	HideFileNames          string
	HideDependencies       string
	HideProjectFolder      bool
	HideProjectNames       string
	Hostname               string
	HumanLineChanges       int
	HumanLineChangesSet    bool
	Include                []string
	IncludeOnlyProjectFile bool
	InternalConfig         Config
	InternalConfigPath     string
	IsUnsavedEntity        bool
	Key                    string
	Language               string
	LegacyQueuePath        string
	LineNumber             int
	LineNumberSet          bool
	LinesInFile            int
	LinesInFileSet         bool
	LocalFile              string
	LogFile                string
	LogToStdout            bool
	LogWriter              io.Writer
	Metadata               string
	Metrics                bool
	NoSSLVerify            bool
	OfflineCount           bool
	Output                 string
	Plugin                 string
	PluginVersion          string
	PrintOffline           int
	PrintOfflineSet        bool
	Project                string
	ProjectFolder          string
	Proxy                  string
	QueuePath              string
	Range                  string
	SSLCertsFile           string
	SendDiagnosticsOnError bool
	SyncAIActivity         bool
	SyncAIDisabled         bool
	SyncAIAfter            float64
	SyncOffline            int
	SyncOfflineSet         bool
	Time                   float64
	Timeout                int
	Today                  bool
	TodayCodingActivity    bool
	TodayHideCategories    bool
	TodayHideMinutes       bool
	TodayGoal              string
	TodayGoalSet           bool
	TodayMaxCategories     int
	TodayStatusBarEnabled  bool
	UserAgent              bool
	Verbose                bool
	Version                bool
	Write                  bool
	WriteSet               bool
	BackoffAt              time.Time
	BackoffRetries         int
}

func parseCommon(args []string) (Options, error) {
	return parseCommonWithFlagSet(newFlagSet("stint"), args)
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func parseCommonWithFlagSet(fs *flag.FlagSet, args []string) (Options, error) {
	fs.SetOutput(io.Discard)
	var o Options
	fs.IntVar(&o.AILineChanges, "ai-line-changes", 0, "AI line changes")
	aiAgentAlias := ""
	fs.StringVar(&o.AIAgent, "ai-agent", "", "AI agent")
	fs.StringVar(&aiAgentAlias, "ai-agent-name", "", "AI agent")
	fs.StringVar(&o.AIAgentComplexity, "ai-agent-complexity", "", "AI agent complexity")
	fs.StringVar(&o.AIAgentVersion, "ai-agent-version", "", "AI agent version")
	fs.IntVar(&o.AIInputTokens, "ai-input-tokens", 0, "AI input tokens")
	aiModelNameAlias := ""
	modelNameAlias := ""
	llmModelAlias := ""
	modelAlias := ""
	fs.StringVar(&o.AIModel, "ai-model", "", "AI model")
	fs.StringVar(&aiModelNameAlias, "ai-model-name", "", "AI model")
	fs.StringVar(&modelNameAlias, "model-name", "", "model name")
	fs.StringVar(&llmModelAlias, "llm-model", "", "LLM model")
	fs.StringVar(&modelAlias, "model", "", "model")
	fs.IntVar(&o.AIOutputTokens, "ai-output-tokens", 0, "AI output tokens")
	providerAlias := ""
	llmProviderAlias := ""
	fs.StringVar(&o.AIProvider, "ai-provider", "", "AI provider")
	fs.StringVar(&providerAlias, "provider", "", "provider")
	fs.StringVar(&llmProviderAlias, "llm-provider", "", "LLM provider")
	fs.IntVar(&o.AIPromptLength, "ai-prompt-length", 0, "AI prompt length")
	fs.StringVar(&o.AISession, "ai-session", "", "AI session id")
	fs.StringVar(&o.AISubscriptionPlan, "ai-subscription-plan", "", "AI subscription plan")
	fs.StringVar(&o.AlternateBranch, "alternate-branch", "", "fallback branch")
	fs.StringVar(&o.AlternateLanguage, "alternate-language", "", "fallback language")
	fs.StringVar(&o.AlternateProject, "alternate-project", "", "fallback project")
	apiURLAlias := ""
	fs.StringVar(&o.APIURL, "api-url", "", "API base URL")
	fs.StringVar(&apiURLAlias, "apiurl", "", "deprecated API base URL")
	fs.StringVar(&o.Branch, "branch", "", "branch")
	fs.StringVar(&o.Category, "category", "", "category")
	revisionAlias := ""
	fs.StringVar(&o.CommitHash, "commit-hash", "", "commit hash")
	fs.StringVar(&revisionAlias, "revision", "", "revision")
	fs.StringVar(&o.ConfigPath, "config", DefaultWakaTimeConfigPath(), "config path")
	fs.StringVar(&o.ConfigRead, "config-read", "", "read config key")
	fs.StringVar(&o.ConfigSection, "config-section", "settings", "config section")
	configWrite := multiValue{}
	fs.Var(&configWrite, "config-write", "write config key/value")
	fs.IntVar(&o.CursorPosition, "cursorpos", 0, "cursor position")
	disableOfflineAlias := false
	fs.BoolVar(&o.DisableOffline, "disable-offline", false, "disable offline queue")
	fs.BoolVar(&disableOfflineAlias, "disableoffline", false, "deprecated disable offline")
	entityAlias := ""
	fs.StringVar(&o.Entity, "entity", "", "heartbeat entity")
	fs.StringVar(&entityAlias, "file", "", "deprecated heartbeat entity")
	fs.StringVar(&o.EntityType, "entity-type", "", "entity type")
	fs.StringVar(&o.Editor, "editor", "", "editor")
	fs.StringVar(&o.EditorVersion, "editor-version", "", "editor version")
	fs.Var((*sliceValue)(&o.Exclude), "exclude", "exclude regex")
	fs.BoolVar(&o.ExcludeUnknownProject, "exclude-unknown-project", false, "skip unknown project")
	fs.BoolVar(&o.ExtraHeartbeats, "extra-heartbeats", false, "read extra heartbeats JSON from stdin")
	fs.BoolVar(&o.FileExperts, "file-experts", false, "fetch file experts")
	fs.StringVar(&o.Goal, "goal", "", "goal id")
	fs.IntVar(&o.HeartbeatRateLimit, "heartbeat-rate-limit-seconds", 0, "heartbeat send rate limit")
	fs.StringVar(&o.HideBranchNames, "hide-branch-names", "", "hide branch names")
	fs.StringVar(&o.HideDependencies, "hide-dependencies", "", "hide dependencies")
	hideFileNamesAliasOne := ""
	hideFileNamesAliasTwo := ""
	fs.StringVar(&o.HideFileNames, "hide-file-names", "", "hide file names")
	fs.StringVar(&hideFileNamesAliasOne, "hide-filenames", "", "deprecated hide file names")
	fs.StringVar(&hideFileNamesAliasTwo, "hidefilenames", "", "deprecated hide file names")
	fs.BoolVar(&o.HideProjectFolder, "hide-project-folder", false, "hide project folder")
	fs.StringVar(&o.HideProjectNames, "hide-project-names", "", "hide project names")
	fs.StringVar(&o.Hostname, "hostname", "", "hostname")
	fs.IntVar(&o.HumanLineChanges, "human-line-changes", 0, "human line changes")
	fs.Var((*sliceValue)(&o.Include), "include", "include regex")
	fs.BoolVar(&o.IncludeOnlyProjectFile, "include-only-with-project-file", false, "require .wakatime-project")
	fs.BoolVar(&o.GuessLanguage, "guess-language", false, "guess language from file contents")
	fs.StringVar(&o.InternalConfigPath, "internal-config", DefaultInternalConfigPath(), "internal config path")
	fs.BoolVar(&o.IsUnsavedEntity, "is-unsaved-entity", false, "track unsaved entity")
	apiKeyAlias := ""
	fs.StringVar(&o.Key, "key", "", "API key")
	fs.StringVar(&apiKeyAlias, "api-key", "", "API key")
	fs.StringVar(&o.Language, "language", "", "language")
	fs.IntVar(&o.LineNumber, "lineno", 0, "line number")
	fs.IntVar(&o.LinesInFile, "lines-in-file", 0, "lines in file")
	fs.StringVar(&o.LocalFile, "local-file", "", "local file for stats")
	logFileAlias := ""
	fs.StringVar(&o.LogFile, "log-file", "", "log file")
	fs.StringVar(&logFileAlias, "logfile", "", "deprecated log file")
	fs.BoolVar(&o.LogToStdout, "log-to-stdout", false, "log to stdout")
	fs.StringVar(&o.Metadata, "metadata", "", "JSON metadata")
	fs.BoolVar(&o.Metrics, "metrics", false, "collect metrics")
	fs.BoolVar(&o.NoSSLVerify, "no-ssl-verify", false, "disable SSL verification")
	fs.BoolVar(&o.OfflineCount, "offline-count", false, "count offline heartbeats")
	fs.StringVar(&o.Output, "output", "", "output format")
	fs.StringVar(&o.Plugin, "plugin", "", "plugin")
	fs.StringVar(&o.PluginVersion, "plugin-version", "", "plugin version")
	fs.IntVar(&o.PrintOffline, "print-offline-heartbeats", defaultPrintOfflineMax, "print offline heartbeats")
	fs.StringVar(&o.Project, "project", "", "project")
	fs.StringVar(&o.ProjectFolder, "project-folder", "", "project folder")
	fs.StringVar(&o.Proxy, "proxy", "", "proxy URL")
	fs.StringVar(&o.QueuePath, "offline-queue-file", DefaultQueuePath(), "offline queue")
	fs.StringVar(&o.LegacyQueuePath, "offline-queue-file-legacy", DefaultLegacyQueuePath(), "legacy offline queue")
	fs.StringVar(&o.Range, "range", "", "stats range")
	fs.BoolVar(&o.SendDiagnosticsOnError, "send-diagnostics-on-errors", false, "send diagnostics")
	fs.StringVar(&o.SSLCertsFile, "ssl-certs-file", "", "SSL certs file")
	syncAIHeartbeatsAlias := false
	fs.BoolVar(&o.SyncAIActivity, "sync-ai-activity", false, "sync AI activity")
	fs.BoolVar(&syncAIHeartbeatsAlias, "sync-ai-heartbeats", false, "sync AI activity")
	syncAIDisableAlias := false
	fs.BoolVar(&o.SyncAIDisabled, "sync-ai-disabled", false, "disable AI sync")
	fs.BoolVar(&syncAIDisableAlias, "sync-ai-disable", false, "disable AI sync")
	fs.Float64Var(&o.SyncAIAfter, "sync-ai-after", 0, "deprecated AI sync timestamp")
	fs.IntVar(&o.SyncOffline, "sync-offline-activity", defaultQueueMaxSync, "sync offline heartbeats")
	fs.IntVar(&o.Timeout, "timeout", 0, "request timeout seconds")
	fs.Float64Var(&o.Time, "time", 0, "unix timestamp")
	fs.BoolVar(&o.Today, "today", false, "fetch today")
	fs.StringVar(&o.TodayGoal, "today-goal", "", "fetch today goal")
	todayHideCategories := ""
	fs.StringVar(&todayHideCategories, "today-hide-categories", "", "hide today categories")
	fs.IntVar(&o.TodayMaxCategories, "today-max-categories", 0, "max today categories")
	fs.BoolVar(&o.UserAgent, "user-agent", false, "print user agent")
	fs.BoolVar(&o.Verbose, "verbose", false, "verbose logging")
	fs.BoolVar(&o.Version, "version", false, "print version")
	fs.BoolVar(&o.Write, "write", false, "write heartbeat")
	if err := fs.Parse(normalizeConfigWrite(args)); err != nil {
		return o, err
	}
	o.ConfigWrite = configWrite
	visited := visitedFlags(fs)
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "print-offline-heartbeats" {
			o.PrintOfflineSet = true
		}
		if f.Name == "sync-offline-activity" {
			o.SyncOfflineSet = true
		}
		if f.Name == "category" {
			o.CategorySet = true
		}
	})
	o.AIInputTokensSet = visited["ai-input-tokens"]
	o.AILineChangesSet = visited["ai-line-changes"]
	o.AIOutputTokensSet = visited["ai-output-tokens"]
	o.AIPromptLengthSet = visited["ai-prompt-length"]
	o.ConfigReadSet = visited["config-read"]
	o.CursorPositionSet = visited["cursorpos"]
	o.EntitySet = visited["entity"] || visited["file"]
	o.HumanLineChangesSet = visited["human-line-changes"]
	o.LineNumberSet = visited["lineno"]
	o.LinesInFileSet = visited["lines-in-file"]
	o.TodayGoalSet = visited["today-goal"]
	o.WriteSet = visited["write"]
	if o.AIAgent == "" && aiAgentAlias != "" {
		o.AIAgent = aiAgentAlias
	}
	if o.AIModel == "" {
		o.AIModel = first(aiModelNameAlias, modelNameAlias, llmModelAlias, modelAlias)
	}
	if o.AIProvider == "" {
		o.AIProvider = first(providerAlias, llmProviderAlias)
	}
	if o.APIURL == "" && apiURLAlias != "" {
		o.APIURL = apiURLAlias
	}
	if o.CommitHash == "" && revisionAlias != "" {
		o.CommitHash = revisionAlias
	}
	if o.Key == "" && apiKeyAlias != "" {
		o.Key = apiKeyAlias
	}
	if o.LogFile == "" && logFileAlias != "" {
		o.LogFile = logFileAlias
	}
	if o.Entity == "" && entityAlias != "" {
		o.Entity = entityAlias
	}
	if o.HideFileNames == "" {
		if hideFileNamesAliasOne != "" {
			o.HideFileNames = hideFileNamesAliasOne
		} else if hideFileNamesAliasTwo != "" {
			o.HideFileNames = hideFileNamesAliasTwo
		}
	}
	if !visited["disable-offline"] && visited["disableoffline"] {
		o.DisableOffline = disableOfflineAlias
	}
	if !visited["sync-ai-activity"] && visited["sync-ai-heartbeats"] {
		o.SyncAIActivity = syncAIHeartbeatsAlias
	}
	if !visited["sync-ai-disabled"] && visited["sync-ai-disable"] {
		o.SyncAIDisabled = syncAIDisableAlias
	}
	outputSet := visited["output"]
	if outputSet && o.Output != "" {
		if err := validateOutput(o.Output); err != nil {
			return o, err
		}
	}
	if o.EntityType == "" {
		o.EntityType = "file"
	}
	if !validEntityType(o.EntityType) {
		return o, fmt.Errorf("failed to parse entity type: invalid entity type %q", o.EntityType)
	}
	if !validCategory(o.Category) {
		return o, fmt.Errorf("failed to parse category: invalid category %q", o.Category)
	}
	o.ConfigPath = defaultConfigPathIfEmpty(o.ConfigPath)
	o.InternalConfigPath = defaultInternalConfigPathIfEmpty(o.InternalConfigPath)
	if expandedConfigPath, err := expandHomeStrict(o.ConfigPath); err != nil {
		return o, fmt.Errorf("failed to expand config param: %w", err)
	} else {
		o.ConfigPath = expandedConfigPath
	}
	if expandedInternalConfigPath, err := expandHomeStrict(o.InternalConfigPath); err != nil {
		return o, fmt.Errorf("failed to expand internal-config param: %w", err)
	} else {
		o.InternalConfigPath = expandedInternalConfigPath
	}
	cfg, projectCfg, projectConfigLoaded, err := LoadConfigForEntity(o.ConfigPath, first(o.LocalFile, o.Entity), o.EntityType, o.IsUnsavedEntity)
	if err != nil {
		return o, err
	}
	nativeCfg := loadNativeConfig()
	o.Config = cfg
	internalCfg, _ := LoadConfig(o.InternalConfigPath)
	o.InternalConfig = internalCfg
	o.APIURL = first(o.APIURL, os.Getenv("STINT_API_URL"), configFirst(nativeCfg, "api_url", "api-url", "apiurl"), configFirst(cfg, "api_url", "api-url", "apiurl"), defaultAPIURL)
	apiKey := first(o.Key, os.Getenv("STINT_API_KEY"))
	if apiKey == "" {
		apiKey, err = resolveAPIKeyFromConfigs("", []Config{nativeCfg, cfg}, os.Getenv("WAKATIME_API_KEY"))
	}
	if err != nil {
		return o, err
	}
	o.APIKey = apiKey
	if projectConfigLoaded {
		o.APIURL = projectFirst(projectCfg, o.APIURL, "api_url", "api-url", "apiurl")
		if projectCfg.Has("settings", "api_key") || projectCfg.Has("settings", "apikey") || projectCfg.Has("settings", "api_key_vault_cmd") {
			apiKey, err := resolveAPIKeyFromConfig(projectCfg, o.Key, os.Getenv("STINT_API_KEY"), os.Getenv("WAKATIME_API_KEY"))
			if err != nil {
				return o, err
			}
			o.APIKey = apiKey
		}
	}
	if o.APIURL != "" {
		apiURL, err := normalizeAPIURL(o.APIURL)
		if err != nil {
			return o, fmt.Errorf("invalid api url: %w", err)
		}
		o.APIURL = apiURL
	}
	if !visited["heartbeat-rate-limit-seconds"] {
		if value, ok := configInt(cfg, "settings", "heartbeat_rate_limit_seconds"); ok {
			o.HeartbeatRateLimit = value
		} else {
			o.HeartbeatRateLimit = 120
		}
	}
	if !visited["timeout"] {
		if value, ok := configInt(cfg, "settings", "timeout"); ok {
			o.Timeout = value
		} else {
			o.Timeout = defaultTimeoutSeconds
		}
	}
	o.BackoffAt = parseBackoffAt(internalCfg.Get("internal", "backoff_at"))
	o.BackoffRetries = internalCfg.Int("internal", "backoff_retries")
	o.Proxy = first(o.Proxy, cfg.Get("settings", "proxy"), proxyFromEnvironment(o.APIURL))
	o.SSLCertsFile = first(o.SSLCertsFile, cfg.Get("settings", "ssl_certs_file"))
	if o.SSLCertsFile != "" {
		expandedSSLCertsFile, err := expandHomeStrict(o.SSLCertsFile)
		if err != nil {
			return o, fmt.Errorf("failed expanding ssl certs file: %w", err)
		}
		o.SSLCertsFile = expandedSSLCertsFile
	}
	o.Hostname = first(o.Hostname, cfg.Get("settings", "hostname"))
	o.LogFile = first(o.LogFile, cfg.Get("settings", "log_file"), DefaultLogFilePath())
	if expandedLogFile, err := expandHomeStrict(o.LogFile); err != nil {
		return o, fmt.Errorf("failed to expand log file: %w", err)
	} else {
		o.LogFile = expandedLogFile
	}
	if !visited["no-ssl-verify"] && cfg.Bool("settings", "no_ssl_verify") {
		o.NoSSLVerify = true
	}
	if !visited["send-diagnostics-on-errors"] && cfg.Bool("settings", "send_diagnostics_on_errors") {
		o.SendDiagnosticsOnError = true
	}
	if cfg.Bool("settings", "sync_ai_disabled") {
		o.SyncAIDisabled = true
	}
	if !visited["verbose"] && cfg.Bool("settings", "debug") {
		o.Verbose = true
	}
	if !visited["guess-language"] && cfg.Bool("settings", "guess_language") {
		o.GuessLanguage = true
	}
	if !visited["metrics"] && cfg.Bool("settings", "metrics") {
		o.Metrics = true
	}
	if !visited["disable-offline"] && !visited["disableoffline"] {
		if v, ok := configBool(cfg, "settings", "offline"); ok {
			o.DisableOffline = !v
		}
	}
	if !visited["hide-project-folder"] {
		if v, ok := configBool(cfg, "settings", "hide_project_folder"); ok {
			o.HideProjectFolder = v
		}
	}
	if err := resolveStatusBarOptions(&o, cfg, visited["today-hide-categories"], todayHideCategories, visited["today-max-categories"]); err != nil {
		return o, err
	}
	o.HideFileNames = first(o.HideFileNames, cfg.Get("settings", "hide_file_names"), cfg.Get("settings", "hide_filenames"), cfg.Get("settings", "hidefilenames"))
	o.HideProjectNames = first(o.HideProjectNames, cfg.Get("settings", "hide_project_names"), cfg.Get("settings", "hide_projectnames"), cfg.Get("settings", "hideprojectnames"))
	o.HideBranchNames = first(o.HideBranchNames, cfg.Get("settings", "hide_branch_names"), cfg.Get("settings", "hide_branchnames"), cfg.Get("settings", "hidebranchnames"))
	o.HideDependencies = first(o.HideDependencies, cfg.Get("settings", "hide_dependencies"))
	if !visited["include-only-with-project-file"] {
		if v, ok := configBool(cfg, "settings", "include_only_with_project_file"); ok {
			o.IncludeOnlyProjectFile = v
		}
	}
	if !visited["exclude-unknown-project"] {
		if v, ok := configBool(cfg, "settings", "exclude_unknown_project"); ok {
			o.ExcludeUnknownProject = v
		}
	}
	o.Include = resolveListOption(splitFlagListValues(o.Include), []string{cfg.Get("settings", "include")}, visited["include"], false)
	o.Exclude = resolveListOption(splitFlagListValues(o.Exclude), []string{cfg.Get("settings", "exclude"), cfg.Get("settings", "ignore")}, visited["exclude"], false)
	if projectConfigLoaded {
		if projectCfg.Has("settings", "include") {
			o.Include = splitConfigList(projectCfg.Get("settings", "include"))
		}
		if projectCfg.Has("settings", "exclude") || projectCfg.Has("settings", "ignore") {
			o.Exclude = append(splitConfigList(projectCfg.Get("settings", "exclude")), splitConfigList(projectCfg.Get("settings", "ignore"))...)
		}
		if value, ok := configInt(projectCfg, "settings", "heartbeat_rate_limit_seconds"); ok {
			o.HeartbeatRateLimit = value
		}
		o.HideFileNames = projectFirst(projectCfg, o.HideFileNames, "hide_file_names", "hide_filenames", "hidefilenames")
		o.HideProjectNames = projectFirst(projectCfg, o.HideProjectNames, "hide_project_names", "hide_projectnames", "hideprojectnames")
		o.HideBranchNames = projectFirst(projectCfg, o.HideBranchNames, "hide_branch_names", "hide_branchnames", "hidebranchnames")
		o.HideDependencies = projectFirst(projectCfg, o.HideDependencies, "hide_dependencies")
		if v, ok := configBool(projectCfg, "settings", "hide_project_folder"); ok {
			o.HideProjectFolder = v
		}
		if v, ok := configBool(projectCfg, "settings", "include_only_with_project_file"); ok {
			o.IncludeOnlyProjectFile = v
		}
		if v, ok := configBool(projectCfg, "settings", "exclude_unknown_project"); ok {
			o.ExcludeUnknownProject = v
		}
		if v, ok := configBool(projectCfg, "settings", "guess_language"); ok {
			o.GuessLanguage = v
		}
		if !visited["disable-offline"] && !visited["disableoffline"] {
			if v, ok := configBool(projectCfg, "settings", "offline"); ok {
				o.DisableOffline = !v
			}
		}
	}
	if err := validateHideRegexOptions(o); err != nil {
		return o, err
	}
	if o.HeartbeatRateLimit < 0 {
		o.HeartbeatRateLimit = 0
	}
	if o.QueuePath == "" {
		o.QueuePath = DefaultQueuePath()
	}
	o.LegacyQueuePath = defaultLegacyQueuePathIfEmpty(o.LegacyQueuePath)
	if expandedQueuePath, err := expandHomeStrict(o.QueuePath); err != nil {
		return o, fmt.Errorf("failed expanding offline-queue-file param: %w", err)
	} else {
		o.QueuePath = expandedQueuePath
	}
	if expandedLegacyQueuePath, err := expandHomeStrict(o.LegacyQueuePath); err != nil {
		return o, fmt.Errorf("failed expanding offline-queue-file-legacy param: %w", err)
	} else {
		o.LegacyQueuePath = expandedLegacyQueuePath
	}
	return o, nil
}

func validateOutput(value string) error {
	switch strings.TrimSpace(value) {
	case "", "text", "json", "raw-json":
		return nil
	default:
		return fmt.Errorf("failed to parse output: invalid output %q", value)
	}
}

func validateHideRegexOptions(o Options) error {
	for _, item := range []struct {
		name  string
		value string
	}{
		{name: "hide branch names", value: o.HideBranchNames},
		{name: "hide dependencies", value: o.HideDependencies},
		{name: "hide file names", value: o.HideFileNames},
		{name: "hide project names", value: o.HideProjectNames},
	} {
		if err := validateHideRegexOption(item.value); err != nil {
			return fmt.Errorf("failed to parse regex %s param %q: %w", item.name, item.value, err)
		}
	}
	return nil
}

func validateHideRegexOption(raw string) error {
	for _, pattern := range splitConfigList(raw) {
		if _, err := compileWakaPattern(pattern); err != nil {
			return err
		}
	}
	return nil
}

func resolveStatusBarOptions(o *Options, cfg Config, todayHideSet bool, todayHideRaw string, todayMaxSet bool) error {
	flagMaxCategories := o.TodayMaxCategories
	o.TodayStatusBarEnabled = true
	o.TodayCodingActivity = true
	o.TodayHideCategories = true
	o.TodayMaxCategories = 2
	if raw := strings.TrimSpace(cfg.Get("settings", "status_bar_enabled")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("failed to parse status_bar_enabled: %w", err)
		}
		o.TodayStatusBarEnabled = value
	}
	if raw := strings.TrimSpace(cfg.Get("settings", "status_bar_coding_activity")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("failed to parse status_bar_coding_activity: %w", err)
		}
		o.TodayCodingActivity = value
	}
	if todayHideSet {
		value, err := strconv.ParseBool(strings.TrimSpace(todayHideRaw))
		if err != nil {
			return fmt.Errorf("failed to parse today-hide-categories: %w", err)
		}
		o.TodayHideCategories = value
	} else if raw := strings.TrimSpace(cfg.Get("settings", "status_bar_show_categories")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("failed to parse status_bar_show_categories: %w", err)
		}
		o.TodayHideCategories = !value
	}
	if raw := strings.TrimSpace(cfg.Get("settings", "status_bar_hide_minutes")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("failed to parse status_bar_hide_minutes: %w", err)
		}
		o.TodayHideMinutes = value
	}
	maxRaw := strings.TrimSpace(cfg.Get("settings", "status_bar_max_categories"))
	if todayMaxSet {
		maxRaw = strconv.Itoa(flagMaxCategories)
	}
	if maxRaw != "" {
		value, err := strconv.Atoi(maxRaw)
		if err != nil {
			return fmt.Errorf("failed to parse today-max-categories: %w", err)
		}
		if value < 0 {
			return fmt.Errorf("today-max-categories must be a positive number, got %d", value)
		}
		o.TodayMaxCategories = value
	}
	return nil
}

func configFirst(cfg Config, keys ...string) string {
	for _, key := range keys {
		if value := cfg.Get("settings", key); value != "" {
			return value
		}
	}
	return ""
}

func projectFirst(cfg Config, fallback string, keys ...string) string {
	for _, key := range keys {
		if cfg.Has("settings", key) {
			return first(cfg.Get("settings", key), fallback)
		}
	}
	return fallback
}

func visitedFlags(fs *flag.FlagSet) map[string]bool {
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})
	return visited
}

func resolveListOption(flags []string, rawValues []string, flagSet, configPrecedence bool) []string {
	var configValues []string
	for _, raw := range rawValues {
		configValues = append(configValues, splitConfigList(raw)...)
	}
	if configPrecedence {
		if len(configValues) > 0 {
			return configValues
		}
		return flags
	}
	if flagSet {
		return append(append([]string{}, flags...), configValues...)
	}
	return configValues
}

func splitFlagListValues(values []string) []string {
	var out []string
	for _, value := range values {
		for _, field := range strings.Split(value, ",") {
			field = strings.TrimSpace(field)
			if field != "" {
				out = append(out, field)
			}
		}
	}
	return out
}

func configBool(cfg Config, section, key string) (bool, bool) {
	raw := strings.TrimSpace(cfg.Get(section, key))
	if raw == "" {
		return false, false
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func configInt(cfg Config, section, key string) (int, bool) {
	raw := strings.TrimSpace(cfg.Get(section, key))
	if raw == "" {
		return 0, false
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return value, true
}

func validEntityType(entityType string) bool {
	switch entityType {
	case "file", "domain", "url", "event", "app":
		return true
	default:
		return false
	}
}

func validCategory(category string) bool {
	switch category {
	case "", "null", "advising", "ai coding", "browsing", "building", "code reviewing", "coding", "communicating", "debugging", "designing", "indexing", "learning", "manual testing", "meeting", "notes", "planning", "researching", "running tests", "supporting", "translating", "writing docs", "writing tests":
		return true
	default:
		return false
	}
}

func resolveAPIKey(key string, cfg Config, fallbacks ...string) (string, error) {
	if key != "" {
		return key, nil
	}
	return resolveAPIKeyFromConfig(cfg, fallbacks...)
}

func resolveAPIKeyFromConfigs(key string, configs []Config, fallbacks ...string) (string, error) {
	if key != "" {
		return key, nil
	}
	for _, cfg := range configs {
		apiKey, err := resolveAPIKeyFromConfig(cfg)
		if err != nil {
			return "", err
		}
		if apiKey != "" {
			return apiKey, nil
		}
	}
	return first(fallbacks...), nil
}

func resolveAPIKeyFromConfig(cfg Config, fallbacks ...string) (string, error) {
	if key := first(cfg.Get("settings", "api_key"), cfg.Get("settings", "apikey")); key != "" {
		return key, nil
	}
	key, err := apiKeyFromVault(cfg)
	if err != nil {
		return "", err
	}
	return first(append([]string{key}, fallbacks...)...), nil
}

func apiKeyFromVault(cfg Config) (string, error) {
	cmdText := strings.TrimSpace(cfg.Get("settings", "api_key_vault_cmd"))
	if cmdText == "" {
		return "", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmdName := "sh"
	cmdArgs := []string{"-c", cmdText}
	if runtime.GOOS == "windows" {
		cmdName = "cmd"
		cmdArgs = []string{"/C", cmdText}
	}

	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...) //nolint:gosec // User-configured WakaTime-compatible vault command.
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to read api key from vault: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func normalizeConfigWrite(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--config-write" && i+2 < len(args) && !strings.Contains(args[i+1], "=") {
			out = append(out, "--config-write", args[i+1]+"="+args[i+2])
			i += 2
			continue
		}
		out = append(out, args[i])
	}
	return out
}

type sliceValue []string

func (s *sliceValue) String() string { return strings.Join(*s, ",") }
func (s *sliceValue) Set(v string) error {
	*s = append(*s, v)
	return nil
}

type multiValue map[string]string

func (m *multiValue) String() string {
	if m == nil || len(*m) == 0 {
		return ""
	}
	parts := make([]string, 0, len(*m))
	for key, value := range *m {
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, ",")
}
func (m *multiValue) Set(v string) error {
	if *m == nil {
		*m = map[string]string{}
	}
	items := []string{strings.Trim(v, `"`)}
	switch strings.Count(v, "=") {
	case 0:
		return fmt.Errorf("%s must be formatted as key=value", v)
	case 1:
	default:
		reader := csv.NewReader(strings.NewReader(v))
		parsed, err := reader.Read()
		if err != nil {
			return err
		}
		items = parsed
	}
	for _, item := range items {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return fmt.Errorf("%s must be formatted as key=value", item)
		}
		(*m)[strings.TrimSpace(parts[0])] = parts[1]
	}
	return nil
}

func (o Options) shouldQueueForRateLimit() bool {
	if o.HeartbeatRateLimit <= 0 {
		return false
	}
	lastSentText := o.InternalConfig.Get("internal", "heartbeats_last_sent_at")
	if lastSentText == "" {
		return false
	}
	lastSent, err := time.Parse(wakaTimeDateFormat, strings.TrimSpace(lastSentText))
	if err != nil {
		return false
	}
	now := time.Now()
	if lastSent.After(now) {
		lastSent = now
	}
	return now.Before(lastSent.Add(time.Duration(o.HeartbeatRateLimit) * time.Second))
}

func (o Options) recordLastSent() error {
	return WriteConfigValue(o.InternalConfigPath, "internal", "heartbeats_last_sent_at", time.Now().Format(wakaTimeDateFormat))
}
