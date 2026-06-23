package collector

import (
	"testing"

	"github.com/keithah/stint/internal/usage"
)

const claudeFixtureDir = "testdata/claude"

func TestClaudeScan(t *testing.T) {
	events, report, err := scanClaude([]string{claudeFixtureDir}, NewState())
	if err != nil {
		t.Fatalf("scanClaude: %v", err)
	}

	// Fixture has 6 lines: 1 user, 2 identical assistant/usage (dup),
	// 1 distinct assistant/usage, 1 system, 1 malformed.
	if report.FilesScanned != 1 {
		t.Errorf("FilesScanned = %d, want 1", report.FilesScanned)
	}
	if report.LinesParsed != 6 {
		t.Errorf("LinesParsed = %d, want 6", report.LinesParsed)
	}
	// 3 usage lines emit events (before dedup); EventsEmitted counts pre-dedup.
	if report.EventsEmitted != 3 {
		t.Errorf("EventsEmitted = %d, want 3", report.EventsEmitted)
	}
	// Skipped: 1 user + 1 system = 2 (malformed is counted under Errors+Skipped too).
	if report.Errors != 1 {
		t.Errorf("Errors = %d, want 1 (malformed line)", report.Errors)
	}
	// user, system, malformed all increment LinesSkipped => 3.
	if report.LinesSkipped != 3 {
		t.Errorf("LinesSkipped = %d, want 3", report.LinesSkipped)
	}

	// After dedup the two identical msg_001 lines collapse to one.
	if len(events) != 2 {
		t.Fatalf("deduped events = %d, want 2", len(events))
	}

	byMsg := map[string]usage.Event{}
	for _, e := range events {
		byMsg[e.MessageID] = e
	}

	e1, ok := byMsg["msg_001"]
	if !ok {
		t.Fatal("missing msg_001")
	}
	// Explicit 5m/1h split must be parsed.
	if e1.CacheCreate5mTokens != 1500 || e1.CacheCreate1hTokens != 500 {
		t.Errorf("msg_001 cache split = (%d,%d), want (1500,500)", e1.CacheCreate5mTokens, e1.CacheCreate1hTokens)
	}
	if e1.InputTokens != 123 || e1.OutputTokens != 45 || e1.CacheReadTokens != 10000 {
		t.Errorf("msg_001 tokens wrong: %+v", e1)
	}
	if e1.RequestID != "req_001" {
		t.Errorf("msg_001 RequestID = %q, want req_001", e1.RequestID)
	}
	if e1.SessionID != "sess-abc" {
		t.Errorf("msg_001 SessionID = %q, want sess-abc", e1.SessionID)
	}
	if e1.Project != "proj" {
		t.Errorf("msg_001 Project = %q, want proj", e1.Project)
	}
	if e1.Model != "claude-sonnet-4-5-20250929" {
		t.Errorf("msg_001 Model = %q", e1.Model)
	}
	if e1.Timestamp != "2026-06-20T18:03:11Z" {
		t.Errorf("msg_001 Timestamp = %q, want RFC3339 UTC", e1.Timestamp)
	}

	e2, ok := byMsg["msg_002"]
	if !ok {
		t.Fatal("missing msg_002")
	}
	// No explicit split: lumped cache_creation_input_tokens => all into 5m.
	if e2.CacheCreate5mTokens != 750 || e2.CacheCreate1hTokens != 0 {
		t.Errorf("msg_002 cache = (%d,%d), want (750,0)", e2.CacheCreate5mTokens, e2.CacheCreate1hTokens)
	}

	// Exact sums across the deduped set.
	var sumIn, sumOut, sum5m, sum1h, sumRead int
	for _, e := range events {
		sumIn += e.InputTokens
		sumOut += e.OutputTokens
		sum5m += e.CacheCreate5mTokens
		sum1h += e.CacheCreate1hTokens
		sumRead += e.CacheReadTokens
	}
	if sumIn != 203 { // 123 + 80
		t.Errorf("sum input = %d, want 203", sumIn)
	}
	if sumOut != 345 { // 45 + 300
		t.Errorf("sum output = %d, want 345", sumOut)
	}
	if sum5m != 2250 { // 1500 + 750
		t.Errorf("sum 5m = %d, want 2250", sum5m)
	}
	if sum1h != 500 {
		t.Errorf("sum 1h = %d, want 500", sum1h)
	}
	if sumRead != 14096 { // 10000 + 4096
		t.Errorf("sum read = %d, want 14096", sumRead)
	}
}

// Property: scanning the same fixtures twice with fresh state yields an
// identical deduped event set.
func TestClaudeScanDeterministic(t *testing.T) {
	a, _, err := scanClaude([]string{claudeFixtureDir}, NewState())
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := scanClaude([]string{claudeFixtureDir}, NewState())
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != len(b) {
		t.Fatalf("len mismatch %d vs %d", len(a), len(b))
	}
	idsA := map[string]int{}
	for _, e := range a {
		idsA[e.EventID]++
	}
	for _, e := range b {
		idsA[e.EventID]--
	}
	for id, n := range idsA {
		if n != 0 {
			t.Errorf("event id %s differs between scans (delta %d)", id, n)
		}
	}
}

func TestRegistryStubs(t *testing.T) {
	reg := DefaultRegistry()
	if _, ok := reg["claude"]; !ok {
		t.Fatal("claude not registered")
	}
	for _, id := range []string{"codex", "cursor", "copilot", "gemini", "opencode",
		"goose", "zed", "amp", "qwen", "kimi", "kiro", "kilo", "roo", "cline",
		"hermes", "pi-agent", "openclaw", "factory-droid", "crush", "octofriend"} {
		e, ok := reg[id]
		if !ok {
			t.Errorf("missing stub agent %q", id)
			continue
		}
		ev, rep, err := e.Adapter.Scan(nil, NewState())
		if err != nil {
			t.Errorf("stub %s returned error: %v", id, err)
		}
		if len(ev) != 0 {
			t.Errorf("stub %s emitted %d events", id, len(ev))
		}
		if rep.Note != "not implemented" {
			t.Errorf("stub %s note = %q", id, rep.Note)
		}
	}
}
