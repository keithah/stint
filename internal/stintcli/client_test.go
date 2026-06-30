package stintcli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestNewClientUsesProxySSLAndTimeoutOptions(t *testing.T) {
	proxyURL, err := url.Parse("http://127.0.0.1:9999")
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(Options{APIURL: "https://example.test/api/v1", APIKey: "waka_test", Proxy: proxyURL.String(), NoSSLVerify: true, Timeout: 7})
	if err != nil {
		t.Fatal(err)
	}
	if client.client.Timeout.String() != "7s" {
		t.Fatalf("timeout = %s", client.client.Timeout)
	}
	transport := client.client.Transport.(*http.Transport)
	req := httptest.NewRequest(http.MethodGet, "https://example.test", nil)
	gotProxy, err := transport.Proxy(req)
	if err != nil {
		t.Fatal(err)
	}
	if gotProxy.String() != proxyURL.String() {
		t.Fatalf("proxy = %s", gotProxy)
	}
	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("expected TLS InsecureSkipVerify, got %#v", transport.TLSClientConfig)
	}
}

func TestNewClientKeepsProxyFromEnvironmentFallback(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9998")
	t.Setenv("HTTP_PROXY", "")
	t.Setenv("NO_PROXY", "")
	client, err := NewClient(Options{APIURL: "https://example.test/api/v1", APIKey: "waka_test", Timeout: 7})
	if err != nil {
		t.Fatal(err)
	}
	transport := client.client.Transport.(*http.Transport)
	req := httptest.NewRequest(http.MethodGet, "https://example.test", nil)
	gotProxy, err := testTransportProxy(transport, req)
	if err != nil {
		t.Fatal(err)
	}
	if gotProxy == nil || gotProxy.String() != "http://127.0.0.1:9998" {
		t.Fatalf("proxy = %v", gotProxy)
	}
}

func TestClientRetriesHTTPSProxyAsHTTPForPlainProxy(t *testing.T) {
	var calls int
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.String() != "http://example.test/api/v1/meta" {
			t.Fatalf("proxy request URL = %q", r.URL.String())
		}
		_, _ = w.Write([]byte(`{"data":true}`))
	}))
	defer proxy.Close()

	client, err := NewClient(Options{
		APIURL:  "http://example.test/api/v1",
		APIKey:  "waka_test",
		Proxy:   strings.Replace(proxy.URL, "http://", "https://", 1),
		Timeout: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := client.Get(context.Background(), "/meta")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != `{"data":true}` || calls != 1 {
		t.Fatalf("body=%q calls=%d", body, calls)
	}
}

func TestNewClientAcceptsNTLMProxyCredentialsWithoutClobberingAPIAuth(t *testing.T) {
	var auth, proxyAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		proxyAuth = r.Header.Get("Proxy-Authorization")
		_, _ = w.Write([]byte(`{"data":true}`))
	}))
	defer server.Close()
	client, err := NewClient(Options{APIURL: server.URL + "/api/v1", APIKey: "waka_test", Proxy: `domain\\john:secret`, Timeout: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(context.Background(), "/meta"); err != nil {
		t.Fatal(err)
	}
	if auth != basicAuthHeader("waka_test") {
		t.Fatalf("authorization = %q", auth)
	}
	expectedProxyAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(`domain\\john:secret`))
	if proxyAuth != expectedProxyAuth {
		t.Fatalf("proxy auth = %q, want %q", proxyAuth, expectedProxyAuth)
	}
	transport := client.client.Transport.(*http.Transport)
	if got := transport.ProxyConnectHeader.Get("Proxy-Authorization"); got != expectedProxyAuth {
		t.Fatalf("proxy connect auth = %q", got)
	}
}

func TestClientDoesNotRetryCustomAPIDNSFailure(t *testing.T) {
	var calls int
	client, err := NewClient(Options{APIURL: "https://custom.example.com/api/v1", APIKey: "waka_test", Timeout: 1})
	if err != nil {
		t.Fatal(err)
	}
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, &net.DNSError{Err: "no such host", Name: r.URL.Host}
	})
	_, err = client.Get(context.Background(), "/meta")
	if err == nil {
		t.Fatal("expected dns error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestClientRejectsOversizedResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), maxClientResponseBytes+1))
	}))
	defer server.Close()
	client, err := NewClient(Options{APIURL: server.URL + "/api/v1", APIKey: "waka_test", Timeout: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get(context.Background(), "/meta")
	if err == nil || !strings.Contains(err.Error(), "response body exceeds") {
		t.Fatalf("err = %v, want oversized response error", err)
	}
}

func TestClientCreatesLogDirectoryWithoutWorldPermissions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":true}`))
	}))
	defer server.Close()
	logDir := filepath.Join(t.TempDir(), "nested")
	client, err := NewClient(Options{APIURL: server.URL + "/api/v1", APIKey: "waka_test", LogFile: filepath.Join(logDir, "wakatime.log"), Timeout: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(t.Context(), "/meta"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(logDir)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode&0o007 != 0 {
		t.Fatalf("log directory mode = %o, expected no world permissions", mode)
	}
}

func TestRunSendsDiagnosticsOnVerboseError(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(dir, "wakatime.log")
	if err := os.WriteFile(logFile, []byte("previous log line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var diagnostics diagnosticsPayload
	var diagnosticCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/users/current/heartbeats.bulk" {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		if r.URL.Path != "/api/v1/plugins/errors" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		diagnosticCalls++
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.Header.Get("Authorization") != basicAuthHeader("waka_test") {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&diagnostics); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	err := Run([]string{
		"--entity", file,
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--log-file", logFile,
		"--verbose",
		"--send-diagnostics-on-errors",
		"--disable-offline",
		"--sync-ai-disabled",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatalf("expected command error")
	}
	if diagnosticCalls != 1 {
		t.Fatalf("diagnostic calls = %d", diagnosticCalls)
	}
	if !strings.Contains(diagnostics.ErrorMessage, "status 500") {
		t.Fatalf("diagnostic error = %#v", diagnostics)
	}
	if !strings.Contains(diagnostics.Logs, "previous log line\n") || !strings.Contains(diagnostics.Logs, "/api/v1/users/current/heartbeats.bulk status=500") {
		t.Fatalf("diagnostic logs = %q", diagnostics.Logs)
	}
	if diagnostics.Plugin != "" || diagnostics.Platform == "" || diagnostics.Architecture == "" || diagnostics.CLIVersion == "" || diagnostics.Stacktrace == "" {
		t.Fatalf("diagnostics missing metadata: %#v", diagnostics)
	}
}

func TestRunDoesNotSendDiagnosticsOnNonVerboseError(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	diagnosticCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/plugins/errors" {
			diagnosticCalls++
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()
	err := Run([]string{
		"--entity", file,
		"--api-url", server.URL + "/api/v1",
		"--key", "waka_test",
		"--send-diagnostics-on-errors",
		"--disable-offline",
		"--sync-ai-disabled",
	}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatalf("expected command error")
	}
	if diagnosticCalls != 0 {
		t.Fatalf("diagnostic calls = %d", diagnosticCalls)
	}
}

func TestDiagnosticOptionsParsesAllReadCommandCommonFlags(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "wakatime.log")
	commands := [][]string{
		{"all-time"},
		{"machine-names"},
		{"user-agents"},
		{"events"},
		{"usage-events", "summary", "--range", "last_7_days"},
		{"insights", "languages", "last_7_days"},
		{"durations", "2026-06-28"},
		{"summaries", "2026-06-01", "2026-06-30"},
		{"pricing-sources"},
		{"pricing-models"},
	}
	for _, command := range commands {
		args := append([]string{}, command...)
		args = append(args,
			"--api-url", "http://diagnostics.example/api/v1",
			"--key", "waka_diagnostic",
			"--log-file", logFile,
			"--verbose",
			"--send-diagnostics-on-errors",
		)
		opts, err := diagnosticOptions(args)
		if err != nil {
			t.Fatalf("diagnosticOptions(%v): %v", command, err)
		}
		if opts.APIURL != "http://diagnostics.example/api/v1" || opts.APIKey != "waka_diagnostic" || opts.LogFile != logFile {
			t.Fatalf("diagnosticOptions(%v) missed common flags: %#v", command, opts)
		}
		if !opts.Verbose || !opts.SendDiagnosticsOnError {
			t.Fatalf("diagnosticOptions(%v) missed diagnostic flags: %#v", command, opts)
		}
	}
}
