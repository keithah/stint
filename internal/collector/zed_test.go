package collector

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/keithah/stint/internal/usage"

	_ "modernc.org/sqlite"
)

// buildZedFixtureDB creates a temp SQLite DB shaped like Zed's threads.db: a
// `threads` table whose `data_json` column holds a serialized JSON thread with a
// messages array. The fixture exercises: two distinct usage messages, a
// duplicate of one of them (same message id), and a no-usage message.
func buildZedFixtureDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "threads.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE threads (id TEXT PRIMARY KEY, summary TEXT, updated_at TEXT, data_json TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	// Thread 1: two distinct usage messages + one no-usage user message.
	thread1 := `{
		"id": "thread-1",
		"summary": "stint project",
		"messages": [
			{"id": "u-1", "role": "user", "timestamp": "2026-06-22T10:00:00Z"},
			{"id": "a-1", "role": "assistant", "model": "claude-sonnet-4-6",
			 "timestamp": "2026-06-22T10:00:05Z",
			 "usage": {"input_tokens": 100, "output_tokens": 50,
			           "cache_read_input_tokens": 200, "cache_creation_input_tokens": 30}},
			{"id": "a-2", "role": "assistant", "model": "claude-opus-4",
			 "timestamp": "2026-06-22T10:01:00Z",
			 "usage": {"input_tokens": 10, "output_tokens": 5}}
		]
	}`

	// Thread 2: a duplicate of a-1 (same message id, same usage). After dedup it
	// must collapse into the single a-1 event.
	thread2 := `{
		"id": "thread-2",
		"summary": "dup thread",
		"messages": [
			{"id": "a-1", "role": "assistant", "model": "claude-sonnet-4-6",
			 "timestamp": "2026-06-22T10:00:05Z",
			 "usage": {"input_tokens": 100, "output_tokens": 50,
			           "cache_read_input_tokens": 200, "cache_creation_input_tokens": 30}}
		]
	}`

	if _, err := db.Exec(`INSERT INTO threads (id, summary, updated_at, data_json) VALUES (?,?,?,?)`,
		"thread-1", "stint project", "2026-06-22T10:01:00Z", thread1); err != nil {
		t.Fatalf("insert thread1: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO threads (id, summary, updated_at, data_json) VALUES (?,?,?,?)`,
		"thread-2", "dup thread", "2026-06-22T10:00:05Z", thread2); err != nil {
		t.Fatalf("insert thread2: %v", err)
	}
	return path
}

func TestZedScan(t *testing.T) {
	path := buildZedFixtureDB(t)
	state := NewState()

	events, report, err := scanZed([]string{path}, state)
	if err != nil {
		t.Fatalf("scanZed: %v", err)
	}

	if report.FilesScanned != 1 {
		t.Errorf("FilesScanned = %d, want 1", report.FilesScanned)
	}
	// 2 thread rows parsed.
	if report.LinesParsed != 2 {
		t.Errorf("LinesParsed = %d, want 2", report.LinesParsed)
	}
	// 3 usage messages emit events pre-dedup (a-1, a-2, dup a-1).
	if report.EventsEmitted != 3 {
		t.Errorf("EventsEmitted = %d, want 3", report.EventsEmitted)
	}
	// no-usage user message u-1 is skipped.
	if report.LinesSkipped != 1 {
		t.Errorf("LinesSkipped = %d, want 1", report.LinesSkipped)
	}
	if report.Errors != 0 {
		t.Errorf("Errors = %d, want 0", report.Errors)
	}

	// After dedup the duplicate a-1 collapses: 2 distinct events.
	if len(events) != 2 {
		t.Fatalf("deduped events = %d, want 2", len(events))
	}

	// Verify token sums and field mapping across the deduped set.
	var (
		sumIn, sumOut, sumCacheRead, sumCacheCreate int
		sawSonnet, sawOpus                          bool
	)
	for _, ev := range events {
		if ev.Agent != agentZed {
			t.Errorf("Agent = %q, want %q", ev.Agent, agentZed)
		}
		if ev.BillingType != usage.BillingAPI {
			t.Errorf("BillingType = %q, want %q", ev.BillingType, usage.BillingAPI)
		}
		sumIn += ev.InputTokens
		sumOut += ev.OutputTokens
		sumCacheRead += ev.CacheReadTokens
		sumCacheCreate += ev.CacheCreate5mTokens
		switch ev.Model {
		case "claude-sonnet-4-6":
			sawSonnet = true
			if ev.SessionID != "thread-1" {
				t.Errorf("a-1 SessionID = %q, want thread-1", ev.SessionID)
			}
			if ev.Project != "stint project" {
				t.Errorf("a-1 Project = %q, want 'stint project'", ev.Project)
			}
			if ev.Timestamp != "2026-06-22T10:00:05Z" {
				t.Errorf("a-1 Timestamp = %q", ev.Timestamp)
			}
		case "claude-opus-4":
			sawOpus = true
		}
	}
	if !sawSonnet || !sawOpus {
		t.Errorf("expected both models present: sonnet=%v opus=%v", sawSonnet, sawOpus)
	}
	// a-1: in100 out50 cacheRead200 cacheCreate30 ; a-2: in10 out5.
	if sumIn != 110 {
		t.Errorf("sum InputTokens = %d, want 110", sumIn)
	}
	if sumOut != 55 {
		t.Errorf("sum OutputTokens = %d, want 55", sumOut)
	}
	if sumCacheRead != 200 {
		t.Errorf("sum CacheReadTokens = %d, want 200", sumCacheRead)
	}
	if sumCacheCreate != 30 {
		t.Errorf("sum CacheCreate5mTokens = %d, want 30", sumCacheCreate)
	}

	// Rescan with the same state must emit nothing new (coarse incremental skip),
	// and even if it did read again, dedup is the backstop.
	events2, report2, err := scanZed([]string{path}, state)
	if err != nil {
		t.Fatalf("rescan scanZed: %v", err)
	}
	if report2.EventsEmitted != 0 {
		t.Errorf("rescan EventsEmitted = %d, want 0 (DB unchanged, skipped)", report2.EventsEmitted)
	}
	if len(events2) != 0 {
		t.Errorf("rescan deduped events = %d, want 0", len(events2))
	}
}
