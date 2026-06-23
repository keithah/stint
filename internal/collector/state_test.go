package collector

import (
	"path/filepath"
	"testing"
)

// TestIncrementalRescan proves that scanning the fixture, saving state, and
// re-scanning with that state yields zero new events the second time.
func TestIncrementalRescan(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "collector-state.json")

	state, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	first, _, err := scanClaude([]string{claudeFixtureDir}, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 {
		t.Fatal("first scan emitted no events")
	}
	if err := state.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Re-load from disk to ensure persistence round-trips.
	reloaded, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	second, report, err := scanClaude([]string{claudeFixtureDir}, reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Errorf("re-scan emitted %d events, want 0", len(second))
	}
	if report.LinesParsed != 0 {
		t.Errorf("re-scan parsed %d lines, want 0 (all consumed)", report.LinesParsed)
	}
	if report.EventsEmitted != 0 {
		t.Errorf("re-scan EventsEmitted = %d, want 0", report.EventsEmitted)
	}
}

func TestStateSaveLoadRoundTrip(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "s.json")
	s, err := LoadState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	s.commit("/x/y.jsonl", 100, 42, 100, 5)
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s2, err := LoadState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	fs, ok := s2.get("/x/y.jsonl")
	if !ok {
		t.Fatal("file state not persisted")
	}
	if fs.Offset != 100 || fs.Lines != 5 || fs.Size != 100 {
		t.Errorf("round-trip mismatch: %+v", fs)
	}
}
