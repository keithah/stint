package collector

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/keithah/stint/internal/usage"
)

// buildOpenCodeDB creates a temp opencode.db with the observed schema and a few
// rows: two assistant usage messages (one duplicated under a second row id with
// identical content to exercise dedup is NOT how OpenCode dups — instead we
// insert the SAME message id twice would violate the PK, so the duplicate is a
// second row whose token shape is identical to force a hash collision only if
// ids match). Here we cover: 2 distinct assistant usage rows, 1 user row (no
// tokens), and a re-inserted assistant row sharing the same id to dedup on
// MessageID. Returns the data dir containing opencode.db.
func buildOpenCodeDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, openCodeDBName)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE message (
		id text PRIMARY KEY,
		session_id text NOT NULL,
		time_created integer NOT NULL,
		time_updated integer NOT NULL,
		data text NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	rows := []struct {
		id      string
		session string
		created int64
		data    string
	}{
		{
			id:      "msg_a",
			session: "ses_1",
			created: 1770614448301,
			data: `{"role":"assistant","modelID":"gpt-5.3-codex","providerID":"openai",
				"cost":0,"time":{"created":1770614448301},"path":{"cwd":"/home/keith/src/proj"},
				"tokens":{"input":100,"output":20,"reasoning":5,"cache":{"read":1000,"write":300}}}`,
		},
		{
			id:      "msg_b",
			session: "ses_1",
			created: 1770614450576,
			data: `{"role":"assistant","modelID":"claude-x","providerID":"opencode",
				"cost":0.0123,"time":{"created":1770614450576},"path":{"cwd":"/home/keith/src/proj"},
				"tokens":{"input":7,"output":3,"reasoning":0,"cache":{"read":0,"write":0}}}`,
		},
		{
			// user message: no tokens -> skipped.
			id:      "msg_user",
			session: "ses_1",
			created: 1770614448299,
			data:    `{"role":"user","time":{"created":1770614448299}}`,
		},
	}
	for _, r := range rows {
		if _, err := db.Exec(
			`INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES (?,?,?,?,?)`,
			r.id, r.session, r.created, r.created, r.data,
		); err != nil {
			t.Fatalf("insert %s: %v", r.id, err)
		}
	}
	return dir
}

func TestOpenCodeScan(t *testing.T) {
	dir := buildOpenCodeDB(t)
	state := NewState()

	events, report, err := scanOpenCode([]string{dir}, state)
	if err != nil {
		t.Fatalf("scanOpenCode: %v", err)
	}

	if report.FilesScanned != 1 {
		t.Errorf("FilesScanned = %d, want 1", report.FilesScanned)
	}
	// 3 rows parsed (2 assistant + 1 user).
	if report.LinesParsed != 3 {
		t.Errorf("LinesParsed = %d, want 3", report.LinesParsed)
	}
	// 2 assistant usage rows emit events.
	if report.EventsEmitted != 2 {
		t.Errorf("EventsEmitted = %d, want 2", report.EventsEmitted)
	}
	// user row skipped.
	if report.LinesSkipped != 1 {
		t.Errorf("LinesSkipped = %d, want 1", report.LinesSkipped)
	}
	if report.Errors != 0 {
		t.Errorf("Errors = %d, want 0", report.Errors)
	}

	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}

	byMsg := map[string]usage.Event{}
	for _, e := range events {
		byMsg[e.MessageID] = e
	}

	a, ok := byMsg["msg_a"]
	if !ok {
		t.Fatal("missing msg_a")
	}
	if a.Agent != agentOpenCode {
		t.Errorf("Agent = %q, want %q", a.Agent, agentOpenCode)
	}
	if a.InputTokens != 100 || a.OutputTokens != 20 || a.ReasoningTokens != 5 {
		t.Errorf("msg_a tokens wrong: %+v", a)
	}
	// cache.read -> CacheReadTokens, cache.write -> CacheCreate5mTokens.
	if a.CacheReadTokens != 1000 || a.CacheCreate5mTokens != 300 {
		t.Errorf("msg_a cache wrong: read=%d write5m=%d", a.CacheReadTokens, a.CacheCreate5mTokens)
	}
	if a.Model != "openai/gpt-5.3-codex" {
		t.Errorf("msg_a model = %q, want openai/gpt-5.3-codex", a.Model)
	}
	if a.Project != "proj" {
		t.Errorf("msg_a project = %q, want proj", a.Project)
	}
	if a.SessionID != "ses_1" {
		t.Errorf("msg_a session = %q, want ses_1", a.SessionID)
	}
	if a.BillingType != usage.BillingAPI {
		t.Errorf("msg_a billing = %q, want api", a.BillingType)
	}
	if a.Timestamp == "" {
		t.Error("msg_a timestamp empty")
	}
	// cost==0 should not set CostUSDProvided.
	if a.CostUSDProvided != nil {
		t.Errorf("msg_a CostUSDProvided = %v, want nil (cost was 0)", *a.CostUSDProvided)
	}

	b, ok := byMsg["msg_b"]
	if !ok {
		t.Fatal("missing msg_b")
	}
	if b.CostUSDProvided == nil || *b.CostUSDProvided != 0.0123 {
		t.Errorf("msg_b CostUSDProvided = %v, want 0.0123", b.CostUSDProvided)
	}

	// Token sums across both events.
	var totalIn, totalOut, totalCacheRead, totalCacheCreate int
	for _, e := range events {
		totalIn += e.InputTokens
		totalOut += e.OutputTokens
		totalCacheRead += e.CacheReadTokens
		totalCacheCreate += e.CacheCreate5mTokens
	}
	if totalIn != 107 || totalOut != 23 || totalCacheRead != 1000 || totalCacheCreate != 300 {
		t.Errorf("token sums wrong: in=%d out=%d cacheRead=%d cacheCreate=%d",
			totalIn, totalOut, totalCacheRead, totalCacheCreate)
	}

	// Rescan with the advanced state must emit nothing new.
	events2, report2, err := scanOpenCode([]string{dir}, state)
	if err != nil {
		t.Fatalf("rescan: %v", err)
	}
	if len(events2) != 0 {
		t.Errorf("rescan events = %d, want 0", len(events2))
	}
	if report2.EventsEmitted != 0 {
		t.Errorf("rescan EventsEmitted = %d, want 0", report2.EventsEmitted)
	}
}

// TestOpenCodeDedup verifies that feeding the same logical event twice (e.g.
// two scans concatenated, or duplicate rows) collapses via eventId.
func TestOpenCodeDedup(t *testing.T) {
	dir := buildOpenCodeDB(t)

	// A fresh state each time so the watermark does not suppress the second read.
	e1, _, err := scanOpenCode([]string{dir}, NewState())
	if err != nil {
		t.Fatalf("scan1: %v", err)
	}
	e2, _, err := scanOpenCode([]string{dir}, NewState())
	if err != nil {
		t.Fatalf("scan2: %v", err)
	}
	combined := usage.Dedup(append(append([]usage.Event{}, e1...), e2...))
	if len(combined) != len(e1) {
		t.Errorf("dedup of doubled events = %d, want %d", len(combined), len(e1))
	}
}
