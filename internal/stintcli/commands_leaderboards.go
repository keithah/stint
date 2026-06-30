package stintcli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

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
