package stintcli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
)

var uuid4Regex = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)

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
