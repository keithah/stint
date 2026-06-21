package services

import (
	"bytes"
	"compress/gzip"
	"testing"
)

func TestExtractHeartbeatsFromWakaTimeDumpDataArray(t *testing.T) {
	raw := []byte(`{"data":[{"entity":"/tmp/main.go","type":"file","time":1781859600,"project":"stint","language":"Go"}]}`)

	heartbeats, err := ExtractHeartbeatsFromWakaTimeDump(raw)
	if err != nil {
		t.Fatalf("ExtractHeartbeatsFromWakaTimeDump returned error: %v", err)
	}
	if len(heartbeats) != 1 {
		t.Fatalf("expected 1 heartbeat, got %d", len(heartbeats))
	}
	if heartbeats[0].Project != "stint" || heartbeats[0].Language != "Go" {
		t.Fatalf("unexpected heartbeat: %#v", heartbeats[0])
	}
}

func TestExtractHeartbeatsFromWakaTimeDumpDirectArrayDefaultsType(t *testing.T) {
	raw := []byte(`[{"entity":"/tmp/main.go","time":1781859600,"project":"stint","language":"Go"}]`)

	heartbeats, err := ExtractHeartbeatsFromWakaTimeDump(raw)
	if err != nil {
		t.Fatalf("ExtractHeartbeatsFromWakaTimeDump returned error: %v", err)
	}
	if len(heartbeats) != 1 {
		t.Fatalf("expected 1 heartbeat, got %d", len(heartbeats))
	}
	if heartbeats[0].Type != "file" {
		t.Fatalf("expected missing type to default to file, got %q", heartbeats[0].Type)
	}
}

func TestExtractHeartbeatsFromWakaTimeDumpHeartbeatsWrapper(t *testing.T) {
	raw := []byte(`{"heartbeats":[{"entity":"/tmp/main.go","type":"file","time":1781859600,"project":"wrapped","language":"Go"}]}`)

	heartbeats, err := ExtractHeartbeatsFromWakaTimeDump(raw)
	if err != nil {
		t.Fatalf("ExtractHeartbeatsFromWakaTimeDump returned error: %v", err)
	}
	if len(heartbeats) != 1 {
		t.Fatalf("expected 1 heartbeat, got %d", len(heartbeats))
	}
	if heartbeats[0].Project != "wrapped" {
		t.Fatalf("unexpected heartbeat: %#v", heartbeats[0])
	}
}

func TestExtractHeartbeatsFromWakaTimeDumpDaysWrapper(t *testing.T) {
	raw := []byte(`{
		"user":{"id":"1a7015db-ad47-471b-9101-0145c5bfdc34","username":"keithah"},
		"range":{"start":"2026-01-08","end":"2026-06-20"},
		"days":[
			{"date":"2026-01-08","heartbeats":[
				{"entity":"/tmp/one.go","time":1767922327.540152,"project":"opencode","language":"Go"},
				{"entity":"/tmp/two.ts","type":"file","time":1767922386.684614,"project":"opencode","language":"TypeScript","dependencies":["bun","zod"]}
			]},
			{"date":"2026-01-09","heartbeats":[
				{"entity":"/tmp/three.yml","time":1768008727.540152,"project":"opencode","language":"YAML"}
			]}
		]
	}`)

	heartbeats, err := ExtractHeartbeatsFromWakaTimeDump(raw)
	if err != nil {
		t.Fatalf("ExtractHeartbeatsFromWakaTimeDump returned error: %v", err)
	}
	if len(heartbeats) != 3 {
		t.Fatalf("expected 3 heartbeats, got %d", len(heartbeats))
	}
	if heartbeats[0].Type != "file" || heartbeats[1].Dependencies != "bun,zod" || heartbeats[2].Project != "opencode" {
		t.Fatalf("unexpected day heartbeats: %#v", heartbeats)
	}
}

func TestExtractHeartbeatsFromWakaTimeDumpGzipJSON(t *testing.T) {
	raw := gzipBytes(t, []byte(`{"data":[{"entity":"/tmp/main.go","time":1781859600,"project":"compressed","language":"Go"}]}`))

	heartbeats, err := ExtractHeartbeatsFromWakaTimeDump(raw)
	if err != nil {
		t.Fatalf("ExtractHeartbeatsFromWakaTimeDump returned error: %v", err)
	}
	if len(heartbeats) != 1 {
		t.Fatalf("expected 1 heartbeat, got %d", len(heartbeats))
	}
	if heartbeats[0].Project != "compressed" || heartbeats[0].Type != "file" {
		t.Fatalf("unexpected heartbeat: %#v", heartbeats[0])
	}
}

func TestExtractHeartbeatsFromWakaTimeDumpRejectsMissingHeartbeats(t *testing.T) {
	if _, err := ExtractHeartbeatsFromWakaTimeDump([]byte(`{"daily":[]}`)); err == nil {
		t.Fatal("expected missing heartbeat data to be rejected")
	}
}

func TestExtractHeartbeatsFromWakaTimeDumpExplainsProfileOnlyExport(t *testing.T) {
	_, err := ExtractHeartbeatsFromWakaTimeDump([]byte(`{"user":{"id":"1a7015db-ad47-471b-9101-0145c5bfdc34","username":"keithah"}}`))
	if err == nil {
		t.Fatal("expected profile-only export to be rejected")
	}
	if err.Error() != "import file contains WakaTime profile metadata but no heartbeat rows; export Heartbeats from WakaTime settings" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func gzipBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(raw); err != nil {
		t.Fatalf("gzip write failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}
	return buf.Bytes()
}
