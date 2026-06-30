package stintcli

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
)

const (
	defaultAPIURL          = "http://localhost:8080/api/v1"
	defaultHeartbeatLimit  = 10
	defaultPrintOfflineMax = 10
	defaultQueueMaxSync    = 1000
	defaultTimeoutSeconds  = 120
)

// Run executes the Stint CLI. It supports WakaTime-compatible root flags and
// clearer Stint-native subcommands backed by the same implementation.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = sendCommandDiagnostics(args, fmt.Sprint(recovered), "", string(debug.Stack()), true)
			panic(recovered)
		}
	}()
	if metricsRequested(args) {
		stopProfiling, err := startMetricsProfiling()
		if err == nil {
			defer stopProfiling()
		}
	}
	err := run(args, stdin, stdout, stderr)
	if err != nil {
		_ = sendCommandDiagnostics(args, err.Error(), diagnosticLogs(args), string(debug.Stack()), false)
	}
	return err
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "help", "--help", "-h":
			printHelp(stdout)
			return nil
		case "version", "--version":
			printVersion(stdout, args[1:])
			return nil
		case "heartbeat":
			return runHeartbeat(args[1:], stdin, stdout)
		case "heartbeats":
			return runHeartbeatsList(args[1:], stdout)
		case "config":
			return runConfig(args[1:], stdout)
		case "today":
			return runToday(args[1:], stdout)
		case "today-goal":
			return runTodayGoal(args[1:], stdout)
		case "file-experts":
			return runFileExperts(args[1:], stdout)
		case "stats":
			return runStats(args[1:], stdout)
		case "projects":
			return runProjects(args[1:], stdout)
		case "goals":
			return runGoals(args[1:], stdin, stdout)
		case "account", "me":
			return runAccount(args[1:], stdin, stdout)
		case "health":
			return runHealth(args[1:], stdout)
		case "dev":
			return runDev(args[1:], stdout)
		case "meta":
			return runSimpleGET(args[1:], stdout, "/meta")
		case "api-docs", "openapi":
			return runSimpleGET(args[1:], stdout, "/docs")
		case "leaders":
			return runLeaders(args[1:], stdout)
		case "editors":
			return runSimpleGET(args[1:], stdout, "/editors")
		case "program-languages", "program_languages":
			return runSimpleGET(args[1:], stdout, "/program_languages")
		case "users":
			return runPublicUsers(args[1:], stdout)
		case "share":
			return runPublicShare(args[1:], stdout)
		case "all-time", "all-time-since-today":
			return runSimpleGET(args[1:], stdout, "/users/current/all_time_since_today")
		case "machine-names", "machine_names":
			return runSimpleGET(args[1:], stdout, "/users/current/machine_names")
		case "user-agents", "user_agents":
			return runSimpleGET(args[1:], stdout, "/users/current/user_agents")
		case "external-durations", "external_durations":
			return runExternalDurations(args[1:], stdin, stdout)
		case "custom-pricing", "custom_pricing":
			return runCustomPricing(args[1:], stdin, stdout)
		case "pricing-sources", "pricing_sources":
			return runSimpleGET(args[1:], stdout, "/users/current/pricing/sources")
		case "pricing-models", "pricing_models":
			return runSimpleGET(args[1:], stdout, "/users/current/pricing/models")
		case "billing-prefs", "billing_prefs":
			return runBillingPrefs(args[1:], stdin, stdout)
		case "ai-costs", "ai_costs":
			return runAICosts(args[1:], stdin, stdout)
		case "events":
			return runSimpleGET(args[1:], stdout, "/users/current/events")
		case "leaderboards":
			return runLeaderboards(args[1:], stdin, stdout)
		case "share-tokens", "share_tokens":
			return runShareTokens(args[1:], stdout)
		case "api-keys", "api_keys":
			return runAPIKeys(args[1:], stdout)
		case "oauth-apps", "oauth_apps":
			return runOAuthApps(args[1:], stdout)
		case "oauth":
			if len(args) > 1 && args[1] == "apps" {
				return runOAuthApps(args[2:], stdout)
			}
			return runOAuth(args[1:], stdout)
		case "import":
			return runImport(args[1:], stdin, stdout)
		case "data-dumps", "data_dumps":
			return runDataDumps(args[1:], stdout)
		case "custom-rules", "custom_rules":
			return runCustomRules(args[1:], stdin, stdout)
		case "custom-rules-progress", "custom_rules_progress":
			return runSimpleGET(args[1:], stdout, "/users/current/custom_rules_progress")
		case "usage-events", "usage_events":
			return runUsageEvents(args[1:], stdout)
		case "insights":
			return runInsights(args[1:], stdout)
		case "durations":
			return runDurations(args[1:], stdout)
		case "summaries":
			return runSummaries(args[1:], stdout)
		case "offline":
			return runOffline(args[1:], stdout)
		case "doctor":
			return runDoctor(args[1:], stdout)
		case "collect":
			return runCollect(args[1:], stdin, stdout, stderr)
		}
	}
	return runRoot(args, stdin, stdout, stderr)
}

func printVersion(stdout io.Writer, args []string) {
	if hasFlag(args, "verbose") {
		fmt.Fprintln(stdout, verboseVersion())
		return
	}
	fmt.Fprintln(stdout, Version())
}

func hasFlag(args []string, name string) bool {
	long := "--" + name
	for _, arg := range args {
		if arg == long || strings.HasPrefix(arg, long+"=") {
			return true
		}
	}
	return false
}

func sendCommandDiagnostics(args []string, message, logs, stack string, panicked bool) error {
	opts, err := diagnosticOptions(args)
	if err != nil {
		return nil
	}
	if !panicked && (!opts.Verbose || !opts.SendDiagnosticsOnError) {
		return nil
	}
	client, err := NewClient(opts)
	if err != nil {
		return nil
	}
	return client.SendDiagnostics(context.Background(), message, logs, stack, panicked)
}

func diagnosticOptions(args []string) (Options, error) {
	if len(args) > 0 && diagnosticCommand(args[0]) {
		return parseCommon(diagnosticCommonArgs(args[1:]))
	}
	return parseCommon(args)
}

func diagnosticCommand(command string) bool {
	switch command {
	case "heartbeat", "heartbeats", "today", "today-goal", "file-experts", "stats", "projects",
		"goals", "account", "me", "health", "dev", "meta", "api-docs", "openapi", "leaders",
		"editors", "program-languages", "program_languages", "users", "share", "all-time",
		"all-time-since-today", "machine-names", "machine_names", "user-agents", "user_agents",
		"external-durations", "external_durations", "custom-pricing", "custom_pricing",
		"pricing-sources", "pricing_sources", "pricing-models", "pricing_models", "billing-prefs",
		"billing_prefs", "ai-costs", "ai_costs", "events", "leaderboards", "share-tokens",
		"share_tokens", "api-keys", "api_keys", "oauth-apps", "oauth_apps", "oauth", "import",
		"data-dumps", "data_dumps", "custom-rules", "custom_rules", "custom-rules-progress",
		"custom_rules_progress", "usage-events", "usage_events", "insights", "durations",
		"summaries", "offline", "doctor", "collect":
		return true
	default:
		return false
	}
}

func diagnosticCommonArgs(args []string) []string {
	out := make([]string, 0, len(args))
	valueFlags := map[string]bool{
		"--api-key":         true,
		"--api-url":         true,
		"--apiurl":          true,
		"--config":          true,
		"--internal-config": true,
		"--key":             true,
		"--log-file":        true,
		"--logfile":         true,
		"--plugin":          true,
		"--proxy":           true,
		"--ssl-certs-file":  true,
		"--timeout":         true,
	}
	boolFlags := map[string]bool{
		"--log-to-stdout":              true,
		"--metrics":                    true,
		"--no-ssl-verify":              true,
		"--send-diagnostics-on-errors": true,
		"--verbose":                    true,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		name, hasInlineValue := arg, false
		if before, _, ok := strings.Cut(arg, "="); ok {
			name = before
			hasInlineValue = true
		}
		if valueFlags[name] {
			out = append(out, arg)
			if !hasInlineValue && i+1 < len(args) {
				out = append(out, args[i+1])
				i++
			}
			continue
		}
		if boolFlags[name] {
			out = append(out, arg)
		}
	}
	return out
}

func diagnosticLogs(args []string) string {
	opts, err := diagnosticOptions(args)
	if err != nil || !opts.Verbose {
		return ""
	}
	data, err := os.ReadFile(expandHome(opts.LogFile))
	if err != nil {
		return ""
	}
	const maxDiagnosticLogBytes = 64 * 1024
	if len(data) > maxDiagnosticLogBytes {
		data = data[len(data)-maxDiagnosticLogBytes:]
	}
	return string(data)
}
