package collector

import (
	"testing"

	"github.com/keithah/stint/internal/usage"
)

const qwenFixtureDir = "testdata/qwen"

func TestQwenScan(t *testing.T) {
	events, report, err := scanQwen([]string{qwenFixtureDir}, NewState())
	if err != nil {
		t.Fatalf("scanQwen: %v", err)
	}

	// Fixture: 1 user (non-usage), 2 usage rows for the SAME message (dup),
	// 1 distinct usage row, 1 malformed (truncated JSON).
	if report.FilesScanned != 1 {
		t.Errorf("FilesScanned = %d, want 1", report.FilesScanned)
	}
	if report.LinesParsed != 5 {
		t.Errorf("LinesParsed = %d, want 5", report.LinesParsed)
	}
	if report.EventsEmitted != 3 { // pre-dedup
		t.Errorf("EventsEmitted = %d, want 3", report.EventsEmitted)
	}
	if report.Errors != 1 {
		t.Errorf("Errors = %d, want 1 (malformed line)", report.Errors)
	}
	if report.LinesSkipped != 2 { // user + malformed
		t.Errorf("LinesSkipped = %d, want 2", report.LinesSkipped)
	}

	if len(events) != 2 {
		t.Fatalf("deduped events = %d, want 2", len(events))
	}

	byMsg := map[string]usage.Event{}
	for _, e := range events {
		byMsg[e.MessageID] = e
	}

	e1 := byMsg["qmsg_001"]
	// usageMetadata: prompt inclusive of cached => input = 1000 - 300 = 700.
	if e1.InputTokens != 700 || e1.OutputTokens != 200 || e1.CacheReadTokens != 300 || e1.ReasoningTokens != 50 {
		t.Errorf("qmsg_001 tokens wrong: %+v", e1)
	}
	if e1.Model != "qwen3-coder" {
		t.Errorf("qmsg_001 Model = %q", e1.Model)
	}
	if e1.Project != "myproj" {
		t.Errorf("qmsg_001 Project = %q, want myproj", e1.Project)
	}
	if e1.BillingType != usage.BillingAPI {
		t.Errorf("qmsg_001 BillingType = %q", e1.BillingType)
	}

	e2 := byMsg["qmsg_002"]
	// usage block: input = 500 - 100 = 400.
	if e2.InputTokens != 400 || e2.OutputTokens != 120 || e2.CacheReadTokens != 100 || e2.ReasoningTokens != 40 {
		t.Errorf("qmsg_002 tokens wrong: %+v", e2)
	}

	var sumIn, sumOut, sumRead, sumReason int
	for _, e := range events {
		sumIn += e.InputTokens
		sumOut += e.OutputTokens
		sumRead += e.CacheReadTokens
		sumReason += e.ReasoningTokens
	}
	if sumIn != 1100 || sumOut != 320 || sumRead != 400 || sumReason != 90 {
		t.Errorf("sums wrong: in=%d out=%d read=%d reason=%d", sumIn, sumOut, sumRead, sumReason)
	}
}

func TestQwenScanDeterministic(t *testing.T) {
	a, _, _ := scanQwen([]string{qwenFixtureDir}, NewState())
	b, _, _ := scanQwen([]string{qwenFixtureDir}, NewState())
	if len(a) != len(b) {
		t.Fatalf("len mismatch %d vs %d", len(a), len(b))
	}
}
