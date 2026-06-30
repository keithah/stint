package stintcli

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func between(source, start, end string) string {
	_, tail, ok := strings.Cut(source, start)
	if !ok {
		return ""
	}
	head, _, ok := strings.Cut(tail, end)
	if !ok {
		return tail
	}
	return head
}

func after(source, marker string) string {
	_, tail, ok := strings.Cut(source, marker)
	if !ok {
		return ""
	}
	return tail
}

func testTransportProxy(transport *http.Transport, req *http.Request) (*url.URL, error) {
	if transport.Proxy == nil {
		return nil, nil
	}
	return transport.Proxy(req)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func bulkResponseFor(heartbeats []Heartbeat, status int) []byte {
	responses := make([]any, 0, len(heartbeats))
	for _, hb := range heartbeats {
		responses = append(responses, []any{map[string]any{"data": hb}, status})
	}
	body, _ := json.Marshal(map[string]any{"responses": responses})
	return body
}

func writeTestJSONLines(t *testing.T, path string, values []map[string]any) {
	t.Helper()
	var lines []string
	for _, value := range values {
		lines = append(lines, testJSONString(t, value))
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
}

func testJSONString(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
