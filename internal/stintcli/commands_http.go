package stintcli

import (
	"bytes"
	"context"
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
