package stintcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var uuid4Regex = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)
var executablePath = os.Executable

func runRoot(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	opts, err := parseCommon(args)
	if err != nil {
		_ = fallbackSaveHeartbeatWithoutConfig(args)
		return err
	}
	opts.LogWriter = stdout
	switch {
	case opts.UserAgent:
		fmt.Fprintln(stdout, userAgent(opts.Plugin))
		return nil
	case opts.Version:
		if opts.Verbose {
			fmt.Fprintln(stdout, verboseVersion())
			return nil
		}
		fmt.Fprintln(stdout, Version())
		return nil
	case opts.ConfigReadSet:
		return writeConfigRead(stdout, opts.Config, opts.ConfigSection, opts.ConfigRead)
	case len(opts.ConfigWrite) > 0:
		return WriteConfigValues(opts.ConfigPath, opts.ConfigSection, opts.ConfigWrite)
	case opts.Today:
		return runTodayWithOptions(stdout, opts)
	case opts.TodayGoalSet:
		return runTodayGoalWithOptions(stdout, opts)
	case opts.FileExperts:
		return postFileExperts(stdout, opts, opts.Entity, opts.Project)
	case opts.EntitySet:
		return sendHeartbeat(stdin, stdout, opts)
	case opts.SyncOfflineSet:
		return syncOffline(stdout, opts, opts.SyncOffline)
	case opts.SyncAIActivity:
		return runSyncAIActivity(stdout, opts)
	case opts.OfflineCount:
		count, err := CountQueue(opts.QueuePath)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, count)
		return nil
	case opts.PrintOfflineSet:
		return printQueue(stdout, opts.QueuePath, opts.PrintOffline)
	default:
		printHelp(stdout)
		return fmt.Errorf("provide a command or one of --entity, --today, --today-goal, --file-experts, --offline-count, --print-offline-heartbeats, --sync-offline-activity, --config-read, --config-write")
	}
}

func fallbackSaveHeartbeatWithoutConfig(args []string) error {
	sanitized := withoutConfigFlag(args)
	sanitized = append(sanitized, "--config", filepath.Join(os.TempDir(), "stint-fallback-missing.cfg"))
	opts, err := parseCommon(sanitized)
	if err != nil {
		return err
	}
	if opts.Entity == "" || opts.DisableOffline {
		return nil
	}
	hb, err := BuildHeartbeat(opts)
	if err != nil {
		return err
	}
	return AppendQueue(opts.QueuePath, []Heartbeat{hb})
}

func withoutConfigFlag(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--config" {
			if i+1 < len(args) {
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, "--config=") {
			continue
		}
		out = append(out, arg)
	}
	return out
}

func runHeartbeat(args []string, stdin io.Reader, stdout io.Writer) error {
	opts, err := parseCommon(args)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	if opts.Entity == "" {
		return fmt.Errorf("--entity is required")
	}
	return sendHeartbeat(stdin, stdout, opts)
}

func runHeartbeatsList(args []string, stdout io.Writer) error {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		args = append([]string{"--date", args[0]}, args[1:]...)
	}
	date, commonArgs, err := splitDateReadArgs(args)
	if err != nil {
		return err
	}
	opts, err := parseCommon(commonArgs)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	path := "/users/current/heartbeats"
	if strings.TrimSpace(date) != "" {
		path += "?date=" + url.QueryEscape(strings.TrimSpace(date))
	}
	return runSimpleGETWithOptions(stdout, opts, path)
}

func splitDateReadArgs(args []string) (date string, commonArgs []string, err error) {
	commonArgs = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, ok := strings.CutPrefix(arg, "--date="); ok {
			date = value
			continue
		}
		if arg == "--date" {
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("--date requires a value")
			}
			i++
			date = args[i]
			continue
		}
		commonArgs = append(commonArgs, arg)
	}
	return date, commonArgs, nil
}

func runConfig(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: stint config init|read|write")
	}
	switch args[0] {
	case "init":
		fs := newFlagSet("stint config init")
		configPath := fs.String("config", DefaultWakaTimeConfigPath(), "config path")
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
		configPath := fs.String("config", DefaultWakaTimeConfigPath(), "config path")
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
		configPath := fs.String("config", DefaultWakaTimeConfigPath(), "config path")
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

func runSimpleGET(args []string, stdout io.Writer, path string) error {
	opts, err := parseCommon(args)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	return runSimpleGETWithOptions(stdout, opts, path)
}

func runSimpleGETWithOptions(stdout io.Writer, opts Options, path string) error {
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	body, err := client.Get(context.Background(), path)
	if err != nil {
		return err
	}
	return writeOutput(stdout, opts.Output, body)
}

func runToday(args []string, stdout io.Writer) error {
	opts, err := parseCommon(args)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	return runTodayWithOptions(stdout, opts)
}

func runTodayWithOptions(stdout io.Writer, opts Options) error {
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	body, err := client.Get(context.Background(), "/users/current/statusbar/today")
	if err != nil {
		return err
	}
	return writeTodayOutput(stdout, opts, body)
}

func runTodayGoal(args []string, stdout io.Writer) error {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		args = append([]string{"--today-goal", args[0]}, args[1:]...)
	}
	opts, err := parseCommon(args)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	if opts.TodayGoal == "" {
		return fmt.Errorf("goal id is required")
	}
	return runTodayGoalWithOptions(stdout, opts)
}

func runTodayGoalWithOptions(stdout io.Writer, opts Options) error {
	if !uuid4Regex.MatchString(strings.ToLower(opts.TodayGoal)) {
		return fmt.Errorf("goal id invalid")
	}
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	body, err := client.Get(context.Background(), "/users/current/goals/"+url.PathEscape(opts.TodayGoal))
	if err != nil {
		return err
	}
	return writeTodayGoalOutput(stdout, opts, body)
}

func runFileExperts(args []string, stdout io.Writer) error {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		args = append([]string{"--entity", args[0]}, args[1:]...)
	}
	opts, err := parseCommon(args)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	return postFileExperts(stdout, opts, opts.Entity, opts.Project)
}

func runStats(args []string, stdout io.Writer) error {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		args = append([]string{"--range", args[0]}, args[1:]...)
	}
	opts, err := parseCommon(args)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	path := "/users/current/stats"
	if opts.Range != "" {
		path += "/" + url.PathEscape(opts.Range)
	}
	return runSimpleGETWithOptions(stdout, opts, path)
}

func runProjects(args []string, stdout io.Writer) error {
	if len(args) >= 2 && !strings.HasPrefix(args[0], "-") && args[1] == "commits" {
		return runProjectCommits(args[0], args[2:], stdout)
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		args = append([]string{"--project", args[0]}, args[1:]...)
	}
	opts, err := parseCommon(args)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	path := "/users/current/projects"
	if opts.Project != "" {
		path += "/" + url.PathEscape(opts.Project)
		if opts.Range != "" {
			path += "?range=" + url.QueryEscape(opts.Range)
		}
	}
	return runSimpleGETWithOptions(stdout, opts, path)
}

func runProjectCommits(project string, args []string, stdout io.Writer) error {
	hash, values, commonArgs, err := splitProjectCommitsArgs(args)
	if err != nil {
		return err
	}
	opts, err := parseCommon(commonArgs)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	path := "/users/current/projects/" + url.PathEscape(strings.TrimSpace(project)) + "/commits"
	if strings.TrimSpace(hash) != "" {
		path += "/" + url.PathEscape(strings.TrimSpace(hash))
	} else if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return runSimpleGETWithOptions(stdout, opts, path)
}

func splitProjectCommitsArgs(args []string) (hash string, values url.Values, commonArgs []string, err error) {
	values = url.Values{}
	commonArgs = make([]string, 0, len(args))
	sawCommonFlag := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, ok := strings.CutPrefix(arg, "--branch="); ok {
			values.Set("branch", value)
			sawCommonFlag = true
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--page="); ok {
			values.Set("page", value)
			sawCommonFlag = true
			continue
		}
		switch arg {
		case "--branch", "--page":
			if i+1 >= len(args) {
				return "", nil, nil, fmt.Errorf("%s requires a value", arg)
			}
			i++
			values.Set(strings.TrimPrefix(arg, "--"), args[i])
			sawCommonFlag = true
		default:
			if strings.HasPrefix(arg, "-") {
				sawCommonFlag = true
			}
			if !sawCommonFlag && hash == "" {
				hash = arg
				continue
			}
			commonArgs = append(commonArgs, arg)
		}
	}
	return hash, values, commonArgs, nil
}

func runGoals(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "create":
			return runGoalCreate(args[1:], stdin, stdout)
		case "update":
			return runGoalUpdate(args[1:], stdin, stdout)
		case "delete":
			return runGoalDelete(args[1:], stdout)
		case "list":
			return runSimpleGET(args[1:], stdout, "/users/current/goals")
		}
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		args = append([]string{"--goal", args[0]}, args[1:]...)
	}
	opts, err := parseCommon(args)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	path := "/users/current/goals"
	if opts.Goal != "" {
		path += "/" + url.PathEscape(opts.Goal)
	}
	return runSimpleGETWithOptions(stdout, opts, path)
}

func runGoalCreate(args []string, stdin io.Reader, stdout io.Writer) error {
	body, commonArgs, err := readJSONCommandBody(args, stdin, "usage: stint goals create FILE|--stdin")
	if err != nil {
		return err
	}
	opts, err := parseCommon(commonArgs)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	response, err := client.PostRaw(context.Background(), "/users/current/goals", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	return writeOutput(stdout, opts.Output, response)
}

func runGoalUpdate(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("usage: stint goals update GOAL_ID FILE|--stdin")
	}
	goalID := strings.TrimSpace(args[0])
	body, commonArgs, err := readJSONCommandBody(args[1:], stdin, "usage: stint goals update GOAL_ID FILE|--stdin")
	if err != nil {
		return err
	}
	opts, err := parseCommon(commonArgs)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	response, err := client.PutRaw(context.Background(), "/users/current/goals/"+url.PathEscape(goalID), "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	return writeOutput(stdout, opts.Output, response)
}

func runGoalDelete(args []string, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("usage: stint goals delete GOAL_ID")
	}
	goalID := strings.TrimSpace(args[0])
	opts, err := parseCommon(args[1:])
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	response, err := client.Delete(context.Background(), "/users/current/goals/"+url.PathEscape(goalID))
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(response)) == 0 {
		return nil
	}
	return writeOutput(stdout, opts.Output, response)
}

func runAccount(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runSimpleGET(args, stdout, "/users/current")
	}
	switch args[0] {
	case "get":
		return runSimpleGET(args[1:], stdout, "/users/current")
	case "update":
		return runPutJSONBody(args[1:], stdin, stdout, "/users/current", "usage: stint account update FILE|--stdin")
	case "delete":
		return runAccountDelete(args[1:], stdout)
	default:
		return fmt.Errorf("unknown account command %q", args[0])
	}
}

func runAccountDelete(args []string, stdout io.Writer) error {
	commonArgs, confirmed := splitConfirmArgs(args)
	if !confirmed {
		return errors.New("usage: stint account delete --confirm")
	}
	opts, err := parseCommon(commonArgs)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	response, err := client.DeleteJSON(context.Background(), "/users/current", map[string]string{"confirmation": "DELETE"})
	if err != nil {
		return err
	}
	return writeOutput(stdout, opts.Output, response)
}

func splitConfirmArgs(args []string) ([]string, bool) {
	commonArgs := make([]string, 0, len(args))
	confirmed := false
	for _, arg := range args {
		if arg == "--confirm" {
			confirmed = true
			continue
		}
		commonArgs = append(commonArgs, arg)
	}
	return commonArgs, confirmed
}

func runLeaders(args []string, stdout io.Writer) error {
	values, commonArgs, err := splitPublicQueryArgs(args, map[string]string{
		"--country":  "country",
		"--language": "language",
	})
	if err != nil {
		return err
	}
	return runSimpleGET(commonArgs, stdout, pathWithQuery("/leaders", values))
}

func runPublicUsers(args []string, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return errors.New("usage: stint users USER [stats [RANGE|--range RANGE]|summaries [--start YYYY-MM-DD --end YYYY-MM-DD]|share TOKEN stats|summaries]")
	}
	username := strings.TrimSpace(args[0])
	if username == "" {
		return errors.New("user is required")
	}
	if len(args) == 1 || strings.HasPrefix(args[1], "-") {
		return runSimpleGET(args[1:], stdout, "/users/"+url.PathEscape(username))
	}
	switch args[1] {
	case "stats":
		if len(args) > 2 && !strings.HasPrefix(args[2], "-") {
			return runSimpleGET(args[3:], stdout, "/users/"+url.PathEscape(username)+"/stats/"+url.PathEscape(strings.TrimSpace(args[2])))
		}
		return runPublicStats(args[2:], stdout, "/users/"+url.PathEscape(username)+"/stats")
	case "summaries":
		return runPublicSummaries(args[2:], stdout, "/users/"+url.PathEscape(username)+"/summaries")
	case "share":
		if len(args) < 4 || strings.HasPrefix(args[2], "-") {
			return errors.New("usage: stint users USER share TOKEN stats|summaries")
		}
		return runUserShareRead(username, args[2:], stdout)
	default:
		return fmt.Errorf("unknown users command %q", args[1])
	}
}

func runPublicShare(args []string, stdout io.Writer) error {
	if len(args) < 2 || strings.HasPrefix(args[0], "-") {
		return errors.New("usage: stint share TOKEN stats|summaries")
	}
	token := strings.TrimSpace(args[0])
	if token == "" {
		return errors.New("share token is required")
	}
	switch args[1] {
	case "stats":
		return runPublicStats(args[2:], stdout, "/share/"+url.PathEscape(token)+"/stats")
	case "summaries":
		return runPublicSummaries(args[2:], stdout, "/share/"+url.PathEscape(token)+"/summaries")
	default:
		return fmt.Errorf("unknown share command %q", args[1])
	}
}

func runUserShareRead(username string, args []string, stdout io.Writer) error {
	token := strings.TrimSpace(args[0])
	if token == "" {
		return errors.New("share token is required")
	}
	switch args[1] {
	case "stats":
		return runPublicStats(args[2:], stdout, "/users/"+url.PathEscape(username)+"/share/"+url.PathEscape(token)+"/stats")
	case "summaries":
		return runPublicSummaries(args[2:], stdout, "/users/"+url.PathEscape(username)+"/share/"+url.PathEscape(token)+"/summaries")
	default:
		return fmt.Errorf("unknown users share command %q", args[1])
	}
}

func runPublicStats(args []string, stdout io.Writer, path string) error {
	values, commonArgs, err := splitPublicQueryArgs(args, map[string]string{"--range": "range"})
	if err != nil {
		return err
	}
	return runSimpleGET(commonArgs, stdout, pathWithQuery(path, values))
}

func runPublicSummaries(args []string, stdout io.Writer, path string) error {
	values, commonArgs, err := splitPublicQueryArgs(args, map[string]string{
		"--end":   "end",
		"--start": "start",
	})
	if err != nil {
		return err
	}
	return runSimpleGET(commonArgs, stdout, pathWithQuery(path, values))
}

func splitPublicQueryArgs(args []string, flags map[string]string) (url.Values, []string, error) {
	values := url.Values{}
	commonArgs := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if flag, value, ok := strings.Cut(arg, "="); ok {
			if key, exists := flags[flag]; exists {
				values.Set(key, strings.TrimSpace(value))
				continue
			}
		}
		if key, exists := flags[arg]; exists {
			value, next, err := nextValueArg(args, i, arg)
			if err != nil {
				return nil, nil, err
			}
			values.Set(key, value)
			i = next
			continue
		}
		commonArgs = append(commonArgs, arg)
	}
	return values, commonArgs, nil
}

func pathWithQuery(path string, values url.Values) string {
	if encoded := values.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

func runHealth(args []string, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runPublicRootGET(args, stdout, "/healthz")
	}
	switch args[0] {
	case "ingestion":
		return runPublicRootGET(args[1:], stdout, "/healthz/ingestion")
	default:
		return fmt.Errorf("unknown health command %q", args[0])
	}
}

func runDev(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: stint dev seed-key|heartbeats-purge|leaderboard-update|goals-evaluate")
	}
	subcommand := args[0]
	values, commonArgs, err := splitDevArgs(args[1:])
	if err != nil {
		return err
	}
	path := ""
	switch subcommand {
	case "seed-key":
		path = "/dev/seed-key"
	case "heartbeats-purge":
		path = "/dev/jobs/heartbeats-purge"
	case "leaderboard-update":
		path = "/dev/jobs/leaderboard-update"
	case "goals-evaluate":
		path = "/dev/jobs/goals-evaluate"
	default:
		return fmt.Errorf("unknown dev command %q", subcommand)
	}
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return runPublicPOST(commonArgs, stdout, path)
}

func runPublicRootGET(args []string, stdout io.Writer, path string) error {
	opts, err := parseCommon(args)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewPublicClient(opts)
	if err != nil {
		return err
	}
	response, err := client.GetRoot(context.Background(), path)
	if err != nil {
		return err
	}
	return writeOutput(stdout, opts.Output, response)
}

func runPublicPOST(args []string, stdout io.Writer, path string) error {
	opts, err := parseCommon(args)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewPublicClient(opts)
	if err != nil {
		return err
	}
	response, err := client.PostEmpty(context.Background(), path)
	if err != nil {
		return err
	}
	return writeOutput(stdout, opts.Output, response)
}

func splitDevArgs(args []string) (url.Values, []string, error) {
	values := url.Values{}
	commonArgs := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, ok := strings.CutPrefix(arg, "--github-id="); ok {
			values.Set("github_id", strings.TrimSpace(value))
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--username="); ok {
			values.Set("username", strings.TrimSpace(value))
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--retention-days="); ok {
			values.Set("retention_days", strings.TrimSpace(value))
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--range="); ok {
			values.Set("range", strings.TrimSpace(value))
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--now-unix="); ok {
			values.Set("now_unix", strings.TrimSpace(value))
			continue
		}
		switch arg {
		case "--github-id":
			value, next, err := nextValueArg(args, i, "--github-id")
			if err != nil {
				return nil, nil, err
			}
			values.Set("github_id", value)
			i = next
		case "--username":
			value, next, err := nextValueArg(args, i, "--username")
			if err != nil {
				return nil, nil, err
			}
			values.Set("username", value)
			i = next
		case "--retention-days":
			value, next, err := nextValueArg(args, i, "--retention-days")
			if err != nil {
				return nil, nil, err
			}
			values.Set("retention_days", value)
			i = next
		case "--range":
			value, next, err := nextValueArg(args, i, "--range")
			if err != nil {
				return nil, nil, err
			}
			values.Set("range", value)
			i = next
		case "--now-unix":
			value, next, err := nextValueArg(args, i, "--now-unix")
			if err != nil {
				return nil, nil, err
			}
			values.Set("now_unix", value)
			i = next
		default:
			commonArgs = append(commonArgs, arg)
		}
	}
	return values, commonArgs, nil
}

func nextValueArg(args []string, i int, flag string) (string, int, error) {
	if i+1 >= len(args) {
		return "", i, fmt.Errorf("%s requires a value", flag)
	}
	return strings.TrimSpace(args[i+1]), i + 1, nil
}

func runLeaderboards(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "create":
			return runPutOrPostJSONBody(http.MethodPost, args[1:], stdin, stdout, "/users/current/leaderboards", "usage: stint leaderboards create FILE|--stdin")
		case "update":
			return runLeaderboardUpdate(args[1:], stdin, stdout)
		case "delete":
			return runDeletePathArg(args[1:], stdout, "/users/current/leaderboards", "BOARD_ID", "usage: stint leaderboards delete BOARD_ID")
		case "add-member":
			return runLeaderboardAddMember(args[1:], stdout)
		case "remove-member":
			return runLeaderboardRemoveMember(args[1:], stdout)
		case "list":
			return runSimpleGET(args[1:], stdout, "/users/current/leaderboards")
		}
	}
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		args = append([]string{"--leaderboard", args[0]}, args[1:]...)
	}
	board, commonArgs, err := splitLeaderboardArgs(args)
	if err != nil {
		return err
	}
	opts, err := parseCommon(commonArgs)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	path := "/users/current/leaderboards"
	if strings.TrimSpace(board) != "" {
		path += "/" + url.PathEscape(strings.TrimSpace(board))
	}
	return runSimpleGETWithOptions(stdout, opts, path)
}

func runLeaderboardUpdate(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("usage: stint leaderboards update BOARD_ID FILE|--stdin")
	}
	boardID := strings.TrimSpace(args[0])
	return runPutOrPostJSONBody(http.MethodPut, args[1:], stdin, stdout, "/users/current/leaderboards/"+url.PathEscape(boardID), "usage: stint leaderboards update BOARD_ID FILE|--stdin")
}

func runLeaderboardAddMember(args []string, stdout io.Writer) error {
	if len(args) < 2 || strings.HasPrefix(args[0], "-") || strings.HasPrefix(args[1], "-") {
		return fmt.Errorf("usage: stint leaderboards add-member BOARD_ID USERNAME")
	}
	boardID := strings.TrimSpace(args[0])
	username := strings.TrimSpace(args[1])
	opts, err := parseCommon(args[2:])
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	response, err := client.PostJSON(context.Background(), "/users/current/leaderboards/"+url.PathEscape(boardID)+"/members", map[string]string{"username": username})
	if err != nil {
		return err
	}
	return writeOutput(stdout, opts.Output, response)
}

func runLeaderboardRemoveMember(args []string, stdout io.Writer) error {
	if len(args) < 2 || strings.HasPrefix(args[0], "-") || strings.HasPrefix(args[1], "-") {
		return fmt.Errorf("usage: stint leaderboards remove-member BOARD_ID USER_ID")
	}
	boardID := strings.TrimSpace(args[0])
	userID := strings.TrimSpace(args[1])
	opts, err := parseCommon(args[2:])
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	response, err := client.Delete(context.Background(), "/users/current/leaderboards/"+url.PathEscape(boardID)+"/members/"+url.PathEscape(userID))
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(response)) == 0 {
		return nil
	}
	return writeOutput(stdout, opts.Output, response)
}

func splitLeaderboardArgs(args []string) (board string, commonArgs []string, err error) {
	commonArgs = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, ok := strings.CutPrefix(arg, "--leaderboard="); ok {
			board = value
			continue
		}
		if arg == "--leaderboard" {
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("--leaderboard requires a value")
			}
			i++
			board = args[i]
			continue
		}
		commonArgs = append(commonArgs, arg)
	}
	return board, commonArgs, nil
}

func runInsights(args []string, stdout io.Writer) error {
	insightType, rangeName, commonArgs, err := splitInsightsArgs(args)
	if err != nil {
		return err
	}
	if strings.TrimSpace(insightType) == "" || strings.TrimSpace(rangeName) == "" {
		return fmt.Errorf("usage: stint insights TYPE RANGE")
	}
	opts, err := parseCommon(commonArgs)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	path := "/users/current/insights/" + url.PathEscape(strings.TrimSpace(insightType)) + "/" + url.PathEscape(strings.TrimSpace(rangeName))
	return runSimpleGETWithOptions(stdout, opts, path)
}

func splitInsightsArgs(args []string) (insightType, rangeName string, commonArgs []string, err error) {
	commonArgs = make([]string, 0, len(args))
	positionals := make([]string, 0, 2)
	sawCommonFlag := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, ok := strings.CutPrefix(arg, "--type="); ok {
			insightType = value
			sawCommonFlag = true
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--insight-type="); ok {
			insightType = value
			sawCommonFlag = true
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--range="); ok {
			rangeName = value
			sawCommonFlag = true
			continue
		}
		switch arg {
		case "--type", "--insight-type":
			if i+1 >= len(args) {
				return "", "", nil, fmt.Errorf("%s requires a value", arg)
			}
			i++
			insightType = args[i]
			sawCommonFlag = true
		case "--range":
			if i+1 >= len(args) {
				return "", "", nil, fmt.Errorf("--range requires a value")
			}
			i++
			rangeName = args[i]
			sawCommonFlag = true
		default:
			if strings.HasPrefix(arg, "-") {
				sawCommonFlag = true
			}
			if !sawCommonFlag && len(positionals) < 2 {
				positionals = append(positionals, arg)
				continue
			}
			commonArgs = append(commonArgs, arg)
		}
	}
	if insightType == "" && len(positionals) > 0 {
		insightType = positionals[0]
	}
	if rangeName == "" && len(positionals) > 1 {
		rangeName = positionals[1]
	}
	return insightType, rangeName, commonArgs, nil
}

func runUsageEvents(args []string, stdout io.Writer) error {
	subcommand, values, commonArgs, err := splitUsageEventsArgs(args)
	if err != nil {
		return err
	}
	opts, err := parseCommon(commonArgs)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	path := "/users/current/usage_events"
	switch subcommand {
	case "", "list":
	case "summary":
		path += "/summary"
	case "blocks":
		path += "/blocks"
	default:
		return fmt.Errorf("unknown usage-events command %q", subcommand)
	}
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return runSimpleGETWithOptions(stdout, opts, path)
}

func runDataDumps(args []string, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runSimpleGET(args, stdout, "/users/current/data_dumps")
	}
	switch args[0] {
	case "list":
		return runSimpleGET(args[1:], stdout, "/users/current/data_dumps")
	case "download":
		if len(args) < 2 || strings.HasPrefix(args[1], "-") {
			return fmt.Errorf("usage: stint data-dumps download DUMP_ID")
		}
		path := "/users/current/data_dumps/" + url.PathEscape(strings.TrimSpace(args[1])) + "/download"
		return runSimpleGET(args[2:], stdout, path)
	case "create":
		dumpType, commonArgs, err := splitDataDumpCreateArgs(args[1:])
		if err != nil {
			return err
		}
		dumpType = strings.TrimSpace(dumpType)
		if !validDataDumpType(dumpType) {
			return fmt.Errorf("usage: stint data-dumps create heartbeats|daily")
		}
		opts, err := parseCommon(commonArgs)
		if err != nil {
			return err
		}
		opts.LogWriter = stdout
		client, err := NewClient(opts)
		if err != nil {
			return err
		}
		body, err := client.PostJSON(context.Background(), "/users/current/data_dumps", map[string]string{"type": dumpType})
		if err != nil {
			return err
		}
		return writeOutput(stdout, opts.Output, body)
	default:
		return fmt.Errorf("unknown data-dumps command %q", args[0])
	}
}

func validDataDumpType(value string) bool {
	switch strings.TrimSpace(value) {
	case "heartbeats", "daily":
		return true
	default:
		return false
	}
}

func splitDataDumpCreateArgs(args []string) (dumpType string, commonArgs []string, err error) {
	commonArgs = make([]string, 0, len(args))
	sawCommonFlag := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, ok := strings.CutPrefix(arg, "--type="); ok {
			dumpType = value
			sawCommonFlag = true
			continue
		}
		if arg == "--type" {
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("--type requires a value")
			}
			i++
			dumpType = args[i]
			sawCommonFlag = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			sawCommonFlag = true
		}
		if !sawCommonFlag && dumpType == "" {
			dumpType = arg
			continue
		}
		commonArgs = append(commonArgs, arg)
	}
	return dumpType, commonArgs, nil
}

func runExternalDurations(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runSimpleGET(args, stdout, "/users/current/external_durations")
	}
	switch args[0] {
	case "list":
		return runSimpleGET(args[1:], stdout, "/users/current/external_durations")
	case "create":
		return runExternalDurationJSON(args[1:], stdin, stdout, "/users/current/external_durations")
	case "bulk":
		return runExternalDurationJSON(args[1:], stdin, stdout, "/users/current/external_durations.bulk")
	case "delete":
		return runExternalDurationsDelete(args[1:], stdout)
	default:
		return fmt.Errorf("unknown external-durations command %q", args[0])
	}
}

func runExternalDurationJSON(args []string, stdin io.Reader, stdout io.Writer, path string) error {
	body, commonArgs, err := readJSONCommandBody(args, stdin, "usage: stint external-durations create|bulk FILE|--stdin")
	if err != nil {
		return err
	}
	opts, err := parseCommon(commonArgs)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	response, err := client.PostRaw(context.Background(), path, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	return writeOutput(stdout, opts.Output, response)
}

func runExternalDurationsDelete(args []string, stdout io.Writer) error {
	ids, commonArgs, err := splitIDArgs(args)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return fmt.Errorf("usage: stint external-durations delete ID [ID...]|--ids id1,id2")
	}
	opts, err := parseCommon(commonArgs)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	response, err := client.DeleteJSON(context.Background(), "/users/current/external_durations.bulk", map[string][]string{"ids": ids})
	if err != nil {
		return err
	}
	return writeOutput(stdout, opts.Output, response)
}

func readJSONCommandBody(args []string, stdin io.Reader, usage string) ([]byte, []string, error) {
	bodyPath, useStdin, commonArgs, err := splitBodyArgs(args)
	if err != nil {
		return nil, nil, err
	}
	if !useStdin && strings.TrimSpace(bodyPath) == "" {
		return nil, nil, errors.New(usage)
	}
	if useStdin {
		body, err := io.ReadAll(stdin)
		return body, commonArgs, err
	}
	body, err := os.ReadFile(expandHome(bodyPath))
	return body, commonArgs, err
}

func splitBodyArgs(args []string) (bodyPath string, useStdin bool, commonArgs []string, err error) {
	commonArgs = make([]string, 0, len(args))
	sawCommonFlag := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--stdin" {
			useStdin = true
			sawCommonFlag = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			sawCommonFlag = true
		}
		if !sawCommonFlag && bodyPath == "" {
			bodyPath = arg
			continue
		}
		commonArgs = append(commonArgs, arg)
	}
	return bodyPath, useStdin, commonArgs, nil
}

func splitIDArgs(args []string) (ids []string, commonArgs []string, err error) {
	commonArgs = make([]string, 0, len(args))
	sawCommonFlag := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, ok := strings.CutPrefix(arg, "--ids="); ok {
			ids = appendCommaSeparated(ids, value)
			sawCommonFlag = true
			continue
		}
		if arg == "--ids" {
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--ids requires a value")
			}
			i++
			ids = appendCommaSeparated(ids, args[i])
			sawCommonFlag = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			sawCommonFlag = true
		}
		if !sawCommonFlag {
			ids = append(ids, strings.TrimSpace(arg))
			continue
		}
		commonArgs = append(commonArgs, arg)
	}
	return compactNonEmpty(ids), commonArgs, nil
}

func appendCommaSeparated(values []string, raw string) []string {
	for _, value := range strings.Split(raw, ",") {
		values = append(values, strings.TrimSpace(value))
	}
	return values
}

func compactNonEmpty(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, strings.TrimSpace(value))
		}
	}
	return out
}

func runCustomPricing(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runSimpleGET(args, stdout, "/users/current/custom_pricing")
	}
	switch args[0] {
	case "list":
		return runSimpleGET(args[1:], stdout, "/users/current/custom_pricing")
	case "upsert":
		return runPutJSONBody(args[1:], stdin, stdout, "/users/current/custom_pricing", "usage: stint custom-pricing upsert FILE|--stdin")
	case "delete":
		return runDeletePathArg(args[1:], stdout, "/users/current/custom_pricing", "MODEL", "usage: stint custom-pricing delete MODEL")
	default:
		return fmt.Errorf("unknown custom-pricing command %q", args[0])
	}
}

func runBillingPrefs(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runSimpleGET(args, stdout, "/users/current/billing_prefs")
	}
	switch args[0] {
	case "list":
		return runSimpleGET(args[1:], stdout, "/users/current/billing_prefs")
	case "upsert":
		return runPutJSONBody(args[1:], stdin, stdout, "/users/current/billing_prefs", "usage: stint billing-prefs upsert FILE|--stdin")
	case "delete":
		return runDeletePathArg(args[1:], stdout, "/users/current/billing_prefs", "AGENT", "usage: stint billing-prefs delete AGENT")
	default:
		return fmt.Errorf("unknown billing-prefs command %q", args[0])
	}
}

func runAICosts(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runSimpleGET(args, stdout, "/users/current/ai_costs")
	}
	switch args[0] {
	case "list":
		return runSimpleGET(args[1:], stdout, "/users/current/ai_costs")
	case "replace":
		return runPutJSONBody(args[1:], stdin, stdout, "/users/current/ai_costs", "usage: stint ai-costs replace FILE|--stdin")
	default:
		return fmt.Errorf("unknown ai-costs command %q", args[0])
	}
}

func runPutJSONBody(args []string, stdin io.Reader, stdout io.Writer, path, usage string) error {
	return runPutOrPostJSONBody(http.MethodPut, args, stdin, stdout, path, usage)
}

func runPutOrPostJSONBody(method string, args []string, stdin io.Reader, stdout io.Writer, path, usage string) error {
	body, commonArgs, err := readJSONCommandBody(args, stdin, usage)
	if err != nil {
		return err
	}
	opts, err := parseCommon(commonArgs)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	var response []byte
	switch method {
	case http.MethodPost:
		response, err = client.PostRaw(context.Background(), path, "application/json", bytes.NewReader(body))
	case http.MethodPut:
		response, err = client.PutRaw(context.Background(), path, "application/json", bytes.NewReader(body))
	default:
		err = fmt.Errorf("unsupported JSON body method %s", method)
	}
	if err != nil {
		return err
	}
	return writeOutput(stdout, opts.Output, response)
}

func runDeletePathArg(args []string, stdout io.Writer, basePath, argName, usage string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return errors.New(usage)
	}
	value := strings.TrimSpace(args[0])
	if value == "" {
		return fmt.Errorf("%s is required", strings.ToLower(argName))
	}
	opts, err := parseCommon(args[1:])
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	response, err := client.Delete(context.Background(), basePath+"/"+url.PathEscape(value))
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(response)) == 0 {
		return nil
	}
	return writeOutput(stdout, opts.Output, response)
}

func runCustomRules(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runSimpleGET(args, stdout, "/users/current/custom_rules")
	}
	switch args[0] {
	case "list":
		return runSimpleGET(args[1:], stdout, "/users/current/custom_rules")
	case "progress":
		return runSimpleGET(args[1:], stdout, "/users/current/custom_rules_progress")
	case "replace":
		return runPutJSONBody(args[1:], stdin, stdout, "/users/current/custom_rules", "usage: stint custom-rules replace FILE|--stdin")
	case "delete":
		return runDeletePathArg(args[1:], stdout, "/users/current/custom_rules", "RULE_ID", "usage: stint custom-rules delete RULE_ID")
	case "abort":
		return runDeleteNoBody(args[1:], stdout, "/users/current/custom_rules_progress")
	default:
		return fmt.Errorf("unknown custom-rules command %q", args[0])
	}
}

func runDeleteNoBody(args []string, stdout io.Writer, path string) error {
	opts, err := parseCommon(args)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	response, err := client.Delete(context.Background(), path)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(response)) == 0 {
		return nil
	}
	return writeOutput(stdout, opts.Output, response)
}

func runShareTokens(args []string, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runSimpleGET(args, stdout, "/users/current/share_tokens")
	}
	switch args[0] {
	case "list":
		return runSimpleGET(args[1:], stdout, "/users/current/share_tokens")
	case "create":
		return runShareTokenCreate(args[1:], stdout)
	case "delete":
		return runDeletePathArg(args[1:], stdout, "/users/current/share_tokens", "ID", "usage: stint share-tokens delete ID")
	default:
		return fmt.Errorf("unknown share-tokens command %q", args[0])
	}
}

func runShareTokenCreate(args []string, stdout io.Writer) error {
	name, commonArgs, err := splitSingleValueArgs(args, "--name")
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("usage: stint share-tokens create NAME")
	}
	opts, err := parseCommon(commonArgs)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	response, err := client.PostJSON(context.Background(), "/users/current/share_tokens", map[string]string{"name": strings.TrimSpace(name)})
	if err != nil {
		return err
	}
	return writeOutput(stdout, opts.Output, response)
}

func splitSingleValueArgs(args []string, flag string) (value string, commonArgs []string, err error) {
	commonArgs = make([]string, 0, len(args))
	sawCommonFlag := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, flag+"=") {
			value = strings.TrimPrefix(arg, flag+"=")
			sawCommonFlag = true
			continue
		}
		if arg == flag {
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("%s requires a value", flag)
			}
			i++
			value = args[i]
			sawCommonFlag = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			sawCommonFlag = true
		}
		if !sawCommonFlag && value == "" {
			value = arg
			continue
		}
		commonArgs = append(commonArgs, arg)
	}
	return value, commonArgs, nil
}

func runAPIKeys(args []string, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runSimpleGET(args, stdout, "/api_keys")
	}
	switch args[0] {
	case "list":
		return runSimpleGET(args[1:], stdout, "/api_keys")
	case "create":
		return runAPIKeyCreate(args[1:], stdout)
	case "delete", "revoke":
		return runDeletePathArg(args[1:], stdout, "/api_keys", "ID", "usage: stint api-keys delete ID")
	default:
		return fmt.Errorf("unknown api-keys command %q", args[0])
	}
}

func runAPIKeyCreate(args []string, stdout io.Writer) error {
	name, scopes, commonArgs, err := splitAPIKeyCreateArgs(args)
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("usage: stint api-keys create NAME [--scope SCOPE]")
	}
	opts, err := parseCommon(commonArgs)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	response, err := client.PostJSON(context.Background(), "/api_keys", map[string]any{"name": strings.TrimSpace(name), "scopes": scopes})
	if err != nil {
		return err
	}
	return writeOutput(stdout, opts.Output, response)
}

func splitAPIKeyCreateArgs(args []string) (name string, scopes []string, commonArgs []string, err error) {
	commonArgs = make([]string, 0, len(args))
	sawCommonFlag := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, ok := strings.CutPrefix(arg, "--scope="); ok {
			scopes = append(scopes, strings.TrimSpace(value))
			sawCommonFlag = true
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--scopes="); ok {
			scopes = appendCommaSeparated(scopes, value)
			sawCommonFlag = true
			continue
		}
		switch arg {
		case "--scope":
			if i+1 >= len(args) {
				return "", nil, nil, fmt.Errorf("--scope requires a value")
			}
			i++
			scopes = append(scopes, strings.TrimSpace(args[i]))
			sawCommonFlag = true
		case "--scopes":
			if i+1 >= len(args) {
				return "", nil, nil, fmt.Errorf("--scopes requires a value")
			}
			i++
			scopes = appendCommaSeparated(scopes, args[i])
			sawCommonFlag = true
		default:
			if strings.HasPrefix(arg, "-") {
				sawCommonFlag = true
			}
			if !sawCommonFlag && name == "" {
				name = arg
				continue
			}
			commonArgs = append(commonArgs, arg)
		}
	}
	return strings.TrimSpace(name), compactNonEmpty(scopes), commonArgs, nil
}

func runOAuthApps(args []string, stdout io.Writer) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runSimpleGET(args, stdout, "/oauth/apps")
	}
	switch args[0] {
	case "list":
		return runSimpleGET(args[1:], stdout, "/oauth/apps")
	case "create":
		return runOAuthAppCreate(args[1:], stdout)
	case "delete":
		return runDeletePathArg(args[1:], stdout, "/oauth/apps", "ID", "usage: stint oauth-apps delete ID")
	default:
		return fmt.Errorf("unknown oauth-apps command %q", args[0])
	}
}

func runOAuth(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: stint oauth apps|token|revoke")
	}
	switch args[0] {
	case "apps":
		return runOAuthApps(args[1:], stdout)
	case "token":
		return runOAuthToken(args[1:], stdout)
	case "revoke":
		return runOAuthRevoke(args[1:], stdout)
	default:
		return fmt.Errorf("unknown oauth command %q", args[0])
	}
}

func runOAuthToken(args []string, stdout io.Writer) error {
	clientID, clientSecret, code, redirectURI, refreshToken, commonArgs, err := splitOAuthTokenArgs(args)
	if err != nil {
		return err
	}
	if clientID == "" || clientSecret == "" {
		return fmt.Errorf("usage: stint oauth token --client-id ID --client-secret SECRET (--code CODE --redirect-uri URI|--refresh-token TOKEN)")
	}
	form := url.Values{}
	if refreshToken != "" {
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", refreshToken)
	} else {
		if code == "" || redirectURI == "" {
			return fmt.Errorf("usage: stint oauth token --client-id ID --client-secret SECRET (--code CODE --redirect-uri URI|--refresh-token TOKEN)")
		}
		form.Set("grant_type", "authorization_code")
		form.Set("code", code)
		form.Set("redirect_uri", redirectURI)
	}
	return postOAuthForm(commonArgs, stdout, "/oauth/token", form, clientID, clientSecret)
}

func runOAuthRevoke(args []string, stdout io.Writer) error {
	token, clientID, clientSecret, commonArgs, err := splitOAuthRevokeArgs(args)
	if err != nil {
		return err
	}
	if token == "" || clientID == "" || clientSecret == "" {
		return fmt.Errorf("usage: stint oauth revoke TOKEN --client-id ID --client-secret SECRET")
	}
	form := url.Values{"token": []string{token}}
	return postOAuthForm(commonArgs, stdout, "/oauth/revoke", form, clientID, clientSecret)
}

func postOAuthForm(commonArgs []string, stdout io.Writer, path string, form url.Values, clientID, clientSecret string) error {
	opts, err := parseCommon(commonArgs)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	response, err := client.PostOAuthForm(context.Background(), path, form, clientID, clientSecret)
	if err != nil {
		return err
	}
	return writeOutput(stdout, opts.Output, response)
}

func runOAuthAppCreate(args []string, stdout io.Writer) error {
	name, redirects, scopes, commonArgs, err := splitOAuthAppCreateArgs(args)
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("usage: stint oauth-apps create NAME --redirect-uri URI [--scope SCOPE]")
	}
	if len(redirects) == 0 {
		return fmt.Errorf("--redirect-uri is required")
	}
	opts, err := parseCommon(commonArgs)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	response, err := client.PostJSON(context.Background(), "/oauth/apps", map[string]any{
		"name":          strings.TrimSpace(name),
		"redirect_uris": redirects,
		"scopes":        scopes,
	})
	if err != nil {
		return err
	}
	return writeOutput(stdout, opts.Output, response)
}

func splitOAuthTokenArgs(args []string) (clientID, clientSecret, code, redirectURI, refreshToken string, commonArgs []string, err error) {
	commonArgs = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, ok := strings.CutPrefix(arg, "--client-id="); ok {
			clientID = strings.TrimSpace(value)
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--client-secret="); ok {
			clientSecret = strings.TrimSpace(value)
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--code="); ok {
			code = strings.TrimSpace(value)
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--redirect-uri="); ok {
			redirectURI = strings.TrimSpace(value)
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--refresh-token="); ok {
			refreshToken = strings.TrimSpace(value)
			continue
		}
		switch arg {
		case "--client-id":
			clientID, i, err = nextOAuthArg(args, i, "--client-id")
		case "--client-secret":
			clientSecret, i, err = nextOAuthArg(args, i, "--client-secret")
		case "--code":
			code, i, err = nextOAuthArg(args, i, "--code")
		case "--redirect-uri":
			redirectURI, i, err = nextOAuthArg(args, i, "--redirect-uri")
		case "--refresh-token":
			refreshToken, i, err = nextOAuthArg(args, i, "--refresh-token")
		default:
			commonArgs = append(commonArgs, arg)
		}
		if err != nil {
			return "", "", "", "", "", nil, err
		}
	}
	return clientID, clientSecret, code, redirectURI, refreshToken, commonArgs, nil
}

func splitOAuthRevokeArgs(args []string) (token, clientID, clientSecret string, commonArgs []string, err error) {
	commonArgs = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, ok := strings.CutPrefix(arg, "--client-id="); ok {
			clientID = strings.TrimSpace(value)
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--client-secret="); ok {
			clientSecret = strings.TrimSpace(value)
			continue
		}
		switch arg {
		case "--client-id":
			clientID, i, err = nextOAuthArg(args, i, "--client-id")
		case "--client-secret":
			clientSecret, i, err = nextOAuthArg(args, i, "--client-secret")
		default:
			if token == "" && !strings.HasPrefix(arg, "-") {
				token = strings.TrimSpace(arg)
				continue
			}
			commonArgs = append(commonArgs, arg)
		}
		if err != nil {
			return "", "", "", nil, err
		}
	}
	return token, clientID, clientSecret, commonArgs, nil
}

func nextOAuthArg(args []string, i int, flag string) (string, int, error) {
	if i+1 >= len(args) {
		return "", i, fmt.Errorf("%s requires a value", flag)
	}
	return strings.TrimSpace(args[i+1]), i + 1, nil
}

func splitOAuthAppCreateArgs(args []string) (name string, redirects, scopes, commonArgs []string, err error) {
	commonArgs = make([]string, 0, len(args))
	sawCommonFlag := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, ok := strings.CutPrefix(arg, "--redirect-uri="); ok {
			redirects = append(redirects, strings.TrimSpace(value))
			sawCommonFlag = true
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--redirect-uris="); ok {
			redirects = appendCommaSeparated(redirects, value)
			sawCommonFlag = true
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--scope="); ok {
			scopes = append(scopes, strings.TrimSpace(value))
			sawCommonFlag = true
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--scopes="); ok {
			scopes = appendCommaSeparated(scopes, value)
			sawCommonFlag = true
			continue
		}
		switch arg {
		case "--redirect-uri":
			if i+1 >= len(args) {
				return "", nil, nil, nil, fmt.Errorf("--redirect-uri requires a value")
			}
			i++
			redirects = append(redirects, strings.TrimSpace(args[i]))
			sawCommonFlag = true
		case "--redirect-uris":
			if i+1 >= len(args) {
				return "", nil, nil, nil, fmt.Errorf("--redirect-uris requires a value")
			}
			i++
			redirects = appendCommaSeparated(redirects, args[i])
			sawCommonFlag = true
		case "--scope":
			if i+1 >= len(args) {
				return "", nil, nil, nil, fmt.Errorf("--scope requires a value")
			}
			i++
			scopes = append(scopes, strings.TrimSpace(args[i]))
			sawCommonFlag = true
		case "--scopes":
			if i+1 >= len(args) {
				return "", nil, nil, nil, fmt.Errorf("--scopes requires a value")
			}
			i++
			scopes = appendCommaSeparated(scopes, args[i])
			sawCommonFlag = true
		default:
			if strings.HasPrefix(arg, "-") {
				sawCommonFlag = true
			}
			if !sawCommonFlag && name == "" {
				name = arg
				continue
			}
			commonArgs = append(commonArgs, arg)
		}
	}
	return strings.TrimSpace(name), compactNonEmpty(redirects), compactNonEmpty(scopes), commonArgs, nil
}

func runImport(args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: stint import wakatime FILE|--stdin")
	}
	switch args[0] {
	case "wakatime":
		return runImportWakaTime(args[1:], stdin, stdout)
	default:
		return fmt.Errorf("unknown import command %q", args[0])
	}
}

func runImportWakaTime(args []string, stdin io.Reader, stdout io.Writer) error {
	importPath, useStdin, commonArgs, err := splitImportWakaTimeArgs(args)
	if err != nil {
		return err
	}
	if !useStdin && strings.TrimSpace(importPath) == "" {
		return fmt.Errorf("usage: stint import wakatime FILE|--stdin")
	}
	opts, err := parseCommon(commonArgs)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	var body []byte
	contentType := "application/json"
	if useStdin {
		body, err = io.ReadAll(stdin)
		if err != nil {
			return err
		}
	} else {
		body, contentType, err = multipartImportFile(importPath)
		if err != nil {
			return err
		}
	}
	response, err := client.PostRaw(context.Background(), "/users/current/imports/wakatime", contentType, bytes.NewReader(body))
	if err != nil {
		return err
	}
	return writeOutput(stdout, opts.Output, response)
}

func splitImportWakaTimeArgs(args []string) (importPath string, useStdin bool, commonArgs []string, err error) {
	commonArgs = make([]string, 0, len(args))
	sawCommonFlag := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--stdin" {
			useStdin = true
			sawCommonFlag = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			sawCommonFlag = true
		}
		if !sawCommonFlag && importPath == "" {
			importPath = arg
			continue
		}
		commonArgs = append(commonArgs, arg)
	}
	return importPath, useStdin, commonArgs, nil
}

func multipartImportFile(importPath string) ([]byte, string, error) {
	data, err := os.ReadFile(expandHome(importPath))
	if err != nil {
		return nil, "", err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(importPath))
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(data); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}

func splitUsageEventsArgs(args []string) (subcommand string, values url.Values, commonArgs []string, err error) {
	values = url.Values{}
	commonArgs = make([]string, 0, len(args))
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "list", "summary", "blocks":
			subcommand = args[0]
			args = args[1:]
		}
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if key, value, ok := usageEventsInlineFlag(arg); ok {
			values.Set(key, value)
			continue
		}
		if key, ok := usageEventsFlagKey(arg); ok {
			if i+1 >= len(args) {
				return "", nil, nil, fmt.Errorf("%s requires a value", arg)
			}
			i++
			values.Set(key, args[i])
			continue
		}
		commonArgs = append(commonArgs, arg)
	}
	return subcommand, values, commonArgs, nil
}

func usageEventsInlineFlag(arg string) (key, value string, ok bool) {
	for _, name := range []string{"start", "end", "range", "cost-mode", "cost_mode", "agent"} {
		prefix := "--" + name + "="
		if strings.HasPrefix(arg, prefix) {
			key, ok := usageEventsFlagKey("--" + name)
			return key, strings.TrimPrefix(arg, prefix), ok
		}
	}
	return "", "", false
}

func usageEventsFlagKey(arg string) (string, bool) {
	switch arg {
	case "--start":
		return "start", true
	case "--end":
		return "end", true
	case "--range":
		return "range", true
	case "--cost-mode", "--cost_mode":
		return "cost_mode", true
	case "--agent":
		return "agent", true
	default:
		return "", false
	}
}

func runDurations(args []string, stdout io.Writer) error {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		args = append([]string{"--date", args[0]}, args[1:]...)
	}
	date, sliceBy, commonArgs, err := splitDurationsArgs(args)
	if err != nil {
		return err
	}
	opts, err := parseCommon(commonArgs)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	values := url.Values{}
	if strings.TrimSpace(date) != "" {
		values.Set("date", strings.TrimSpace(date))
	}
	if strings.TrimSpace(sliceBy) != "" {
		values.Set("slice_by", strings.TrimSpace(sliceBy))
	}
	path := "/users/current/durations"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return runSimpleGETWithOptions(stdout, opts, path)
}

func splitDurationsArgs(args []string) (date, sliceBy string, commonArgs []string, err error) {
	commonArgs = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, ok := strings.CutPrefix(arg, "--date="); ok {
			date = value
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--slice-by="); ok {
			sliceBy = value
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--slice_by="); ok {
			sliceBy = value
			continue
		}
		switch arg {
		case "--date":
			if i+1 >= len(args) {
				return "", "", nil, fmt.Errorf("--date requires a value")
			}
			i++
			date = args[i]
		case "--slice-by", "--slice_by":
			if i+1 >= len(args) {
				return "", "", nil, fmt.Errorf("%s requires a value", arg)
			}
			i++
			sliceBy = args[i]
		default:
			commonArgs = append(commonArgs, arg)
		}
	}
	return date, sliceBy, commonArgs, nil
}

func runSummaries(args []string, stdout io.Writer) error {
	start, end, commonArgs, err := splitSummariesArgs(args)
	if err != nil {
		return err
	}
	opts, err := parseCommon(commonArgs)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	values := url.Values{}
	if strings.TrimSpace(start) != "" {
		values.Set("start", strings.TrimSpace(start))
	}
	if strings.TrimSpace(end) != "" {
		values.Set("end", strings.TrimSpace(end))
	}
	path := "/users/current/summaries"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return runSimpleGETWithOptions(stdout, opts, path)
}

func splitSummariesArgs(args []string) (start, end string, commonArgs []string, err error) {
	commonArgs = make([]string, 0, len(args))
	positionals := make([]string, 0, 2)
	sawCommonFlag := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if value, ok := strings.CutPrefix(arg, "--start="); ok {
			start = value
			sawCommonFlag = true
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--end="); ok {
			end = value
			sawCommonFlag = true
			continue
		}
		switch arg {
		case "--start":
			if i+1 >= len(args) {
				return "", "", nil, fmt.Errorf("--start requires a value")
			}
			i++
			start = args[i]
			sawCommonFlag = true
		case "--end":
			if i+1 >= len(args) {
				return "", "", nil, fmt.Errorf("--end requires a value")
			}
			i++
			end = args[i]
			sawCommonFlag = true
		default:
			if strings.HasPrefix(arg, "-") {
				sawCommonFlag = true
			}
			if !sawCommonFlag && len(positionals) < 2 {
				positionals = append(positionals, arg)
				continue
			}
			commonArgs = append(commonArgs, arg)
		}
	}
	if start == "" && len(positionals) > 0 {
		start = positionals[0]
	}
	if end == "" && len(positionals) > 1 {
		end = positionals[1]
	}
	return start, end, commonArgs, nil
}

func runOffline(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: stint offline count|print|sync")
	}
	opts, err := parseCommon(args[1:])
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	switch args[0] {
	case "count":
		count, err := CountQueue(opts.QueuePath)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, count)
		return nil
	case "print":
		return printQueue(stdout, opts.QueuePath, opts.PrintOffline)
	case "sync":
		limit := opts.SyncOffline
		return syncOffline(stdout, opts, limit)
	default:
		return fmt.Errorf("unknown offline command %q", args[0])
	}
}

func runDoctor(args []string, stdout io.Writer) error {
	opts, err := parseCommon(args)
	if err != nil {
		return err
	}
	opts.LogWriter = stdout
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	body, err := client.Get(context.Background(), "/meta")
	if err != nil {
		return err
	}
	count, err := CountQueue(opts.QueuePath)
	return writeDoctorOutput(stdout, opts.Output, body, count, err)
}

func writeDoctorOutput(stdout io.Writer, format string, body []byte, offlineCount int, countErr error) error {
	format = strings.TrimSpace(format)
	if format == "json" || format == "raw-json" {
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			return err
		}
		if countErr == nil {
			payload["offline_queue_count"] = offlineCount
		}
		encoded, err := marshalJSONNoHTMLEscape(payload)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, string(encoded))
		return err
	}
	if err := writeOutput(stdout, format, body); err != nil {
		return err
	}
	if countErr == nil {
		fmt.Fprintf(stdout, "\noffline_queue_count=%d\n", offlineCount)
	}
	return nil
}

func runCollect(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if path, err := collectHelperPath(); err == nil {
		cmd := exec.Command(path, args...)
		cmd.Stdin = stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		return cmd.Run()
	}
	if fileExists("go.mod") && fileExists(filepath.Join("cmd", "collect", "main.go")) {
		goArgs := append([]string{"run", "./cmd/collect"}, args...)
		cmd := exec.Command("go", goArgs...)
		cmd.Stdin = stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		return cmd.Run()
	}
	return fmt.Errorf("stint-collect is not installed; run `make collect-install` or use `go run ./cmd/collect ...` from the repository")
}

func collectHelperPath() (string, error) {
	if path, err := exec.LookPath("stint-collect"); err == nil {
		return path, nil
	}
	exe, err := executablePath()
	if err != nil || exe == "" {
		return "", fmt.Errorf("stint-collect not found")
	}
	candidate := filepath.Join(filepath.Dir(exe), "stint-collect")
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
		return candidate, nil
	}
	return "", fmt.Errorf("stint-collect not found")
}

func postFileExperts(stdout io.Writer, opts Options, entity, project string) error {
	entity = strings.TrimSpace(entity)
	if entity == "" {
		return fmt.Errorf("--entity is required")
	}
	opts.Entity = entity
	opts.Project = first(project, opts.Project)
	hb, err := BuildHeartbeat(opts)
	if err != nil {
		if errors.Is(err, errHeartbeatFiltered) {
			return nil
		}
		return err
	}
	if !validFileExpertsHeartbeat(hb) {
		return nil
	}
	payload := map[string]any{"entity": hb.Entity}
	if strings.TrimSpace(hb.Project) != "" {
		payload["project"] = hb.Project
	}
	if hb.ProjectRootCount != nil {
		payload["project_root_count"] = hb.ProjectRootCount
	}
	client, err := NewClient(opts)
	if err != nil {
		return err
	}
	body, err := client.PostJSON(context.Background(), "/users/current/file_experts", payload)
	if err != nil {
		return err
	}
	return writeFileExpertsOutput(stdout, opts.Output, body)
}

func validFileExpertsHeartbeat(hb Heartbeat) bool {
	return strings.TrimSpace(hb.Entity) != "" &&
		strings.TrimSpace(hb.Project) != "" &&
		hb.ProjectRootCount != nil &&
		*hb.ProjectRootCount > 0
}

func printQueue(stdout io.Writer, path string, limit int) error {
	if limit <= 0 {
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode([]Heartbeat{})
	}
	heartbeats, err := ReadQueue(path, limit)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(heartbeats)
}

func syncOffline(stdout io.Writer, opts Options, limit int) error {
	if limit < 0 {
		limit = 0
	}
	totalSynced := 0
	if opts.LegacyQueuePath != "" && opts.LegacyQueuePath != opts.QueuePath && fileExists(opts.LegacyQueuePath) {
		synced, err := syncOfflineQueue(opts, opts.LegacyQueuePath, limit)
		if err != nil {
			return err
		}
		totalSynced += synced
		if err := os.Remove(opts.LegacyQueuePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	synced, err := syncOfflineQueue(opts, opts.QueuePath, limit)
	if err != nil {
		return err
	}
	totalSynced += synced
	if totalSynced > 0 {
		fmt.Fprintf(stdout, "synced=%d\n", totalSynced)
	}
	return nil
}

func syncOfflineQueue(opts Options, queuePath string, limit int) (int, error) {
	if _, err := DeleteQueueDuplicates(queuePath); err != nil {
		return 0, err
	}
	totalSynced := 0
	remaining := limit
	for {
		batchLimit := defaultHeartbeatLimit
		if remaining > 0 && remaining < batchLimit {
			batchLimit = remaining
		}
		heartbeats, err := ReadQueue(queuePath, batchLimit)
		if err != nil {
			return totalSynced, err
		}
		if len(heartbeats) == 0 {
			break
		}
		if err := RemoveQueuePrefix(queuePath, len(heartbeats)); err != nil {
			return totalSynced, err
		}
		requeue, err := postOfflineHeartbeats(opts, heartbeats)
		if err != nil {
			if qerr := AppendQueue(queuePath, heartbeats); qerr != nil {
				return totalSynced, errors.Join(err, qerr)
			}
			return totalSynced, err
		}
		if len(requeue) > 0 {
			if err := AppendQueue(queuePath, requeue); err != nil {
				return totalSynced, err
			}
		}
		totalSynced += len(heartbeats)
		if limit > 0 {
			remaining -= len(heartbeats)
			if remaining <= 0 {
				break
			}
		}
		if len(heartbeats) < batchLimit {
			break
		}
	}
	return totalSynced, nil
}

func postOfflineHeartbeats(opts Options, heartbeats []Heartbeat) ([]Heartbeat, error) {
	groups, targets, err := heartbeatTargetGroups(opts, heartbeats)
	if err != nil {
		return nil, err
	}
	var requeue []Heartbeat
	for _, target := range targets {
		targetOpts := opts
		targetOpts.APIURL = target.APIURL
		targetOpts.APIKey = target.APIKey
		client, err := NewClient(targetOpts)
		if err != nil {
			return nil, err
		}
		body, err := client.PostJSON(context.Background(), "/users/current/heartbeats.bulk", groups[target])
		if err != nil {
			return nil, err
		}
		groupRequeue, err := offlineRequeueFromBulkResponse(body, groups[target])
		if err != nil {
			return nil, err
		}
		requeue = append(requeue, groupRequeue...)
	}
	return requeue, nil
}
