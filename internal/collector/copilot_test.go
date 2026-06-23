package collector

import (
	"testing"

	"github.com/keithah/stint/internal/usage"
)

const copilotFixtureDir = "testdata/copilot/otel"

func TestCopilotScan(t *testing.T) {
	events, report, err := scanCopilot([]string{copilotFixtureDir}, NewState())
	if err != nil {
		t.Fatalf("scanCopilot: %v", err)
	}

	// Fixture has 5 NDJSON lines: span-aaa (usage), span-aaa dup (usage),
	// span-bbb (usage), span-ccc (tool, no usage), malformed.
	if report.FilesScanned != 1 {
		t.Errorf("FilesScanned = %d, want 1", report.FilesScanned)
	}
	if report.LinesParsed != 5 {
		t.Errorf("LinesParsed = %d, want 5", report.LinesParsed)
	}
	// 3 usage-bearing spans emit events (pre-dedup).
	if report.EventsEmitted != 3 {
		t.Errorf("EventsEmitted = %d, want 3", report.EventsEmitted)
	}
	// Only the malformed line is an error.
	if report.Errors != 1 {
		t.Errorf("Errors = %d, want 1 (malformed)", report.Errors)
	}
	// Skipped lines: tool span (no usage) + malformed = 2.
	if report.LinesSkipped != 2 {
		t.Errorf("LinesSkipped = %d, want 2", report.LinesSkipped)
	}

	// After dedup the duplicate span-aaa collapses => 2 events.
	if len(events) != 2 {
		t.Fatalf("deduped events = %d, want 2", len(events))
	}

	byReq := map[string]usage.Event{}
	for _, e := range events {
		byReq[e.RequestID] = e
	}

	a, ok := byReq["span-aaa"]
	if !ok {
		t.Fatal("missing span-aaa")
	}
	if a.InputTokens != 1500 || a.OutputTokens != 400 || a.CacheReadTokens != 600 {
		t.Errorf("span-aaa tokens wrong: %+v", a)
	}
	// response.model wins over request.model.
	if a.Model != "gpt-4o-2024-08-06" {
		t.Errorf("span-aaa Model = %q, want gpt-4o-2024-08-06", a.Model)
	}
	if a.MessageID != "resp-aaa" {
		t.Errorf("span-aaa MessageID = %q, want resp-aaa", a.MessageID)
	}
	if a.BillingType != usage.BillingSubscription {
		t.Errorf("span-aaa BillingType = %q, want subscription", a.BillingType)
	}
	if a.Timestamp == "" {
		t.Error("span-aaa Timestamp empty")
	}

	b, ok := byReq["span-bbb"]
	if !ok {
		t.Fatal("missing span-bbb")
	}
	if b.ReasoningTokens != 40 {
		t.Errorf("span-bbb ReasoningTokens = %d, want 40", b.ReasoningTokens)
	}
	if b.Model != "claude-3-5-sonnet" {
		t.Errorf("span-bbb Model = %q", b.Model)
	}

	// Aggregate token sums across the deduped set.
	var totIn, totOut, totRead, totReason int
	for _, e := range events {
		totIn += e.InputTokens
		totOut += e.OutputTokens
		totRead += e.CacheReadTokens
		totReason += e.ReasoningTokens
	}
	if totIn != 2300 { // 1500 + 800
		t.Errorf("total InputTokens = %d, want 2300", totIn)
	}
	if totOut != 520 { // 400 + 120
		t.Errorf("total OutputTokens = %d, want 520", totOut)
	}
	if totRead != 600 {
		t.Errorf("total CacheReadTokens = %d, want 600", totRead)
	}
	if totReason != 40 {
		t.Errorf("total ReasoningTokens = %d, want 40", totReason)
	}
}

// TestCopilotDedup verifies feeding the same data twice collapses via eventId.
func TestCopilotDedup(t *testing.T) {
	e1, _, err := scanCopilot([]string{copilotFixtureDir}, NewState())
	if err != nil {
		t.Fatalf("scan1: %v", err)
	}
	e2, _, err := scanCopilot([]string{copilotFixtureDir}, NewState())
	if err != nil {
		t.Fatalf("scan2: %v", err)
	}
	combined := usage.Dedup(append(append([]usage.Event{}, e1...), e2...))
	if len(combined) != len(e1) {
		t.Errorf("dedup of doubled events = %d, want %d", len(combined), len(e1))
	}
}
