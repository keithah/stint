package api

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGoldenWakaTimeBulkHeartbeatResponse(t *testing.T) {
	payload := map[string]any{
		"responses": []any{
			heartbeatBulkResult(201, map[string]any{
				"data": map[string]any{
					"id":     "heartbeat-id",
					"entity": "/tmp/stint/main.go",
					"time":   1720000000.5,
				},
			}),
		},
	}

	assertGoldenJSON(t, "heartbeat_bulk_response.golden.json", payload)
}

func assertGoldenJSON(t *testing.T, name string, payload any) {
	t.Helper()

	got, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden payload: %v", err)
	}
	got = append(got, '\n')

	want, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read golden file %s: %v", name, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden response %s mismatch\nwant:\n%s\ngot:\n%s", name, want, got)
	}
}
