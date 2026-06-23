package collector

import (
	"testing"

	"github.com/keithah/stint/internal/usage"
)

const factoryDroidTestDir = "testdata/factory-droid"

func TestFactoryDroidScanSums(t *testing.T) {
	events, report, err := scanFactoryDroid([]string{factoryDroidTestDir}, NewState())
	if err != nil {
		t.Fatalf("scanFactoryDroid error: %v", err)
	}

	if report.FilesScanned != 1 {
		t.Errorf("FilesScanned = %d, want 1", report.FilesScanned)
	}
	// The malformed final line must be counted, not abort the scan.
	if report.Errors != 1 {
		t.Errorf("Errors = %d, want 1 (malformed line)", report.Errors)
	}

	// fdmsg-1 (duplicate) + fdmsg-2 => 2 deduped usage events. The tool line
	// (fdmsg-3, no usage) is skipped.
	if len(events) != 2 {
		t.Fatalf("got %d deduped events, want 2: %+v", len(events), events)
	}

	var in, out, c5m, read, reasoning int
	for _, e := range events {
		if e.Agent != "factory-droid" {
			t.Errorf("Agent = %q, want factory-droid", e.Agent)
		}
		if e.BillingType != usage.BillingAPI {
			t.Errorf("BillingType = %q, want api", e.BillingType)
		}
		if e.SessionID != "fd-sess-1" {
			t.Errorf("SessionID = %q, want fd-sess-1", e.SessionID)
		}
		if e.Project != "proj-x" {
			t.Errorf("Project = %q, want proj-x (cwd basename)", e.Project)
		}
		in += e.InputTokens
		out += e.OutputTokens
		c5m += e.CacheCreate5mTokens
		read += e.CacheReadTokens
		reasoning += e.ReasoningTokens
	}

	if want := 400; in != want {
		t.Errorf("InputTokens sum = %d, want %d", in, want)
	}
	if want := 540; out != want {
		t.Errorf("OutputTokens sum = %d, want %d", out, want)
	}
	if want := 700; c5m != want {
		t.Errorf("CacheCreate5mTokens sum = %d, want %d", c5m, want)
	}
	if want := 2000; read != want {
		t.Errorf("CacheReadTokens sum = %d, want %d", read, want)
	}
	if want := 60; reasoning != want {
		t.Errorf("ReasoningTokens sum = %d, want %d", reasoning, want)
	}
}

func TestFactoryDroidDeterministic(t *testing.T) {
	a, _, err := scanFactoryDroid([]string{factoryDroidTestDir}, NewState())
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := scanFactoryDroid([]string{factoryDroidTestDir}, NewState())
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
