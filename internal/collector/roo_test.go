package collector

import (
	"testing"

	"github.com/keithah/stint/internal/usage"
)

const rooFixtureDir = "testdata/roo"

func TestRooScan(t *testing.T) {
	events, report, err := scanRoo([]string{rooFixtureDir}, NewState())
	if err != nil {
		t.Fatalf("scanRoo: %v", err)
	}

	// Fixture: 1 say/text (non-usage), 2 assistant usage rows for the SAME
	// message (dup), 1 api_req_started metadata usage row, 1 malformed.
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
		t.Errorf("Errors = %d, want 1 (malformed)", report.Errors)
	}
	if report.LinesSkipped != 2 { // say/text + malformed
		t.Errorf("LinesSkipped = %d, want 2", report.LinesSkipped)
	}

	if len(events) != 2 {
		t.Fatalf("deduped events = %d, want 2", len(events))
	}

	var byMsg *usage.Event
	var apiReq *usage.Event
	for i := range events {
		if events[i].MessageID == "rmsg_001" {
			byMsg = &events[i]
		} else {
			apiReq = &events[i]
		}
	}
	if byMsg == nil || apiReq == nil {
		t.Fatalf("expected one rmsg_001 and one metadata event, got %+v", events)
	}

	// Explicit 5m/1h split must be honored.
	if byMsg.InputTokens != 200 || byMsg.OutputTokens != 80 ||
		byMsg.CacheCreate5mTokens != 700 || byMsg.CacheCreate1hTokens != 300 ||
		byMsg.CacheReadTokens != 5000 {
		t.Errorf("rmsg_001 tokens wrong: %+v", *byMsg)
	}
	if byMsg.Agent != "roo" {
		t.Errorf("agent = %q, want roo", byMsg.Agent)
	}

	// api_req_started metadata: flat counts, cacheWrites lumped into 5m, cost set.
	if apiReq.InputTokens != 150 || apiReq.OutputTokens != 60 ||
		apiReq.CacheCreate5mTokens != 400 || apiReq.CacheReadTokens != 2000 {
		t.Errorf("api_req tokens wrong: %+v", *apiReq)
	}
	if apiReq.CostUSDProvided == nil || *apiReq.CostUSDProvided != 0.0123 {
		t.Errorf("api_req cost = %v, want 0.0123", apiReq.CostUSDProvided)
	}
}

func TestRooScanDeterministic(t *testing.T) {
	a, _, _ := scanRoo([]string{rooFixtureDir}, NewState())
	b, _, _ := scanRoo([]string{rooFixtureDir}, NewState())
	if len(a) != len(b) {
		t.Fatalf("len mismatch %d vs %d", len(a), len(b))
	}
}
