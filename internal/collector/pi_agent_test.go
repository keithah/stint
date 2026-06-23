package collector

import (
	"testing"

	"github.com/keithah/stint/internal/usage"
)

const piAgentFixtureDir = "testdata/pi-agent"

func TestPiAgentScan(t *testing.T) {
	events, report, err := scanPiAgent([]string{piAgentFixtureDir}, NewState())
	if err != nil {
		t.Fatalf("scanPiAgent: %v", err)
	}

	// Fixture: 1 user (non-usage), 2 usage rows for the SAME message (dup),
	// 1 distinct usage row, 1 malformed.
	if report.LinesParsed != 5 {
		t.Errorf("LinesParsed = %d, want 5", report.LinesParsed)
	}
	if report.EventsEmitted != 3 {
		t.Errorf("EventsEmitted = %d, want 3", report.EventsEmitted)
	}
	if report.Errors != 1 {
		t.Errorf("Errors = %d, want 1", report.Errors)
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

	e1 := byMsg["pmsg_001"]
	// input inclusive of cache_read => 600 - 200 = 400; cache_creation lumped to 5m.
	if e1.InputTokens != 400 || e1.OutputTokens != 150 || e1.CacheReadTokens != 200 ||
		e1.CacheCreate5mTokens != 400 || e1.ReasoningTokens != 75 {
		t.Errorf("pmsg_001 tokens wrong: %+v", e1)
	}
	if e1.Agent != "pi-agent" {
		t.Errorf("agent = %q, want pi-agent", e1.Agent)
	}
	if e1.Project != "myproj" {
		t.Errorf("pmsg_001 Project = %q, want myproj", e1.Project)
	}

	e2 := byMsg["pmsg_002"]
	// synonym fields: input 300 - cached 50 = 250; thoughts => reasoning.
	if e2.InputTokens != 250 || e2.OutputTokens != 90 || e2.CacheReadTokens != 50 || e2.ReasoningTokens != 20 {
		t.Errorf("pmsg_002 tokens wrong: %+v", e2)
	}

	var sumIn, sumOut, sumRead, sum5m, sumReason int
	for _, e := range events {
		sumIn += e.InputTokens
		sumOut += e.OutputTokens
		sumRead += e.CacheReadTokens
		sum5m += e.CacheCreate5mTokens
		sumReason += e.ReasoningTokens
	}
	if sumIn != 650 || sumOut != 240 || sumRead != 250 || sum5m != 400 || sumReason != 95 {
		t.Errorf("sums wrong: in=%d out=%d read=%d 5m=%d reason=%d", sumIn, sumOut, sumRead, sum5m, sumReason)
	}
}

func TestPiAgentScanDeterministic(t *testing.T) {
	a, _, _ := scanPiAgent([]string{piAgentFixtureDir}, NewState())
	b, _, _ := scanPiAgent([]string{piAgentFixtureDir}, NewState())
	if len(a) != len(b) {
		t.Fatalf("len mismatch %d vs %d", len(a), len(b))
	}
}
