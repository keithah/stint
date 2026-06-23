package collector

import (
	"testing"

	"github.com/keithah/stint/internal/usage"
)

const clineFixtureDir = "testdata/cline"

func TestClineScan(t *testing.T) {
	events, report, err := scanCline([]string{clineFixtureDir}, NewState())
	if err != nil {
		t.Fatalf("scanCline: %v", err)
	}

	// Fixture: 1 say/text (non-usage), 2 assistant usage rows for the SAME
	// message (dup), 1 api_req_started metadata usage row, 1 malformed.
	if report.LinesParsed != 5 {
		t.Errorf("LinesParsed = %d, want 5", report.LinesParsed)
	}
	if report.EventsEmitted != 3 {
		t.Errorf("EventsEmitted = %d, want 3", report.EventsEmitted)
	}
	if report.Errors != 1 {
		t.Errorf("Errors = %d, want 1", report.Errors)
	}
	if report.LinesSkipped != 2 {
		t.Errorf("LinesSkipped = %d, want 2", report.LinesSkipped)
	}

	if len(events) != 2 {
		t.Fatalf("deduped events = %d, want 2", len(events))
	}

	var msg, apiReq *usage.Event
	for i := range events {
		if events[i].MessageID == "cmsg_001" {
			msg = &events[i]
		} else {
			apiReq = &events[i]
		}
	}
	if msg == nil || apiReq == nil {
		t.Fatalf("expected one cmsg_001 and one metadata event, got %+v", events)
	}
	if msg.Agent != "cline" {
		t.Errorf("agent = %q, want cline", msg.Agent)
	}
	// No explicit split => lumped cache_creation_input_tokens into 5m.
	if msg.InputTokens != 120 || msg.OutputTokens != 90 ||
		msg.CacheCreate5mTokens != 800 || msg.CacheCreate1hTokens != 0 ||
		msg.CacheReadTokens != 3000 {
		t.Errorf("cmsg_001 tokens wrong: %+v", *msg)
	}
	if apiReq.InputTokens != 70 || apiReq.OutputTokens != 30 ||
		apiReq.CacheCreate5mTokens != 200 || apiReq.CacheReadTokens != 1500 {
		t.Errorf("api_req tokens wrong: %+v", *apiReq)
	}
	if apiReq.CostUSDProvided == nil || *apiReq.CostUSDProvided != 0.0045 {
		t.Errorf("api_req cost = %v, want 0.0045", apiReq.CostUSDProvided)
	}
}

func TestClineScanDeterministic(t *testing.T) {
	a, _, _ := scanCline([]string{clineFixtureDir}, NewState())
	b, _, _ := scanCline([]string{clineFixtureDir}, NewState())
	if len(a) != len(b) {
		t.Fatalf("len mismatch %d vs %d", len(a), len(b))
	}
}
