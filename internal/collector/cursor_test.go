package collector

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/keithah/stint/internal/usage"
)

const cursorCSVDir = "testdata/cursor"

// TestCursorScanCSV exercises the Cursor dashboard usage-export CSV path.
func TestCursorScanCSV(t *testing.T) {
	events, report, err := scanCursor([]string{cursorCSVDir}, NewState())
	if err != nil {
		t.Fatalf("scanCursor: %v", err)
	}

	if report.FilesScanned != 1 {
		t.Errorf("FilesScanned = %d, want 1", report.FilesScanned)
	}
	// 4 data rows read OK: 2 identical usage, 1 errored (zero), 1 usage. The 5th
	// row has a bare quote and is a CSV parse error (not counted in LinesParsed).
	if report.LinesParsed != 4 {
		t.Errorf("LinesParsed = %d, want 4", report.LinesParsed)
	}
	// 3 usage rows emit events (pre-dedup): the two identical + the gpt-4o one.
	if report.EventsEmitted != 3 {
		t.Errorf("EventsEmitted = %d, want 3", report.EventsEmitted)
	}
	// The malformed CSV row is an error.
	if report.Errors != 1 {
		t.Errorf("Errors = %d, want 1 (malformed CSV row)", report.Errors)
	}
	// Skipped: the all-zero errored row.
	if report.LinesSkipped != 1 {
		t.Errorf("LinesSkipped = %d, want 1 (errored zero-token row)", report.LinesSkipped)
	}

	// After dedup the two identical claude rows collapse => 2 events.
	if len(events) != 2 {
		t.Fatalf("deduped events = %d, want 2", len(events))
	}

	var claude, gpt usage.Event
	var gotC, gotG bool
	for _, e := range events {
		switch e.Model {
		case "claude-3-5-sonnet":
			claude, gotC = e, true
		case "gpt-4o":
			gpt, gotG = e, true
		}
	}
	if !gotC || !gotG {
		t.Fatalf("missing events: claude=%v gpt=%v", gotC, gotG)
	}

	// claude row: Input (w/o Cache Write) = 2000 is the plain input; cache write
	// 1500, cache read 1500, output 450.
	if claude.InputTokens != 2000 {
		t.Errorf("claude InputTokens = %d, want 2000", claude.InputTokens)
	}
	if claude.CacheCreate5mTokens != 1500 {
		t.Errorf("claude CacheCreate5mTokens = %d, want 1500", claude.CacheCreate5mTokens)
	}
	if claude.CacheReadTokens != 1500 {
		t.Errorf("claude CacheReadTokens = %d, want 1500", claude.CacheReadTokens)
	}
	if claude.OutputTokens != 450 {
		t.Errorf("claude OutputTokens = %d, want 450", claude.OutputTokens)
	}
	if claude.CostUSDProvided == nil || *claude.CostUSDProvided != 0.0421 {
		t.Errorf("claude CostUSDProvided = %v, want 0.0421", claude.CostUSDProvided)
	}
	if claude.BillingType != usage.BillingSubscription {
		t.Errorf("claude BillingType = %q, want subscription", claude.BillingType)
	}
	if claude.Timestamp == "" {
		t.Error("claude Timestamp empty")
	}

	if gpt.InputTokens != 900 || gpt.OutputTokens != 200 {
		t.Errorf("gpt tokens wrong: %+v", gpt)
	}

	// Token sums across the deduped set.
	var totIn, totOut, totRead, totCreate int
	for _, e := range events {
		totIn += e.InputTokens
		totOut += e.OutputTokens
		totRead += e.CacheReadTokens
		totCreate += e.CacheCreate5mTokens
	}
	if totIn != 2900 { // 2000 + 900
		t.Errorf("total InputTokens = %d, want 2900", totIn)
	}
	if totOut != 650 { // 450 + 200
		t.Errorf("total OutputTokens = %d, want 650", totOut)
	}
	if totRead != 1500 {
		t.Errorf("total CacheReadTokens = %d, want 1500", totRead)
	}
	if totCreate != 1500 {
		t.Errorf("total CacheCreate5mTokens = %d, want 1500", totCreate)
	}

	// Rescan with the advanced state must emit nothing new (watermark).
	events2, _, err := scanCursor([]string{cursorCSVDir}, stateFromFirst(t))
	if err != nil {
		t.Fatalf("rescan: %v", err)
	}
	_ = events2
}

// stateFromFirst runs one scan to advance a State, then returns it so a second
// scan can confirm the row watermark suppresses re-emission.
func stateFromFirst(t *testing.T) *State {
	t.Helper()
	st := NewState()
	if _, _, err := scanCursor([]string{cursorCSVDir}, st); err != nil {
		t.Fatalf("priming scan: %v", err)
	}
	return st
}

func TestCursorCSVWatermark(t *testing.T) {
	st := NewState()
	if _, _, err := scanCursor([]string{cursorCSVDir}, st); err != nil {
		t.Fatalf("scan1: %v", err)
	}
	events2, report2, err := scanCursor([]string{cursorCSVDir}, st)
	if err != nil {
		t.Fatalf("scan2: %v", err)
	}
	if len(events2) != 0 {
		t.Errorf("rescan events = %d, want 0 (watermark)", len(events2))
	}
	if report2.EventsEmitted != 0 {
		t.Errorf("rescan EventsEmitted = %d, want 0", report2.EventsEmitted)
	}
}

// TestCursorDedup verifies feeding the CSV twice collapses via eventId.
func TestCursorDedup(t *testing.T) {
	e1, _, err := scanCursor([]string{cursorCSVDir}, NewState())
	if err != nil {
		t.Fatalf("scan1: %v", err)
	}
	e2, _, err := scanCursor([]string{cursorCSVDir}, NewState())
	if err != nil {
		t.Fatalf("scan2: %v", err)
	}
	combined := usage.Dedup(append(append([]usage.Event{}, e1...), e2...))
	if len(combined) != len(e1) {
		t.Errorf("dedup of doubled events = %d, want %d", len(combined), len(e1))
	}
}

// buildCursorStateDB creates a temp state.vscdb with a cursorUsage table so the
// SQLite probe path is exercised: 2 usage rows + 1 zero-token row (skipped).
func buildCursorStateDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.vscdb")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	// VS Code-style key/value table (ignored) plus a usage table.
	if _, err := db.Exec(`CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value BLOB)`); err != nil {
		t.Fatalf("create ItemTable: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE cursorUsage (
		request_id TEXT PRIMARY KEY,
		model TEXT,
		input_tokens INTEGER,
		cache_write_tokens INTEGER,
		cache_read_tokens INTEGER,
		output_tokens INTEGER,
		cost_usd REAL,
		ts_ms INTEGER
	)`); err != nil {
		t.Fatalf("create cursorUsage: %v", err)
	}

	rows := []struct {
		req             string
		model           string
		in, cw, cr, out int
		cost            float64
		ts              int64
	}{
		{"req-1", "claude-3-5-sonnet", 1000, 200, 800, 150, 0.02, 1750586400000},
		{"req-2", "gpt-4o", 400, 0, 0, 60, 0.005, 1750586460000},
		{"req-zero", "gpt-4o", 0, 0, 0, 0, 0, 1750586520000}, // skipped, no usage
	}
	for _, r := range rows {
		if _, err := db.Exec(
			`INSERT INTO cursorUsage VALUES (?,?,?,?,?,?,?,?)`,
			r.req, r.model, r.in, r.cw, r.cr, r.out, r.cost, r.ts,
		); err != nil {
			t.Fatalf("insert %s: %v", r.req, err)
		}
	}
	return dir
}

func TestCursorScanDB(t *testing.T) {
	dir := buildCursorStateDB(t)

	events, report, err := scanCursor([]string{dir}, NewState())
	if err != nil {
		t.Fatalf("scanCursor DB: %v", err)
	}

	if report.FilesScanned != 1 {
		t.Errorf("FilesScanned = %d, want 1", report.FilesScanned)
	}
	if report.EventsEmitted != 2 {
		t.Errorf("EventsEmitted = %d, want 2", report.EventsEmitted)
	}
	if report.LinesSkipped != 1 {
		t.Errorf("LinesSkipped = %d, want 1 (zero-token row)", report.LinesSkipped)
	}
	if report.Errors != 0 {
		t.Errorf("Errors = %d, want 0", report.Errors)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}

	byReq := map[string]usage.Event{}
	for _, e := range events {
		byReq[e.RequestID] = e
	}
	r1, ok := byReq["req-1"]
	if !ok {
		t.Fatal("missing req-1")
	}
	if r1.InputTokens != 1000 || r1.CacheCreate5mTokens != 200 || r1.CacheReadTokens != 800 || r1.OutputTokens != 150 {
		t.Errorf("req-1 tokens wrong: %+v", r1)
	}
	if r1.Agent != agentCursor || r1.BillingType != usage.BillingSubscription {
		t.Errorf("req-1 agent/billing wrong: %+v", r1)
	}
	if r1.CostUSDProvided == nil || *r1.CostUSDProvided != 0.02 {
		t.Errorf("req-1 cost = %v, want 0.02", r1.CostUSDProvided)
	}
}
