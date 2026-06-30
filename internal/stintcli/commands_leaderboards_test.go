package stintcli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRunLeaderboardsCreateUpdateDeleteAndMembers(t *testing.T) {
	seen := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.EscapedPath()
		seen[key] = true
		switch key {
		case "POST /api/v1/users/current/leaderboards":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["name"] != "CLI board" || payload["time_range"] != "last_7_days" {
				t.Fatalf("unexpected create payload: %#v", payload)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"id":"board-1","name":"CLI board"}}`))
		case "PUT /api/v1/users/current/leaderboards/board%201":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["name"] != "Updated board" {
				t.Fatalf("unexpected update payload: %#v", payload)
			}
			_, _ = w.Write([]byte(`{"data":{"id":"board 1","name":"Updated board"}}`))
		case "DELETE /api/v1/users/current/leaderboards/board%201":
			w.WriteHeader(http.StatusNoContent)
		case "POST /api/v1/users/current/leaderboards/board%201/members":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["username"] != "octocat" {
				t.Fatalf("unexpected member payload: %#v", payload)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"username":"octocat"}}`))
		case "DELETE /api/v1/users/current/leaderboards/board%201/members/user%201":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s", key)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := Run([]string{"leaderboards", "create", "--stdin", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, strings.NewReader(`{"name":"CLI board","time_range":"last_7_days"}`), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name":"CLI board"`) {
		t.Fatalf("create output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"leaderboards", "update", "board 1", "--stdin", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, strings.NewReader(`{"name":"Updated board","time_range":"last_30_days"}`), &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name":"Updated board"`) {
		t.Fatalf("update output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"leaderboards", "add-member", "board 1", "octocat", "--api-url", server.URL + "/api/v1", "--key", "waka_test", "--output", "raw-json"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"username":"octocat"`) {
		t.Fatalf("add-member output missing response: %q", out.String())
	}
	out.Reset()
	err = Run([]string{"leaderboards", "remove-member", "board 1", "user 1", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	err = Run([]string{"leaderboards", "delete", "board 1", "--api-url", server.URL + "/api/v1", "--key", "waka_test"}, nil, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"POST /api/v1/users/current/leaderboards",
		"PUT /api/v1/users/current/leaderboards/board%201",
		"POST /api/v1/users/current/leaderboards/board%201/members",
		"DELETE /api/v1/users/current/leaderboards/board%201/members/user%201",
		"DELETE /api/v1/users/current/leaderboards/board%201",
	} {
		if !seen[want] {
			t.Fatalf("expected request %s in %#v", want, seen)
		}
	}
}
