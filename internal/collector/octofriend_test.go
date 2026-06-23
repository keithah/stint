package collector

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// buildOctofriendFixtureDB creates a temp SQLite DB with a representative
// Octofriend messages schema using dedicated token columns (including a
// reasoning column). Rows: two usage rows, an exact duplicate of the first, and
// a no-usage row.
func buildOctofriendFixtureDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sqlite.db")

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
CREATE TABLE messages (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id  TEXT,
    role             TEXT,
    model            TEXT,
    input_tokens     INTEGER,
    output_tokens    INTEGER,
    cache_read_tokens INTEGER,
    reasoning_tokens INTEGER,
    timestamp        TEXT
);`); err != nil {
		t.Fatalf("create: %v", err)
	}

	insert := `INSERT INTO messages
        (conversation_id, role, model, input_tokens, output_tokens, cache_read_tokens, reasoning_tokens, timestamp)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	if _, err := db.Exec(insert, "c-1", "assistant", "gpt-4o", 100, 50, 30, 10,
		"2026-06-22T10:00:00Z"); err != nil {
		t.Fatalf("row1: %v", err)
	}
	if _, err := db.Exec(insert, "c-1", "assistant", "claude-3-5-sonnet", 200, 80, 0, 0,
		"2026-06-22T10:05:00Z"); err != nil {
		t.Fatalf("row2: %v", err)
	}
	if _, err := db.Exec(insert, "c-1", "assistant", "gpt-4o", 100, 50, 30, 10,
		"2026-06-22T10:00:00Z"); err != nil {
		t.Fatalf("row3: %v", err)
	}
	if _, err := db.Exec(insert, "c-1", "user", "", 0, 0, 0, 0,
		"2026-06-22T10:01:00Z"); err != nil {
		t.Fatalf("row4: %v", err)
	}

	return path
}

func TestScanOctofriendTokensDedupAndIncremental(t *testing.T) {
	path := buildOctofriendFixtureDB(t)
	base := filepath.Dir(path)
	state := NewState()

	events, report, err := scanOctofriend([]string{base}, state)
	if err != nil {
		t.Fatalf("scanOctofriend: %v", err)
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

	var totalIn, totalOut, totalCache, totalReason int
	for _, e := range events {
		if e.Agent != "octofriend" {
			t.Errorf("Agent = %q, want octofriend", e.Agent)
		}
		if e.BillingType != "api" {
			t.Errorf("BillingType = %q, want api", e.BillingType)
		}
		if e.SessionID != "c-1" {
			t.Errorf("SessionID = %q, want c-1", e.SessionID)
		}
		totalIn += e.InputTokens
		totalOut += e.OutputTokens
		totalCache += e.CacheReadTokens
		totalReason += e.ReasoningTokens
	}
	if totalIn != 300 {
		t.Errorf("input sum = %d, want 300", totalIn)
	}
	if totalOut != 130 {
		t.Errorf("output sum = %d, want 130", totalOut)
	}
	if totalCache != 30 {
		t.Errorf("cache-read sum = %d, want 30", totalCache)
	}
	if totalReason != 10 {
		t.Errorf("reasoning sum = %d, want 10", totalReason)
	}

	events2, report2, err := scanOctofriend([]string{base}, state)
	if err != nil {
		t.Fatalf("rescan: %v", err)
	}
	if len(events2) != 0 {
		t.Errorf("rescan emitted %d events, want 0", len(events2))
	}
	if report2.EventsEmitted != 0 {
		t.Errorf("rescan EventsEmitted = %d, want 0", report2.EventsEmitted)
	}
}

func TestScanOctofriendMissingDirNoError(t *testing.T) {
	events, report, err := scanOctofriend([]string{filepath.Join(t.TempDir(), "nope")}, NewState())
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
