package stintcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

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
