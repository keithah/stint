package collector

import (
	"testing"

	"github.com/keithah/stint/internal/usage"
)

const crushTestDir = "testdata/crush"

func TestCrushScanSums(t *testing.T) {
	events, report, err := scanCrush([]string{crushTestDir}, NewState())
	if err != nil {
		t.Fatalf("scanCrush error: %v", err)
	}

	// Three files: session-1.json (usage), projects.json (non-usage index),
	// broken.json (malformed).
	if report.FilesScanned != 3 {
		t.Errorf("FilesScanned = %d, want 3", report.FilesScanned)
	}
	if report.Errors == 0 {
		t.Errorf("Errors = 0, want >0 (malformed file should be counted)")
	}

	// cm-1 (duplicate) + cm-2 => 2 deduped usage events. projects.json has no
	// messages array and contributes nothing.
	if len(events) != 2 {
		t.Fatalf("got %d deduped events, want 2: %+v", len(events), events)
	}

	var in, out, c5m, read, reasoning int
	for _, e := range events {
		if e.Agent != "crush" {
			t.Errorf("Agent = %q, want crush", e.Agent)
		}
		if e.BillingType != usage.BillingAPI {
			t.Errorf("BillingType = %q, want api", e.BillingType)
		}
		if e.SessionID != "sess-crush-1" {
			t.Errorf("SessionID = %q, want sess-crush-1", e.SessionID)
		}
		if e.Project != "gadget" {
			t.Errorf("Project = %q, want gadget (cwd basename)", e.Project)
		}
		in += e.InputTokens
		out += e.OutputTokens
		c5m += e.CacheCreate5mTokens
		read += e.CacheReadTokens
		reasoning += e.ReasoningTokens
	}

	if want := 290; in != want {
		t.Errorf("InputTokens sum = %d, want %d", in, want)
	}
	if want := 510; out != want {
		t.Errorf("OutputTokens sum = %d, want %d", out, want)
	}
	if want := 600; c5m != want {
		t.Errorf("CacheCreate5mTokens sum = %d, want %d", c5m, want)
	}
	if want := 1800; read != want {
		t.Errorf("CacheReadTokens sum = %d, want %d", read, want)
	}
	if want := 50; reasoning != want {
		t.Errorf("ReasoningTokens sum = %d, want %d", reasoning, want)
	}
}

func TestCrushRescanEmitsNothing(t *testing.T) {
	st := NewState()
	first, _, err := scanCrush([]string{crushTestDir}, st)
	if err != nil {
		t.Fatalf("first scan error: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("first scan got %d events, want 2", len(first))
	}

	second, report, err := scanCrush([]string{crushTestDir}, st)
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

func TestCrushDedupIdempotent(t *testing.T) {
	a, _, _ := scanCrush([]string{crushTestDir}, NewState())
	b, _, _ := scanCrush([]string{crushTestDir}, NewState())
	combined := usage.Dedup(append(append([]usage.Event{}, a...), b...))
	if len(combined) != len(a) {
		t.Errorf("combined dedup = %d events, want %d (idempotent)", len(combined), len(a))
	}
}
