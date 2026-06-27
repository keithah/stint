package collector

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/keithah/stint/internal/usage"
)

func TestClientPost(t *testing.T) {
	var gotBody []byte
	var gotAuth, gotCT, gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"received":2,"inserted":2,"duplicates":0,"invalid":0}}`))
	}))
	defer srv.Close()

	events := []usage.Event{
		{Agent: "claude", MessageID: "m1", RequestID: "r1", Model: "x", InputTokens: 1},
		{Agent: "claude", MessageID: "m2", RequestID: "r2", Model: "x", OutputTokens: 2},
	}
	for i := range events {
		events[i].EnsureID()
	}

	c := NewClient(srv.URL, "secret-key")
	res, err := c.Post(context.Background(), events)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}

	if gotPath != "/users/current/usage_events.bulk" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q", gotCT)
	}

	// Body must be a JSON array of events.
	var arr []usage.Event
	if err := json.Unmarshal(gotBody, &arr); err != nil {
		t.Fatalf("body is not a JSON array: %v (%s)", err, gotBody)
	}
	if len(arr) != 2 {
		t.Errorf("posted %d events, want 2", len(arr))
	}

	if res.Received != 2 || res.Inserted != 2 || res.Duplicates != 0 || res.Invalid != 0 {
		t.Errorf("result = %+v", res)
	}
}

func TestClientPostBatches(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var arr []usage.Event
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &arr)
		_, _ = io.WriteString(w, `{"data":{"received":`+itoa(len(arr))+`,"inserted":`+itoa(len(arr))+`}}`)
	}))
	defer srv.Close()

	var events []usage.Event
	for i := 0; i < 1200; i++ {
		events = append(events, usage.Event{Agent: "claude", MessageID: "m" + itoa(i), InputTokens: 1})
		events[i].EnsureID()
	}
	c := NewClient(srv.URL, "k")
	c.BatchSize = 500
	res, err := c.Post(context.Background(), events)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 { // 500 + 500 + 200
		t.Errorf("calls = %d, want 3", calls)
	}
	if res.Received != 1200 {
		t.Errorf("received = %d, want 1200", res.Received)
	}
}

func TestClientPostNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"bad key"}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k")
	ev := usage.Event{Agent: "claude", MessageID: "m", InputTokens: 1}
	ev.EnsureID()
	_, err := c.Post(context.Background(), []usage.Event{ev})
	if err == nil {
		t.Fatal("expected error on 401")
	}
}

func TestClientPostRetriesTransientServerErrors(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, `temporary`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"received":1,"inserted":1}}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k")
	c.RetryBaseDelay = 1
	ev := usage.Event{Agent: "claude", MessageID: "m", InputTokens: 1}
	ev.EnsureID()
	res, err := c.Post(context.Background(), []usage.Event{ev})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if res.Inserted != 1 {
		t.Fatalf("inserted = %d, want 1", res.Inserted)
	}
}

func TestClientUsesTimeoutHTTPClientWhenNoneProvided(t *testing.T) {
	c := NewClient("http://example.test", "k")
	if got := c.httpClient().Timeout; got <= 0 {
		t.Fatalf("expected fallback HTTP client to have a timeout, got %s", got)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
