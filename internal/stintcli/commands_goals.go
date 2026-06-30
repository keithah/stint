package stintcli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
)

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
