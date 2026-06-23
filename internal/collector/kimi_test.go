package collector

import (
	"testing"

	"github.com/keithah/stint/internal/usage"
)

const kimiTestDir = "testdata/kimi"

func TestKimiScanSums(t *testing.T) {
	events, report, err := scanKimi([]string{kimiTestDir}, NewState())
	if err != nil {
		t.Fatalf("scanKimi error: %v", err)
	}

	if report.FilesScanned != 1 {
		t.Errorf("FilesScanned = %d, want 1", report.FilesScanned)
	}
	// The malformed final line must be counted, not abort the scan.
	if report.Errors != 1 {
		t.Errorf("Errors = %d, want 1 (malformed line)", report.Errors)
	}

	// kmsg-1 (duplicate) + kmsg-2 => 2 deduped usage events.
	if len(events) != 2 {
		t.Fatalf("got %d deduped events, want 2: %+v", len(events), events)
	}

	var in, out, read int
	for _, e := range events {
		if e.Agent != "kimi" {
			t.Errorf("Agent = %q, want kimi", e.Agent)
		}
		if e.BillingType != usage.BillingAPI {
			t.Errorf("BillingType = %q, want api", e.BillingType)
		}
		if e.SessionID != "k-sess-1" {
			t.Errorf("SessionID = %q, want k-sess-1", e.SessionID)
		}
		if e.Project != "proj-a" {
			t.Errorf("Project = %q, want proj-a (cwd basename)", e.Project)
		}
		in += e.InputTokens
		out += e.OutputTokens
		read += e.CacheReadTokens
	}

	// prompt - cached: (1000-400) + 500 = 1100
	if want := 1100; in != want {
		t.Errorf("InputTokens sum = %d, want %d", in, want)
	}
	// completion: 250 + 120
	if want := 370; out != want {
		t.Errorf("OutputTokens sum = %d, want %d", out, want)
	}
	// cached -> CacheRead: 400 + 0
	if want := 400; read != want {
		t.Errorf("CacheReadTokens sum = %d, want %d", read, want)
	}
}

func TestKimiDeterministic(t *testing.T) {
	a, _, err := scanKimi([]string{kimiTestDir}, NewState())
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := scanKimi([]string{kimiTestDir}, NewState())
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != len(b) {
		t.Fatalf("len mismatch %d vs %d", len(a), len(b))
	}
	ids := map[string]int{}
	for _, e := range a {
		ids[e.EventID]++
	}
	for _, e := range b {
		ids[e.EventID]--
	}
	for id, n := range ids {
		if n != 0 {
			t.Errorf("event id %s differs between scans (delta %d)", id, n)
		}
	}
}
