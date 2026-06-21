package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestHeartbeatBulkResultUsesWakaTimeTupleShape(t *testing.T) {
	result := heartbeatBulkResult(http.StatusCreated, map[string]any{
		"data": map[string]any{"id": "heartbeat-id"},
	})

	raw, err := json.Marshal(map[string]any{"responses": []any{result}})
	if err != nil {
		t.Fatalf("marshal bulk response: %v", err)
	}

	var decoded struct {
		Responses [][]json.RawMessage `json:"responses"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("expected WakaTime tuple response shape, got %s: %v", raw, err)
	}
	if len(decoded.Responses) != 1 || len(decoded.Responses[0]) != 2 {
		t.Fatalf("unexpected tuple response shape: %s", raw)
	}
	var status int
	if err := json.Unmarshal(decoded.Responses[0][1], &status); err != nil {
		t.Fatalf("status was not tuple element 1: %v", err)
	}
	if status != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, status)
	}
}

func TestHeartbeatBulkErrorUsesWakaTimeTupleShape(t *testing.T) {
	result := heartbeatBulkError(http.StatusBadRequest, "entity is required")

	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal bulk error: %v", err)
	}
	var decoded []json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("expected tuple response shape, got %s: %v", raw, err)
	}
	if len(decoded) != 2 {
		t.Fatalf("expected tuple with body and status, got %s", raw)
	}
	var body map[string]string
	if err := json.Unmarshal(decoded[0], &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] != "entity is required" {
		t.Fatalf("expected WakaTime error body, got %#v", body)
	}
	var status int
	if err := json.Unmarshal(decoded[1], &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, status)
	}
}
