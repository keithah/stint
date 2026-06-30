package stintcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

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
