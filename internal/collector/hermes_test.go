package collector

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// buildHermesFixtureDB creates a temp SQLite DB with a representative Hermes
// messages schema (dedicated token columns + a JSON usage blob). Rows:
//   - an assistant message with dedicated input/output and cache-read via JSON,
//   - another usage row with all counts from columns,
//   - an exact logical duplicate of the first (different rowid) for dedup,
//   - a no-usage row (zero/NULL tokens) to exercise the skip path.
func buildHermesFixtureDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
CREATE TABLE messages (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id    TEXT,
    role          TEXT,
    model         TEXT,
    input_tokens  INTEGER,
    output_tokens INTEGER,
    created_at    TEXT,
    usage         TEXT
);`); err != nil {
		t.Fatalf("create: %v", err)
	}

	insert := `INSERT INTO messages
        (session_id, role, model, input_tokens, output_tokens, created_at, usage)
        VALUES (?, ?, ?, ?, ?, ?, ?)`

	if _, err := db.Exec(insert, "sess-1", "assistant", "gpt-4o", 100, 50,
		"2026-06-22T10:00:00Z", `{"cache_read_tokens":30}`); err != nil {
		t.Fatalf("row1: %v", err)
	}
	if _, err := db.Exec(insert, "sess-1", "assistant", "claude-3-5-sonnet", 200, 80,
		"2026-06-22T10:05:00Z", ""); err != nil {
		t.Fatalf("row2: %v", err)
	}
	if _, err := db.Exec(insert, "sess-1", "assistant", "gpt-4o", 100, 50,
		"2026-06-22T10:00:00Z", `{"cache_read_tokens":30}`); err != nil {
		t.Fatalf("row3: %v", err)
	}
	if _, err := db.Exec(insert, "sess-1", "user", "", 0, 0,
		"2026-06-22T10:01:00Z", ""); err != nil {
		t.Fatalf("row4: %v", err)
	}

	return path
}

func TestScanHermesTokensDedupAndIncremental(t *testing.T) {
	path := buildHermesFixtureDB(t)
	base := filepath.Dir(path)
	state := NewState()

	events, report, err := scanHermes([]string{base}, state)
	if err != nil {
		t.Fatalf("scanHermes: %v", err)
	}
	if report.FilesScanned != 1 {
		t.Errorf("FilesScanned = %d, want 1", report.FilesScanned)
	}
	if report.LinesParsed != 4 {
		t.Errorf("LinesParsed = %d, want 4", report.LinesParsed)
	}
	if len(events) != 2 {
		t.Fatalf("got %d deduped events, want 2: %+v", len(events), events)
	}

	var totalIn, totalOut, totalCache int
	for _, e := range events {
		if e.Agent != "hermes" {
			t.Errorf("Agent = %q, want hermes", e.Agent)
		}
		if e.BillingType != "api" {
			t.Errorf("BillingType = %q, want api", e.BillingType)
		}
		if e.EventID == "" {
			t.Error("EventID not set")
		}
		totalIn += e.InputTokens
		totalOut += e.OutputTokens
		totalCache += e.CacheReadTokens
	}
	if totalIn != 300 {
		t.Errorf("input sum = %d, want 300", totalIn)
	}
	if totalOut != 130 {
		t.Errorf("output sum = %d, want 130", totalOut)
	}
	if totalCache != 30 {
		t.Errorf("cache-read sum = %d, want 30 (from JSON blob)", totalCache)
	}

	events2, report2, err := scanHermes([]string{base}, state)
	if err != nil {
		t.Fatalf("rescan: %v", err)
	}
	if len(events2) != 0 {
		t.Errorf("rescan emitted %d events, want 0", len(events2))
	}
	if report2.EventsEmitted != 0 {
		t.Errorf("rescan EventsEmitted = %d, want 0", report2.EventsEmitted)
	}
	if report2.LinesParsed != 0 {
		t.Errorf("rescan LinesParsed = %d, want 0 (cursor past max rowid)", report2.LinesParsed)
	}
}

func TestScanHermesMissingDirNoError(t *testing.T) {
	events, report, err := scanHermes([]string{filepath.Join(t.TempDir(), "nope")}, NewState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("got %d events, want 0", len(events))
	}
	if report.Errors != 0 {
		t.Errorf("Errors = %d, want 0 for missing dir", report.Errors)
	}
}
