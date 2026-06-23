package collector

import (
	"testing"

	"github.com/keithah/stint/internal/usage"
)

const ampTestDir = "testdata/amp"

func TestAmpScanSums(t *testing.T) {
	events, report, err := scanAmp([]string{ampTestDir}, NewState())
	if err != nil {
		t.Fatalf("scanAmp error: %v", err)
	}

	// Two thread files: the valid one and the malformed one.
	if report.FilesScanned != 2 {
		t.Errorf("FilesScanned = %d, want 2", report.FilesScanned)
	}
	// The malformed file must be counted, not abort the scan.
	if report.Errors == 0 {
		t.Errorf("Errors = 0, want >0 (malformed file should be counted)")
	}

	// amsg-1 appears twice (duplicate) + amsg-2 => 2 deduped usage events.
	if len(events) != 2 {
		t.Fatalf("got %d deduped events, want 2: %+v", len(events), events)
	}

	var in, out, c5m, read, reasoning int
	for _, e := range events {
		if e.Agent != "amp" {
			t.Errorf("Agent = %q, want amp", e.Agent)
		}
		if e.BillingType != usage.BillingAPI {
			t.Errorf("BillingType = %q, want api", e.BillingType)
		}
		if e.SessionID != "thread-abc" {
			t.Errorf("SessionID = %q, want thread-abc", e.SessionID)
		}
		if e.Project != "widget" {
			t.Errorf("Project = %q, want widget (cwd basename)", e.Project)
		}
		in += e.InputTokens
		out += e.OutputTokens
		c5m += e.CacheCreate5mTokens
		read += e.CacheReadTokens
		reasoning += e.ReasoningTokens
	}

	if want := 200; in != want {
		t.Errorf("InputTokens sum = %d, want %d", in, want)
	}
	if want := 450; out != want {
		t.Errorf("OutputTokens sum = %d, want %d", out, want)
	}
	if want := 500; c5m != want {
		t.Errorf("CacheCreate5mTokens sum = %d, want %d", c5m, want)
	}
	if want := 3000; read != want {
		t.Errorf("CacheReadTokens sum = %d, want %d", read, want)
	}
	if want := 40; reasoning != want {
		t.Errorf("ReasoningTokens sum = %d, want %d", reasoning, want)
	}
}

func TestAmpRescanEmitsNothing(t *testing.T) {
	st := NewState()
	first, _, err := scanAmp([]string{ampTestDir}, st)
	if err != nil {
		t.Fatalf("first scan error: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("first scan got %d events, want 2", len(first))
	}

	second, report, err := scanAmp([]string{ampTestDir}, st)
	if err != nil {
		t.Fatalf("second scan error: %v", err)
	}
	if len(second) != 0 {
		t.Errorf("second scan emitted %d events, want 0 (skip unchanged files)", len(second))
	}
	if report.EventsEmitted != 0 {
		t.Errorf("second scan EventsEmitted = %d, want 0", report.EventsEmitted)
	}
}

func TestAmpDedupIdempotent(t *testing.T) {
	a, _, _ := scanAmp([]string{ampTestDir}, NewState())
	b, _, _ := scanAmp([]string{ampTestDir}, NewState())
	combined := usage.Dedup(append(append([]usage.Event{}, a...), b...))
	if len(combined) != len(a) {
		t.Errorf("combined dedup = %d events, want %d (idempotent)", len(combined), len(a))
	}
}
