package collector

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// buildKiloFixtureDB creates a temp SQLite DB matching Kilo's documented
// kilo.db messages shape. Rows: two usage rows, an exact duplicate of the
// first (dedup), and a no-usage row.
func buildKiloFixtureDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kilo.db")

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
CREATE TABLE messages (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id            TEXT,
    role               TEXT,
    model              TEXT,
    input_tokens       INTEGER,
    output_tokens      INTEGER,
    cache_read_tokens  INTEGER,
    cache_creation_tokens INTEGER,
    created_at         TEXT
);`); err != nil {
		t.Fatalf("create: %v", err)
	}

	insert := `INSERT INTO messages (task_id, role, model, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := db.Exec(insert, "db-task", "assistant", "gpt-4o", 400, 150, 25, 10, "2026-06-22T12:00:00Z"); err != nil {
		t.Fatalf("row1: %v", err)
	}
	if _, err := db.Exec(insert, "db-task", "assistant", "claude-3-5-sonnet", 220, 70, 0, 0, "2026-06-22T12:05:00Z"); err != nil {
		t.Fatalf("row2: %v", err)
	}
	if _, err := db.Exec(insert, "db-task", "assistant", "gpt-4o", 400, 150, 25, 10, "2026-06-22T12:00:00Z"); err != nil {
		t.Fatalf("row3 dup: %v", err)
	}
	if _, err := db.Exec(insert, "db-task", "user", "", 0, 0, 0, 0, "2026-06-22T12:01:00Z"); err != nil {
		t.Fatalf("row4: %v", err)
	}

	return path
}

func TestScanKiloBothSourcesDedupAndIncremental(t *testing.T) {
	jsonlBase, err := filepath.Abs("testdata/kilo/globalStorage/kilocode.kilo-code/tasks")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if _, err := os.Stat(jsonlBase); err != nil {
		t.Fatalf("jsonl fixture missing: %v", err)
	}
	dbPath := buildKiloFixtureDB(t)
	dbBase := filepath.Dir(dbPath)
	state := NewState()

	events, report, err := scanKilo([]string{dbBase, jsonlBase}, state)
	if err != nil {
		t.Fatalf("scanKilo: %v", err)
	}

	// SQLite: 2 unique usage rows. JSONL: msg-1 (dup -> 1) + msg-2 = 2 events.
	if len(events) != 4 {
		t.Fatalf("got %d deduped events, want 4: %+v", len(events), events)
	}

	var totalIn, totalOut, totalCacheRead, totalCacheCreate int
	for _, e := range events {
		if e.Agent != "kilo" {
			t.Errorf("Agent = %q, want kilo", e.Agent)
		}
		if e.BillingType != "api" {
			t.Errorf("BillingType = %q, want api", e.BillingType)
		}
		if e.Timestamp == "" {
			t.Error("Timestamp not set")
		}
		totalIn += e.InputTokens
		totalOut += e.OutputTokens
		totalCacheRead += e.CacheReadTokens
		totalCacheCreate += e.CacheCreate5mTokens
	}

	// DB row1: in 400, out 150, cache-read 25, cache-create 10.
	// DB row2: in 220, out 70.
	// JSONL msg-1: input 300-100=200, out 120, cache-read 100, cache-create 40.
	// JSONL msg-2: input 90, out 30.
	wantIn := 400 + 220 + 200 + 90
	wantOut := 150 + 70 + 120 + 30
	if totalIn != wantIn {
		t.Errorf("input sum = %d, want %d", totalIn, wantIn)
	}
	if totalOut != wantOut {
		t.Errorf("output sum = %d, want %d", totalOut, wantOut)
	}
	if totalCacheRead != 25+100 {
		t.Errorf("cache-read sum = %d, want %d", totalCacheRead, 25+100)
	}
	if totalCacheCreate != 10+40 {
		t.Errorf("cache-create sum = %d, want %d", totalCacheCreate, 10+40)
	}

	// EventsEmitted counts per-row emissions before usage.Dedup collapses the
	// duplicate JSONL line, so it is >= len(events).
	if report.EventsEmitted < 4 {
		t.Errorf("EventsEmitted = %d, want >= 4", report.EventsEmitted)
	}

	events2, report2, err := scanKilo([]string{dbBase, jsonlBase}, state)
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

func TestScanKiloMissingDirNoError(t *testing.T) {
	events, report, err := scanKilo([]string{filepath.Join(t.TempDir(), "nope")}, NewState())
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
