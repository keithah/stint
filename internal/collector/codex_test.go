package collector

import (
	"testing"

	"github.com/keithah/stint/internal/usage"
)

const codexFixtureDir = "testdata/codex"

func TestCodexScan(t *testing.T) {
	events, report, err := scanCodex([]string{codexFixtureDir}, NewState())
	if err != nil {
		t.Fatalf("scanCodex: %v", err)
	}

	// Fixture has 9 lines: session_meta, turn_context, agent_message,
	// token_count(A), token_count(B), token_count(dup of B),
	// token_count(info:null), reasoning, malformed.
	if report.FilesScanned != 1 {
		t.Errorf("FilesScanned = %d, want 1", report.FilesScanned)
	}
	if report.LinesParsed != 9 {
		t.Errorf("LinesParsed = %d, want 9", report.LinesParsed)
	}
	// 3 token_count lines carry usage and emit events (pre-dedup).
	if report.EventsEmitted != 3 {
		t.Errorf("EventsEmitted = %d, want 3", report.EventsEmitted)
	}
	// Only the malformed JSON line is an error.
	if report.Errors != 1 {
		t.Errorf("Errors = %d, want 1 (malformed line)", report.Errors)
	}
	// Skipped: session_meta, turn_context, agent_message, null-info token_count,
	// reasoning, malformed = 6.
	if report.LinesSkipped != 6 {
		t.Errorf("LinesSkipped = %d, want 6", report.LinesSkipped)
	}

	// After dedup the two identical event B lines collapse to one => 2 events.
	if len(events) != 2 {
		t.Fatalf("deduped events = %d, want 2", len(events))
	}

	// Identify events by their distinctive output token counts.
	var evA, evB usage.Event
	var gotA, gotB bool
	for _, e := range events {
		switch e.OutputTokens {
		case 300:
			evA, gotA = e, true
		case 500:
			evB, gotB = e, true
		}
	}
	if !gotA || !gotB {
		t.Fatalf("missing expected events: gotA=%v gotB=%v (%+v)", gotA, gotB, events)
	}

	// Event A: input_tokens 1000 total includes 200 cached => InputTokens 800,
	// CacheReadTokens 200, no reasoning.
	if evA.InputTokens != 800 {
		t.Errorf("A InputTokens = %d, want 800 (1000-200 cached)", evA.InputTokens)
	}
	if evA.CacheReadTokens != 200 {
		t.Errorf("A CacheReadTokens = %d, want 200", evA.CacheReadTokens)
	}
	if evA.ReasoningTokens != 0 {
		t.Errorf("A ReasoningTokens = %d, want 0", evA.ReasoningTokens)
	}

	// Event B: from last_token_usage (delta, not the cumulative total): input
	// 5000 includes 1500 cached => InputTokens 3500, CacheReadTokens 1500,
	// ReasoningTokens 250.
	if evB.InputTokens != 3500 {
		t.Errorf("B InputTokens = %d, want 3500 (5000-1500 cached)", evB.InputTokens)
	}
	if evB.CacheReadTokens != 1500 {
		t.Errorf("B CacheReadTokens = %d, want 1500", evB.CacheReadTokens)
	}
	if evB.OutputTokens != 500 {
		t.Errorf("B OutputTokens = %d, want 500", evB.OutputTokens)
	}
	if evB.ReasoningTokens != 250 {
		t.Errorf("B ReasoningTokens = %d, want 250", evB.ReasoningTokens)
	}

	// Common attribution checks.
	for _, e := range events {
		if e.Agent != "codex" {
			t.Errorf("Agent = %q, want codex", e.Agent)
		}
		if e.BillingType != usage.BillingAPI {
			t.Errorf("BillingType = %q, want api", e.BillingType)
		}
		if e.Model != "gpt-5.3-codex" {
			t.Errorf("Model = %q, want gpt-5.3-codex (from turn_context)", e.Model)
		}
		if e.Project != "proj" {
			t.Errorf("Project = %q, want proj (basename of cwd)", e.Project)
		}
		if e.SessionID != "019ee626-b3e8-7e01-bb4a-ef897be7f2fa" {
			t.Errorf("SessionID = %q, want UUID from filename", e.SessionID)
		}
	}

	// Aggregate token sums across the deduped set.
	var totalInput, totalCacheRead, totalReasoning, totalOutput int
	for _, e := range events {
		totalInput += e.InputTokens
		totalCacheRead += e.CacheReadTokens
		totalReasoning += e.ReasoningTokens
		totalOutput += e.OutputTokens
	}
	if totalInput != 4300 { // 800 + 3500
		t.Errorf("total InputTokens = %d, want 4300", totalInput)
	}
	if totalCacheRead != 1700 { // 200 + 1500
		t.Errorf("total CacheReadTokens = %d, want 1700", totalCacheRead)
	}
	if totalReasoning != 250 {
		t.Errorf("total ReasoningTokens = %d, want 250", totalReasoning)
	}
	if totalOutput != 800 { // 300 + 500
		t.Errorf("total OutputTokens = %d, want 800", totalOutput)
	}
}
