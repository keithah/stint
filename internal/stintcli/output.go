package stintcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
)

var (
	versionValue   = "dev"
	commitValue    = "unset"
	buildDateValue = "unset"
)

func writeOutput(stdout io.Writer, format string, body []byte) error {
	format = strings.TrimSpace(format)
	switch format {
	case "", "json", "raw-json":
		_, err := stdout.Write(bytes.TrimSpace(body))
		if err == nil {
			_, err = fmt.Fprintln(stdout)
		}
		return err
	case "text":
		var v any
		if err := json.Unmarshal(body, &v); err != nil {
			_, werr := stdout.Write(body)
			return werr
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func printHelp(w io.Writer) {
	fmt.Fprint(w, `Stint CLI

Usage:
  stint --entity FILE [--write] [--project NAME]
  stint heartbeat --entity FILE [--write]
  stint heartbeats [YYYY-MM-DD]
  stint today
  stint today-goal GOAL_ID
  stint file-experts FILE
  stint stats [RANGE]
  stint projects [PROJECT|PROJECT commits [HASH] [--branch BRANCH] [--page N]]
  stint goals [GOAL_ID|list|create FILE|--stdin|update GOAL_ID FILE|--stdin|delete GOAL_ID]
  stint account [get|update FILE|--stdin|delete --confirm]
  stint meta
  stint api-docs
  stint leaders [--language NAME] [--country CODE]
  stint editors
  stint program-languages
  stint users USER [stats [RANGE|--range RANGE]|summaries [--start YYYY-MM-DD --end YYYY-MM-DD]|share TOKEN stats|summaries]
  stint share TOKEN stats|summaries [--range RANGE|--start YYYY-MM-DD --end YYYY-MM-DD]
  stint health [ingestion]
  stint dev seed-key [--github-id ID] [--username NAME]
  stint dev heartbeats-purge [--retention-days DAYS]
  stint dev leaderboard-update [--range RANGE]
  stint dev goals-evaluate [--now-unix TS]
  stint all-time
  stint machine-names
  stint user-agents
  stint external-durations [list|create FILE|--stdin|bulk FILE|--stdin|delete ID...]
  stint custom-pricing [list|upsert FILE|--stdin|delete MODEL]
  stint pricing-sources
  stint pricing-models
  stint billing-prefs [list|upsert FILE|--stdin|delete AGENT]
  stint ai-costs [list|replace FILE|--stdin]
  stint leaderboards [BOARD_ID|list|create FILE|--stdin|update BOARD_ID FILE|--stdin|delete BOARD_ID|add-member BOARD_ID USERNAME|remove-member BOARD_ID USER_ID]
  stint share-tokens [list|create NAME|delete ID]
  stint api-keys [list|create NAME [--scope SCOPE]|delete ID]
  stint oauth-apps [list|create NAME --redirect-uri URI [--scope SCOPE]|delete ID]
  stint oauth token --client-id ID --client-secret SECRET (--code CODE --redirect-uri URI|--refresh-token TOKEN)
  stint oauth revoke TOKEN --client-id ID --client-secret SECRET
  stint events
  stint usage-events [list|summary|blocks]
  stint data-dumps [list|create heartbeats|daily|download DUMP_ID]
  stint custom-rules [list|progress|replace FILE|--stdin|delete RULE_ID|abort]
  stint import wakatime FILE|--stdin
  stint insights TYPE RANGE
  stint durations [YYYY-MM-DD] [--slice-by project|language|...]
  stint summaries [START] [END]
  stint offline count|print|sync
  stint config init|read|write
  stint collect [COLLECTOR_FLAGS]
  stint doctor

WakaTime-compatible root flags:
  --ai-line-changes, --alternate-branch, --alternate-language,
  --alternate-project, --api-key, --api-url, --apiurl, --category, --config, --config-read,
  --config-section, --config-write, --cursorpos, --disable-offline,
  --disableoffline, --entity, --entity-type, --exclude,
  --exclude-unknown-project, --extra-heartbeats, --file, --file-experts,
  --guess-language, --heartbeat-rate-limit-seconds, --hide-branch-names,
  --help, --hide-dependencies, --hide-file-names, --hide-filenames,
  --hidefilenames, --hide-project-folder, --hide-project-names, --hostname,
  --human-line-changes, --include, --include-only-with-project-file,
  --internal-config, --is-unsaved-entity, --key, --language, --lineno,
  --lines-in-file, --local-file, --log-file, --logfile, --log-to-stdout,
  --metrics, --no-ssl-verify, --offline-count, --offline-queue-file,
  --offline-queue-file-legacy, --output, --plugin, --print-offline-heartbeats,
  --project, --project-folder, --proxy, --send-diagnostics-on-errors,
  --ssl-certs-file, --sync-ai-activity, --sync-ai-after, --sync-ai-disabled,
  --sync-ai-disable, --sync-ai-heartbeats, --sync-offline-activity, --time,
  --timeout, --today, --today-goal, --today-hide-categories,
  --today-max-categories, --user-agent, --verbose, --version, --write

Stint extensions:
  --ai-agent, --ai-agent-name, --ai-agent-complexity, --ai-agent-version,
  --ai-input-tokens, --ai-model, --ai-model-name, --ai-output-tokens,
  --ai-prompt-length, --ai-provider, --ai-session, --ai-subscription-plan,
  --branch, --commit-hash, --editor, --editor-version, --goal, --llm-model,
  --llm-provider, --metadata, --model, --model-name, --plugin-version,
  --provider, --range, --revision
`)
}

func userAgent(plugin string) string {
	if strings.TrimSpace(plugin) == "" {
		plugin = "Unknown/0"
	}
	plugin = normalizePluginVersions(plugin)
	return fmt.Sprintf(
		"stint-cli/%s (%s) %s %s",
		Version(),
		userAgentPlatform(),
		strings.TrimSpace(runtime.Version()),
		strings.TrimSpace(plugin),
	)
}

func normalizePluginVersions(plugin string) string {
	fields := strings.Fields(plugin)
	changed := false
	for i, field := range fields {
		if strings.HasSuffix(field, "/") {
			fields[i] = field + "unknown"
			changed = true
		}
	}
	if !changed {
		return strings.TrimSpace(plugin)
	}
	return strings.Join(fields, " ")
}

func userAgentPlatform() string {
	core := runtime.GOARCH
	if out, err := exec.Command("uname", "-r").Output(); err == nil {
		if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
			core = trimmed
		}
	}
	return runtime.GOOS + "-" + core + "-" + runtime.GOARCH
}

func Version() string {
	return versionValue
}

func verboseVersion() string {
	return fmt.Sprintf(
		"stint-cli\n  Version: %s\n  Commit: %s\n  Built: %s\n  OS/Arch: %s/%s",
		Version(),
		commitValue,
		buildDateValue,
		runtime.GOOS,
		runtime.GOARCH,
	)
}
