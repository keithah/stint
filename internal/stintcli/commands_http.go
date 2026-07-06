package stintcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

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

func pathWithQuery(path string, values url.Values) string {
	if encoded := values.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
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

func nextValueArg(args []string, i int, flag string) (string, int, error) {
	if i+1 >= len(args) {
		return "", i, fmt.Errorf("%s requires a value", flag)
	}
	return strings.TrimSpace(args[i+1]), i + 1, nil
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
	userBody, userErr := client.Get(context.Background(), "/users/current")
	count, err := CountQueue(opts.QueuePath)
	return writeDoctorOutput(stdout, opts, body, userBody, count, err, userErr)
}

func writeDoctorOutput(stdout io.Writer, opts Options, body, userBody []byte, offlineCount int, countErr, userErr error) error {
	format := opts.Output
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
	meta := doctorMeta{}
	_ = json.Unmarshal(body, &meta)
	user := doctorCurrentUser{}
	if userErr == nil {
		_ = json.Unmarshal(userBody, &user)
	}
	apiURL := first(opts.APIURL, meta.Data.APIURL)
	username := first(user.Data.GitHubUsername, user.Data.Username, user.Data.Email)
	fmt.Fprintf(stdout, "Stint CLI %s\n", Version())
	fmt.Fprintf(stdout, "Config: %s\n", doctorConfigPath(opts))
	fmt.Fprintf(stdout, "API: %s\n", apiURL)
	if username != "" {
		fmt.Fprintf(stdout, "Auth: connected as @%s\n", strings.TrimPrefix(username, "@"))
		fmt.Fprintln(stdout, "Status: Stint CLI is connected")
	} else if userErr != nil {
		fmt.Fprintf(stdout, "Auth: not connected (%v)\n", userErr)
		fmt.Fprintln(stdout, "Status: Stint CLI is not connected")
	} else {
		fmt.Fprintln(stdout, "Auth: not connected")
		fmt.Fprintln(stdout, "Status: Stint CLI is not connected")
	}
	if meta.Data.Version != "" {
		fmt.Fprintf(stdout, "Server: %s\n", meta.Data.Version)
	}
	if countErr == nil {
		fmt.Fprintf(stdout, "offline_queue_count=%d\n", offlineCount)
	}
	return nil
}

func doctorConfigPath(opts Options) string {
	nativePath := expandHome(DefaultStintConfigPath())
	native, err := LoadConfig(nativePath)
	if err == nil && (native.Get("settings", "api_url") != "" || native.Get("settings", "api_key") != "") {
		return nativePath
	}
	return opts.ConfigPath
}

type doctorMeta struct {
	Data struct {
		APIURL  string `json:"api_url"`
		Version string `json:"version"`
	} `json:"data"`
}

type doctorCurrentUser struct {
	Data struct {
		GitHubUsername string `json:"github_username"`
		Username       string `json:"username"`
		Email          string `json:"email"`
	} `json:"data"`
}
