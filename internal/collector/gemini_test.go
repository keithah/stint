package collector

import (
	"testing"

	"github.com/keithah/stint/internal/usage"
)

const geminiTestDir = "testdata/gemini"

func TestGeminiScanSums(t *testing.T) {
	events, report, err := scanGemini([]string{geminiTestDir}, NewState())
	if err != nil {
		t.Fatalf("scanGemini error: %v", err)
	}

	// Two session files: the valid one and the malformed one.
	if report.FilesScanned != 2 {
		t.Errorf("FilesScanned = %d, want 2", report.FilesScanned)
	}
	// The malformed file must be counted, not abort the scan.
	if report.Errors == 0 {
		t.Errorf("Errors = 0, want >0 (malformed file should be counted)")
	}

	// After dedup the duplicated gemini message collapses: 2 usage events
	// (gemini-2.5-flash + gemini-2.5-pro).
	if len(events) != 2 {
		t.Fatalf("got %d deduped events, want 2: %+v", len(events), events)
	}

	var in, out, cacheRead, reasoning int
	for _, e := range events {
		if e.Agent != "gemini" {
			t.Errorf("Agent = %q, want gemini", e.Agent)
		}
		if e.BillingType != usage.BillingAPI {
			t.Errorf("BillingType = %q, want api", e.BillingType)
		}
		if e.SessionID == "" {
			t.Errorf("empty SessionID on %+v", e)
		}
		in += e.InputTokens
		out += e.OutputTokens
		cacheRead += e.CacheReadTokens
		reasoning += e.ReasoningTokens
	}

	// input is inclusive of cached; InputTokens = input - cached.
	// flash: 6773-4925=1848 ; pro: 8000-3000=5000 => 6848
	if want := 6848; in != want {
		t.Errorf("InputTokens sum = %d, want %d", in, want)
	}
	// output: 11 + 250
	if want := 261; out != want {
		t.Errorf("OutputTokens sum = %d, want %d", out, want)
	}
	// cached -> CacheReadTokens: 4925 + 3000
	if want := 7925; cacheRead != want {
		t.Errorf("CacheReadTokens sum = %d, want %d", cacheRead, want)
	}
	// thoughts -> ReasoningTokens: 36 + 120
	if want := 156; reasoning != want {
		t.Errorf("ReasoningTokens sum = %d, want %d", reasoning, want)
	}
}

func TestGeminiProjectFromCwd(t *testing.T) {
	events, _, err := scanGemini([]string{geminiTestDir}, NewState())
	if err != nil {
		t.Fatalf("scanGemini error: %v", err)
	}
	for _, e := range events {
		if e.Project != "widget" {
			t.Errorf("Project = %q, want widget (from cwd basename)", e.Project)
		}
	}
}

func TestGeminiIncrementalSkip(t *testing.T) {
	// First scan consumes the files and records the cursor.
	st := NewState()
	first, _, err := scanGemini([]string{geminiTestDir}, st)
	if err != nil {
		t.Fatalf("first scan error: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("first scan got %d events, want 2", len(first))
	}

	// Second scan with the same (unchanged) files must emit nothing new: the
	// size+mtime cursor matches, so files are skipped entirely.
	second, report, err := scanGemini([]string{geminiTestDir}, st)
	if err != nil {
		t.Fatalf("second scan error: %v", err)
	}
	if len(second) != 0 {
		t.Errorf("second scan emitted %d events, want 0 (should skip unchanged files)", len(second))
	}
	if report.EventsEmitted != 0 {
		t.Errorf("second scan EventsEmitted = %d, want 0", report.EventsEmitted)
	}
}

func TestGeminiDedupAcrossScans(t *testing.T) {
	// Feeding the same data twice with a fresh state each time must not change
	// totals after a combined dedup.
	a, _, _ := scanGemini([]string{geminiTestDir}, NewState())
	b, _, _ := scanGemini([]string{geminiTestDir}, NewState())
	combined := usage.Dedup(append(append([]usage.Event{}, a...), b...))
	if len(combined) != len(a) {
		t.Errorf("combined dedup = %d events, want %d (idempotent)", len(combined), len(a))
	}
}
