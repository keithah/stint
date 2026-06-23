package collector

import (
	"testing"

	"github.com/keithah/stint/internal/usage"
)

const openClawFixtureDir = "testdata/openclaw/agents"

func TestOpenClawScan(t *testing.T) {
	events, report, err := scanOpenClaw([]string{openClawFixtureDir}, NewState())
	if err != nil {
		t.Fatalf("scanOpenClaw: %v", err)
	}

	// Fixture has 6 lines: user, assistant(usage), assistant(dup usage),
	// assistant(usage), tool_result, malformed.
	if report.FilesScanned != 1 {
		t.Errorf("FilesScanned = %d, want 1", report.FilesScanned)
	}
	if report.LinesParsed != 6 {
		t.Errorf("LinesParsed = %d, want 6", report.LinesParsed)
	}
	// 3 assistant usage lines emit events (pre-dedup).
	if report.EventsEmitted != 3 {
		t.Errorf("EventsEmitted = %d, want 3", report.EventsEmitted)
	}
	// Only the malformed line is an error.
	if report.Errors != 1 {
		t.Errorf("Errors = %d, want 1 (malformed)", report.Errors)
	}
	// Skipped: user, tool_result, malformed = 3.
	if report.LinesSkipped != 3 {
		t.Errorf("LinesSkipped = %d, want 3", report.LinesSkipped)
	}

	// After dedup the duplicate msg-1 collapses => 2 events.
	if len(events) != 2 {
		t.Fatalf("deduped events = %d, want 2", len(events))
	}

	byMsg := map[string]usage.Event{}
	for _, e := range events {
		byMsg[e.MessageID] = e
	}

	m1, ok := byMsg["msg-1"]
	if !ok {
		t.Fatal("missing msg-1")
	}
	// input_tokens 1200 is inclusive of 1000 cache-read => InputTokens 200.
	if m1.InputTokens != 200 {
		t.Errorf("msg-1 InputTokens = %d, want 200 (1200-1000 cached)", m1.InputTokens)
	}
	if m1.CacheReadTokens != 1000 {
		t.Errorf("msg-1 CacheReadTokens = %d, want 1000", m1.CacheReadTokens)
	}
	if m1.CacheCreate5mTokens != 400 {
		t.Errorf("msg-1 CacheCreate5mTokens = %d, want 400", m1.CacheCreate5mTokens)
	}
	if m1.OutputTokens != 300 {
		t.Errorf("msg-1 OutputTokens = %d, want 300", m1.OutputTokens)
	}
	if m1.ReasoningTokens != 50 {
		t.Errorf("msg-1 ReasoningTokens = %d, want 50", m1.ReasoningTokens)
	}
	if m1.Model != "claude-sonnet-4-5" {
		t.Errorf("msg-1 Model = %q", m1.Model)
	}
	if m1.Project != "proj" {
		t.Errorf("msg-1 Project = %q, want proj", m1.Project)
	}
	if m1.SessionID != "ses-abc" {
		t.Errorf("msg-1 SessionID = %q, want ses-abc", m1.SessionID)
	}
	if m1.BillingType != usage.BillingSubscription {
		t.Errorf("msg-1 BillingType = %q, want subscription", m1.BillingType)
	}

	// Aggregate token sums across the deduped set (msg-1 + msg-2).
	var totIn, totOut, totRead, totCreate, totReason int
	for _, e := range events {
		totIn += e.InputTokens
		totOut += e.OutputTokens
		totRead += e.CacheReadTokens
		totCreate += e.CacheCreate5mTokens
		totReason += e.ReasoningTokens
	}
	if totIn != 700 { // 200 + 500
		t.Errorf("total InputTokens = %d, want 700", totIn)
	}
	if totOut != 380 { // 300 + 80
		t.Errorf("total OutputTokens = %d, want 380", totOut)
	}
	if totRead != 1000 {
		t.Errorf("total CacheReadTokens = %d, want 1000", totRead)
	}
	if totCreate != 400 {
		t.Errorf("total CacheCreate5mTokens = %d, want 400", totCreate)
	}
	if totReason != 50 {
		t.Errorf("total ReasoningTokens = %d, want 50", totReason)
	}
}

// TestOpenClawDedup verifies feeding the same data twice collapses via eventId.
func TestOpenClawDedup(t *testing.T) {
	e1, _, err := scanOpenClaw([]string{openClawFixtureDir}, NewState())
	if err != nil {
		t.Fatalf("scan1: %v", err)
	}
	e2, _, err := scanOpenClaw([]string{openClawFixtureDir}, NewState())
	if err != nil {
		t.Fatalf("scan2: %v", err)
	}
	combined := usage.Dedup(append(append([]usage.Event{}, e1...), e2...))
	if len(combined) != len(e1) {
		t.Errorf("dedup of doubled events = %d, want %d", len(combined), len(e1))
	}
}
