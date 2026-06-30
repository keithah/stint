package stintcli

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
)

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
