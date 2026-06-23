package collector

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// buildGooseFixtureDB creates a temp SQLite DB with a representative Goose
// messages schema and returns its path. Rows include:
//   - two assistant messages with dedicated token columns (one carrying cached
//     tokens via the metadata JSON blob),
//   - a duplicate of the first usage row (same model/session/timestamp/tokens,
//     different rowid) to exercise eventId dedup,
//   - a no-usage row (user/tool message with all-zero/NULL tokens) to exercise
//     the skip-without-usage path.
func buildGooseFixtureDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.db")

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
CREATE TABLE messages (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id    TEXT,
    role          TEXT,
    model         TEXT,
    input_tokens  INTEGER,
    output_tokens INTEGER,
    created_at    TEXT,
    metadata      TEXT
);`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	insert := `INSERT INTO messages
        (session_id, role, model, input_tokens, output_tokens, created_at, metadata)
        VALUES (?, ?, ?, ?, ?, ?, ?)`

	// Row 1: dedicated input/output cols, cached tokens only in metadata JSON.
	if _, err := db.Exec(insert,
		"sess-1", "assistant", "gpt-4o", 100, 50,
		"2026-06-22T10:00:00Z",
		`{"cache_read_tokens": 30}`,
	); err != nil {
		t.Fatalf("insert row1: %v", err)
	}

	// Row 2: another usage row, all counts from columns; metadata empty.
	if _, err := db.Exec(insert,
		"sess-1", "assistant", "claude-3-5-sonnet", 200, 80,
		"2026-06-22T10:05:00Z",
		"",
	); err != nil {
		t.Fatalf("insert row2: %v", err)
	}

	// Row 3: exact duplicate of row 1's logical content (different rowid).
	// Must collapse to one event via ComputeEventID's hash backstop.
	if _, err := db.Exec(insert,
		"sess-1", "assistant", "gpt-4o", 100, 50,
		"2026-06-22T10:00:00Z",
		`{"cache_read_tokens": 30}`,
	); err != nil {
		t.Fatalf("insert row3: %v", err)
	}

	// Row 4: no-usage row (user message, zero/NULL tokens). Must be skipped.
	if _, err := db.Exec(insert,
		"sess-1", "user", "", 0, 0,
		"2026-06-22T10:01:00Z",
		"",
	); err != nil {
		t.Fatalf("insert row4: %v", err)
	}

	return path
}

func TestScanGooseTokensDedupAndIncremental(t *testing.T) {
	path := buildGooseFixtureDB(t)
	base := filepath.Dir(path)
	state := NewState()

	events, report, err := scanGoose([]string{base}, state)
	if err != nil {
		t.Fatalf("scanGoose: %v", err)
	}

	// Four rows parsed; row4 skipped (no usage); the duplicate is deduped to a
	// single event, so two distinct usage events remain.
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
		if e.Agent != "goose" {
			t.Errorf("Agent = %q, want goose", e.Agent)
		}
		if e.BillingType != "api" {
			t.Errorf("BillingType = %q, want api", e.BillingType)
		}
		if e.SessionID != "sess-1" {
			t.Errorf("SessionID = %q, want sess-1", e.SessionID)
		}
		if e.EventID == "" {
			t.Error("EventID not set")
		}
		if e.Timestamp == "" {
			t.Error("Timestamp not set")
		}
		totalIn += e.InputTokens
		totalOut += e.OutputTokens
		totalCache += e.CacheReadTokens
	}

	// Sums over the two unique events: input 100+200, output 50+80, cache 30+0.
	if totalIn != 300 {
		t.Errorf("input sum = %d, want 300", totalIn)
	}
	if totalOut != 130 {
		t.Errorf("output sum = %d, want 130", totalOut)
	}
	if totalCache != 30 {
		t.Errorf("cache-read sum = %d, want 30 (from metadata JSON)", totalCache)
	}

	// Rescan with the same state must emit nothing new (incremental cursor +
	// dedup backstop).
	events2, report2, err := scanGoose([]string{base}, state)
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

func TestScanGooseMissingDirNoError(t *testing.T) {
	events, report, err := scanGoose([]string{filepath.Join(t.TempDir(), "does-not-exist")}, NewState())
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
