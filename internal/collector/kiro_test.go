package collector

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// buildKiroFixtureDB creates a temp SQLite DB matching Kiro CLI's documented
// data.sqlite3 messages shape. Rows: two usage rows, an exact duplicate of the
// first (dedup), and a no-usage row.
func buildKiroFixtureDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "data.sqlite3")

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
    created_at    TEXT
);`); err != nil {
		t.Fatalf("create: %v", err)
	}

	insert := `INSERT INTO messages (session_id, role, model, input_tokens, output_tokens, created_at) VALUES (?, ?, ?, ?, ?, ?)`
	if _, err := db.Exec(insert, "db-sess", "assistant", "gpt-4o", 500, 200, "2026-06-22T11:00:00Z"); err != nil {
		t.Fatalf("row1: %v", err)
	}
	if _, err := db.Exec(insert, "db-sess", "assistant", "claude-3-5-sonnet", 300, 90, "2026-06-22T11:05:00Z"); err != nil {
		t.Fatalf("row2: %v", err)
	}
	if _, err := db.Exec(insert, "db-sess", "assistant", "gpt-4o", 500, 200, "2026-06-22T11:00:00Z"); err != nil {
		t.Fatalf("row3 dup: %v", err)
	}
	if _, err := db.Exec(insert, "db-sess", "user", "", 0, 0, "2026-06-22T11:01:00Z"); err != nil {
		t.Fatalf("row4: %v", err)
	}

	return path
}

func TestScanKiroBothSourcesDedupAndIncremental(t *testing.T) {
	jsonlBase, err := filepath.Abs("testdata/kiro/sessions/cli")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if _, err := os.Stat(jsonlBase); err != nil {
		t.Fatalf("jsonl fixture missing: %v", err)
	}
	dbPath := buildKiroFixtureDB(t)
	dbBase := filepath.Dir(dbPath)
	state := NewState()

	events, report, err := scanKiro([]string{jsonlBase, dbBase}, state)
	if err != nil {
		t.Fatalf("scanKiro: %v", err)
	}

	// JSONL: msg-1 (dup -> 1) + msg-2 = 2 events. SQLite: 2 unique usage rows.
	if len(events) != 4 {
		t.Fatalf("got %d deduped events, want 4: %+v", len(events), events)
	}

	var totalIn, totalOut, totalCacheRead, totalCacheCreate int
	for _, e := range events {
		if e.Agent != "kiro" {
			t.Errorf("Agent = %q, want kiro", e.Agent)
		}
		if e.BillingType != "api" {
			t.Errorf("BillingType = %q, want api", e.BillingType)
		}
		totalIn += e.InputTokens
		totalOut += e.OutputTokens
		totalCacheRead += e.CacheReadTokens
		totalCacheCreate += e.CacheCreate5mTokens
	}

	// JSONL msg-1: input 150-50=100, out 60, cache-read 50, cache-create 20.
	// JSONL msg-2: input 80, out 40.
	// DB row1: in 500, out 200.  DB row2: in 300, out 90.
	wantIn := 100 + 80 + 500 + 300
	wantOut := 60 + 40 + 200 + 90
	if totalIn != wantIn {
		t.Errorf("input sum = %d, want %d", totalIn, wantIn)
	}
	if totalOut != wantOut {
		t.Errorf("output sum = %d, want %d", totalOut, wantOut)
	}
	if totalCacheRead != 50 {
		t.Errorf("cache-read sum = %d, want 50", totalCacheRead)
	}
	if totalCacheCreate != 20 {
		t.Errorf("cache-create sum = %d, want 20", totalCacheCreate)
	}

	// EventsEmitted counts per-row emissions before usage.Dedup collapses the
	// duplicate JSONL line, so it is >= len(events).
	if report.EventsEmitted < 4 {
		t.Errorf("EventsEmitted = %d, want >= 4", report.EventsEmitted)
	}

	// Rescan: incremental cursors (byte offset for JSONL, max rowid for SQLite)
	// plus dedup must emit nothing new.
	events2, report2, err := scanKiro([]string{jsonlBase, dbBase}, state)
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

func TestScanKiroMissingDirNoError(t *testing.T) {
	events, report, err := scanKiro([]string{filepath.Join(t.TempDir(), "nope")}, NewState())
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
