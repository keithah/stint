package stintcli

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

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
