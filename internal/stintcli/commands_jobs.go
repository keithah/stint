package stintcli

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

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
	contentType := "application/json"
	var body io.Reader = stdin
	var cleanup func() error
	if useStdin {
		body = stdin
	} else {
		body, contentType, cleanup, err = multipartImportFile(importPath)
		if err != nil {
			return err
		}
	}
	if cleanup != nil {
		defer cleanup()
	}
	response, err := client.PostRaw(context.Background(), "/users/current/imports/wakatime", contentType, body)
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

func multipartImportFile(importPath string) (io.Reader, string, func() error, error) {
	file, err := os.Open(expandHome(importPath))
	if err != nil {
		return nil, "", nil, err
	}
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	go func() {
		part, err := multipartWriter.CreateFormFile("file", filepath.Base(importPath))
		if err == nil {
			_, err = io.Copy(part, file)
		}
		if closeErr := multipartWriter.Close(); err == nil {
			err = closeErr
		}
		_ = file.Close()
		_ = writer.CloseWithError(err)
	}()
	cleanup := func() error {
		_ = reader.Close()
		return file.Close()
	}
	return reader, multipartWriter.FormDataContentType(), cleanup, nil
}
