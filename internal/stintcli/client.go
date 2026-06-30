package stintcli

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/net/http/httpproxy"
)

type Client struct {
	apiKey          string
	apiURL          string
	client          *http.Client
	logFile         string
	logWriter       io.Writer
	machineName     string
	plugin          string
	proxyAuthHeader string
	timezone        string
}

const (
	maxClientResponseBytes = 8 << 20
	wakaTimeAPIURL         = "https://api.wakatime.com/api/v1"
	wakaTimeAPIIPv4        = "143.244.210.202"
	wakaTimeAPIIPv6        = "2604:a880:4:1d0::2a7:b000"
)

var (
	maxLogFileSizeBytes int64 = 25 * 1024 * 1024
	maxLogFileBackups         = 4
)

type proxyFallbackTransport struct {
	primary  http.RoundTripper
	fallback http.RoundTripper
}

func (t proxyFallbackTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.primary.RoundTrip(req)
	if err == nil || !shouldRetryProxyWithHTTP(err) {
		return resp, err
	}
	retryReq := req.Clone(req.Context())
	if req.Body != nil {
		if req.GetBody == nil {
			return nil, err
		}
		body, bodyErr := req.GetBody()
		if bodyErr != nil {
			return nil, err
		}
		retryReq.Body = body
	}
	return t.fallback.RoundTrip(retryReq)
}

func DefaultLogFilePath() string {
	return filepath.Join(wakaResourcesDir(), "wakatime.log")
}

func NewClient(opts Options) (*Client, error) {
	return newClient(opts, true)
}

func NewPublicClient(opts Options) (*Client, error) {
	return newClient(opts, false)
}

func newClient(opts Options, requireAPIKey bool) (*Client, error) {
	apiURL, err := normalizeAPIURL(opts.APIURL)
	if err != nil {
		return nil, fmt.Errorf("invalid api url: %w", err)
	}
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	if requireAPIKey && opts.APIKey == "" {
		return nil, fmt.Errorf("api key is required (set --key, STINT_API_KEY, WAKATIME_API_KEY, or api_key in ~/.wakatime.cfg)")
	}
	if opts.Proxy == "" {
		opts.Proxy = proxyFromEnvironment(apiURL)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if opts.Proxy == "" {
		transport.Proxy = nil
	}
	proxyAuthHeader := ""
	var fallbackTransport http.RoundTripper
	if opts.Proxy != "" {
		proxyURL, ntlmCreds, err := proxyConfig(opts.Proxy)
		if err != nil {
			return nil, err
		}
		if proxyURL != nil {
			transport.Proxy = http.ProxyURL(proxyURL)
			if strings.EqualFold(proxyURL.Scheme, "https") {
				httpProxyURL := *proxyURL
				httpProxyURL.Scheme = "http"
				fallback := transport.Clone()
				fallback.Proxy = http.ProxyURL(&httpProxyURL)
				fallbackTransport = fallback
			}
		}
		if ntlmCreds != "" {
			proxyAuthHeader = basicAuthHeader(ntlmCreds)
			transport.ProxyConnectHeader = http.Header{"Proxy-Authorization": []string{proxyAuthHeader}}
		}
	}
	if opts.NoSSLVerify || opts.SSLCertsFile != "" {
		tlsConfig := &tls.Config{InsecureSkipVerify: opts.NoSSLVerify} //nolint:gosec // WakaTime-compatible explicit user opt-out.
		if opts.SSLCertsFile != "" {
			pemBytes, err := os.ReadFile(expandHome(opts.SSLCertsFile))
			if err != nil {
				return nil, fmt.Errorf("read ssl certs file: %w", err)
			}
			pool, err := x509.SystemCertPool()
			if err != nil || pool == nil {
				pool = x509.NewCertPool()
			}
			if !pool.AppendCertsFromPEM(pemBytes) {
				return nil, fmt.Errorf("ssl certs file contains no PEM certificates")
			}
			tlsConfig.RootCAs = pool
		}
		transport.TLSClientConfig = tlsConfig
	}
	var roundTripper http.RoundTripper = transport
	if fallbackTransport != nil {
		roundTripper = proxyFallbackTransport{primary: transport, fallback: fallbackTransport}
	}
	return newClientWithTransport(opts, apiURL, roundTripper, proxyAuthHeader), nil
}

func normalizeAPIURL(apiURL string) (string, error) {
	apiURL = strings.TrimSpace(apiURL)
	apiURL = strings.TrimSuffix(apiURL, "/")
	apiURL = strings.TrimSuffix(apiURL, ".bulk")
	apiURL = strings.TrimSuffix(apiURL, "/users/current/heartbeats")
	apiURL = strings.TrimSuffix(apiURL, "/heartbeats")
	apiURL = strings.TrimSuffix(apiURL, "/heartbeat")
	if apiURL == "" {
		return "", nil
	}
	if _, err := url.Parse(apiURL); err != nil {
		return "", err
	}
	return apiURL, nil
}

func newClientWithTransport(opts Options, apiURL string, transport http.RoundTripper, proxyAuthHeader string) *Client {
	var logWriter io.Writer
	if opts.LogToStdout {
		logWriter = opts.LogWriter
		if logWriter == nil {
			logWriter = os.Stdout
		}
	}
	return &Client{
		apiKey:          opts.APIKey,
		apiURL:          apiURL,
		client:          &http.Client{Timeout: time.Duration(opts.Timeout) * time.Second, Transport: transport},
		logFile:         expandHome(opts.LogFile),
		logWriter:       logWriter,
		machineName:     machineName(opts.Hostname),
		plugin:          opts.Plugin,
		proxyAuthHeader: proxyAuthHeader,
		timezone:        localTimezoneName(),
	}
}

func proxyConfig(raw string) (*url.URL, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, "", nil
	}
	if strings.Contains(raw, `\`) {
		if !strings.Contains(raw, `\\`) {
			return nil, "", fmt.Errorf("invalid ntlm proxy credentials %q: expected domain\\\\user[:password]", raw)
		}
		return nil, raw, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	proxyURL, err := url.Parse(raw)
	if err != nil {
		return nil, "", fmt.Errorf("parse proxy URL: %w", err)
	}
	switch strings.ToLower(proxyURL.Scheme) {
	case "http", "https", "socks5":
	default:
		return nil, "", fmt.Errorf("unsupported proxy scheme %q", proxyURL.Scheme)
	}
	if proxyURL.Host == "" {
		return nil, "", fmt.Errorf("proxy URL is missing host")
	}
	return proxyURL, "", nil
}

func proxyFromEnvironment(apiURL string) string {
	parsed, err := url.Parse(apiURL)
	if err != nil || parsed == nil {
		return ""
	}
	if urlShapedNoProxyMatches(parsed) {
		return ""
	}
	proxyURL, err := httpproxy.FromEnvironment().ProxyFunc()(parsed)
	if err != nil || proxyURL == nil {
		return ""
	}
	return proxyURL.String()
}

func urlShapedNoProxyMatches(apiURL *url.URL) bool {
	for _, envName := range []string{"NO_PROXY", "no_proxy"} {
		for _, raw := range strings.Split(os.Getenv(envName), ",") {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			noProxyURL, err := url.Parse(raw)
			if err != nil || noProxyURL.Scheme == "" || noProxyURL.Host == "" {
				continue
			}
			if !strings.EqualFold(noProxyURL.Scheme, apiURL.Scheme) {
				continue
			}
			if !strings.EqualFold(noProxyURL.Hostname(), apiURL.Hostname()) {
				continue
			}
			if noProxyURL.Port() != "" && noProxyURL.Port() != apiURL.Port() {
				continue
			}
			return true
		}
	}
	return false
}

func basicAuthHeader(creds string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(creds))
}

func shouldRetryProxyWithHTTP(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "proxyconnect tcp:") ||
		strings.Contains(msg, "server gave HTTP response to HTTPS client")
}

func localTimezoneName() string {
	if tz := strings.TrimSpace(os.Getenv("TZ")); tz != "" {
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if _, name, ok := strings.Cut(filepath.ToSlash(target), "/zoneinfo/"); ok {
			if _, err := time.LoadLocation(name); err == nil {
				return name
			}
		}
	}
	return "UTC"
}

func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(path), nil)
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

func (c *Client) GetRoot(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.rootURL(path), nil)
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

func (c *Client) PostJSON(ctx context.Context, path string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(path), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req)
}

func (c *Client) PostEmpty(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(path), nil)
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

func (c *Client) PostRaw(ctx context.Context, path, contentType string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(path), body)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(contentType) != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.do(req)
}

func (c *Client) DeleteJSON(ctx context.Context, path string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.url(path), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req)
}

func (c *Client) PostOAuthForm(ctx context.Context, path string, values url.Values, clientID, clientSecret string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.oauthURL(path), strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent(c.plugin))
	if c.proxyAuthHeader != "" {
		req.Header.Set("Proxy-Authorization", c.proxyAuthHeader)
	}
	req.SetBasicAuth(clientID, clientSecret)
	resp, err := c.client.Do(req)
	if err != nil {
		c.logf("%s %s error: %v", req.Method, req.URL.String(), err)
		return nil, err
	}
	return c.readResponse(req, resp)
}

func (c *Client) PutRaw(ctx context.Context, path, contentType string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.url(path), body)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(contentType) != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.do(req)
}

func (c *Client) Delete(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.url(path), nil)
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

type diagnosticsPayload struct {
	Architecture string `json:"architecture"`
	CLIVersion   string `json:"cli_version"`
	ErrorMessage string `json:"error_message,omitempty"`
	IsPanic      bool   `json:"is_panic,omitempty"`
	Logs         string `json:"logs,omitempty"`
	Platform     string `json:"platform"`
	Plugin       string `json:"plugin"`
	Stacktrace   string `json:"stacktrace,omitempty"`
}

func (c *Client) SendDiagnostics(ctx context.Context, errorMessage, logs, stack string, panicked bool) error {
	payload := diagnosticsPayload{
		Architecture: runtime.GOARCH,
		CLIVersion:   Version(),
		ErrorMessage: errorMessage,
		IsPanic:      panicked,
		Logs:         logs,
		Platform:     runtime.GOOS,
		Plugin:       c.plugin,
		Stacktrace:   stack,
	}
	_, err := c.PostJSON(ctx, "/plugins/errors", payload)
	return err
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	req.Header.Set("Accept", "application/json")
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", basicAuthHeader(c.apiKey))
	}
	req.Header.Set("User-Agent", userAgent(c.plugin))
	if c.machineName != "" {
		req.Header.Set("X-Machine-Name", url.QueryEscape(c.machineName))
	}
	if c.timezone != "" {
		req.Header.Set("Timezone", c.timezone)
	}
	if c.proxyAuthHeader != "" {
		req.Header.Set("Proxy-Authorization", c.proxyAuthHeader)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		if resp, retryErr := c.retryWakaTimeDNSFallback(req, err); retryErr == nil {
			return c.readResponse(req, resp)
		}
		c.logf("%s %s error: %v", req.Method, req.URL.String(), err)
		return nil, err
	}
	return c.readResponse(req, resp)
}

func (c *Client) readResponse(req *http.Request, resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxClientResponseBytes+1))
	if readErr != nil {
		return nil, readErr
	}
	if len(body) > maxClientResponseBytes {
		return nil, fmt.Errorf("%s %s: response body exceeds %d bytes", req.Method, req.URL.String(), maxClientResponseBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logf("%s %s status=%d", req.Method, req.URL.String(), resp.StatusCode)
		if len(body) == 0 {
			return nil, fmt.Errorf("%s %s: status %d", req.Method, req.URL.String(), resp.StatusCode)
		}
		return nil, fmt.Errorf("%s %s: status %d: %s", req.Method, req.URL.String(), resp.StatusCode, strings.TrimSpace(string(body)))
	}
	c.logf("%s %s status=%d", req.Method, req.URL.String(), resp.StatusCode)
	return body, nil
}

func (c *Client) retryWakaTimeDNSFallback(req *http.Request, originalErr error) (*http.Response, error) {
	if !strings.HasPrefix(c.apiURL, wakaTimeAPIURL) {
		return nil, originalErr
	}
	var dnsErr *net.DNSError
	if !errors.As(originalErr, &dnsErr) {
		return nil, originalErr
	}
	retryReq := req.Clone(req.Context())
	if req.Body != nil {
		if req.GetBody == nil {
			return nil, originalErr
		}
		body, err := req.GetBody()
		if err != nil {
			return nil, originalErr
		}
		retryReq.Body = body
	}
	retryReq.URL = cloneURL(req.URL)
	retryReq.URL.Host = wakaTimeFallbackIP()
	c.logf("dns error, retrying %s %s with host ip %s: %v", req.Method, req.URL.String(), retryReq.URL.Host, originalErr)
	resp, err := c.wakaTimeDNSFallbackClient().Do(retryReq)
	if err != nil {
		return nil, fmt.Errorf("retry request failed: %w. original error: %v", err, originalErr)
	}
	return resp, nil
}

func (c *Client) wakaTimeDNSFallbackClient() *http.Client {
	client := *c.client
	if transport, ok := c.client.Transport.(*http.Transport); ok {
		fallback := transport.Clone()
		tlsConfig := fallback.TLSClientConfig
		if tlsConfig == nil {
			tlsConfig = &tls.Config{}
		} else {
			tlsConfig = tlsConfig.Clone()
		}
		tlsConfig.MinVersion = tls.VersionTLS12
		tlsConfig.ServerName = "api.wakatime.com"
		fallback.TLSClientConfig = tlsConfig
		client.Transport = fallback
	}
	return &client
}

func cloneURL(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}
	clone := *u
	return &clone
}

func wakaTimeFallbackIP() string {
	conn, err := net.Dial("udp", wakaTimeAPIIPv4+":80")
	if err != nil {
		return wakaTimeAPIIPv6
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok && addr.IP.To4() == nil {
		return wakaTimeAPIIPv6
	}
	return wakaTimeAPIIPv4
}

func (c *Client) logf(format string, args ...any) {
	lineArgs := append([]any{time.Now().UTC().Format(time.RFC3339)}, args...)
	if c.logWriter != nil {
		_, _ = fmt.Fprintf(c.logWriter, "%s "+format+"\n", lineArgs...)
	}
	if c.logFile == "" || c.logWriter != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.logFile), 0o750); err != nil {
		return
	}
	_ = rotateLogFile(c.logFile)
	f, err := os.OpenFile(c.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s "+format+"\n", lineArgs...)
}

func rotateLogFile(path string) error {
	if maxLogFileSizeBytes <= 0 || maxLogFileBackups <= 0 {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Size() < maxLogFileSizeBytes {
		return nil
	}
	for i := maxLogFileBackups; i >= 1; i-- {
		current := fmt.Sprintf("%s.%d", path, i)
		if i == maxLogFileBackups {
			if err := os.Remove(current); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		next := fmt.Sprintf("%s.%d", path, i+1)
		if err := os.Rename(current, next); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return os.Rename(path, path+".1")
}

func (c *Client) url(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.apiURL + path
}

func (c *Client) oauthURL(path string) string {
	return c.rootURL(path)
}

func (c *Client) rootURL(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	base := strings.TrimRight(c.apiURL, "/")
	base = strings.TrimSuffix(base, "/api/v1")
	return base + path
}
