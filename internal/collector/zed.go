package collector

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

const agentZed = "zed"

// Zed stores agent threads in a SQLite database (default
// ~/.local/share/zed/threads/threads.db). Each row in the `threads` table holds
// a thread id, a summary/title, an updated timestamp, and a serialized JSON blob
// of the conversation. The JSON contains a list of messages; assistant messages
// carry a per-message token-usage object (input/output, and sometimes cache
// tokens) plus the model used.
//
// NOTE: This adapter is implemented against Zed's documented threads schema. It
// has NOT been verified against real Zed data on this host. The column/JSON
// field handling is intentionally lenient so reasonable schema variants still
// parse.

// zedThread is one decoded `threads.data_json` blob.
type zedThread struct {
	ID       string       `json:"id"`
	Summary  string       `json:"summary"`
	Messages []zedMessage `json:"messages"`
}

// zedMessage is one message in a thread. Only assistant messages with usage
// produce events.
type zedMessage struct {
	ID    string `json:"id"`
	Role  string `json:"role"`
	Model string `json:"model"`
	// Zed has carried the model name under a few keys across versions.
	ModelAlt  string    `json:"model_name"`
	Timestamp string    `json:"timestamp"`
	UpdatedAt string    `json:"updated_at"`
	Usage     *zedUsage `json:"usage"`
	// Some Zed builds nest token usage under "token_usage".
	TokenUsage *zedUsage `json:"token_usage"`
}

// zedUsage mirrors the token-usage object Zed records per assistant message.
// The core shape is Anthropic's message.usage (decoded via anthropicUsageBlock);
// the extra fields tolerate the alternate snake_case spellings seen across Zed
// versions, applied as a fallback when the canonical keys are absent.
type zedUsage struct {
	anthropicUsageBlock
	// Alternate spellings.
	CacheReadAlt   int `json:"cache_read_tokens"`
	CacheCreateAlt int `json:"cache_creation_tokens"`
}

// canonical applies the Anthropic conventions, then overlays Zed's alternate
// cache spellings when the canonical fields were absent.
func (u *zedUsage) canonical() tokenUsage {
	t := u.anthropicUsageBlock.canonical()
	if t.CacheRead == 0 {
		t.CacheRead = u.CacheReadAlt
	}
	if t.CacheCreate5m == 0 && t.CacheCreate1h == 0 {
		t.CacheCreate5m = u.CacheCreateAlt
	}
	return t
}

func (m *zedMessage) usage() *zedUsage {
	if m.Usage != nil {
		return m.Usage
	}
	return m.TokenUsage
}

func (m *zedMessage) model() string {
	if m.Model != "" {
		return m.Model
	}
	return m.ModelAlt
}

func (m *zedMessage) timestamp() string {
	if m.Timestamp != "" {
		return m.Timestamp
	}
	return m.UpdatedAt
}

// scanZed implements the Adapter signature for Zed agent threads. It opens each
// threads.db under the base dirs, reads thread rows, decodes the JSON message
// blob, and emits one event per assistant message that has usage.
//
// Incremental state is coarse: the DB file's size+mtime are recorded, and an
// unchanged DB is skipped on re-scan. The per-event eventId dedup is the
// backstop that guarantees re-emitting the same data never double-counts.
func scanZed(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	if state == nil {
		state = NewState()
	}
	var (
		events []usage.Event
		report ScanReport
	)
	for _, base := range baseDirs {
		dbs, err := zedDBs(base)
		if err != nil {
			report.Errors++
			continue
		}
		for _, path := range dbs {
			report.FilesScanned++
			scanZedDB(path, state, &events, &report)
		}
	}
	return usage.Dedup(events), report, nil
}

// zedDBs returns the threads.db files to scan for a base. A base may be the DB
// file itself, or a directory containing threads.db (recursively).
func zedDBs(base string) ([]string, error) {
	info, err := os.Stat(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		if strings.HasSuffix(base, ".db") || strings.HasSuffix(base, ".sqlite") {
			return []string{base}, nil
		}
		return nil, nil
	}
	var dbs []string
	err = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // tolerate unreadable subtrees
		}
		if !d.IsDir() && (strings.HasSuffix(d.Name(), ".db") || strings.HasSuffix(d.Name(), ".sqlite")) {
			dbs = append(dbs, p)
		}
		return nil
	})
	return dbs, err
}

// scanZedDB opens one SQLite DB and scans its thread rows. It never returns an
// error; per-DB and per-row problems are counted in the report.
func scanZedDB(path string, state *State, events *[]usage.Event, report *ScanReport) {
	info, err := os.Stat(path)
	if err != nil {
		report.Errors++
		return
	}
	size := info.Size()
	mtime := info.ModTime().UnixNano()

	// Whole-file skip: an unchanged DB (same size+mtime) has nothing new.
	if size > 0 && state.FileUnchanged(path, size, mtime) {
		return
	}

	db, err := openReadOnlySQLite(path)
	if err != nil {
		report.Errors++
		return
	}
	defer db.Close()

	col, table, ok := zedDataColumn(db)
	if !ok {
		report.Errors++
		// Still commit so we don't re-probe an unparseable DB forever; eventId
		// dedup means re-reading later is harmless anyway.
		state.CommitFile(path, size, mtime, 0)
		return
	}

	rows, err := db.Query("SELECT " + sqliteQuoteIdent(col) + " FROM " + sqliteQuoteIdent(table))
	if err != nil {
		report.Errors++
		return
	}
	defer rows.Close()

	rowCount := 0
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			report.Errors++
			report.LinesSkipped++
			continue
		}
		rowCount++
		report.LinesParsed++
		scanZedThreadBlob(blob, events, report)
	}
	if err := rows.Err(); err != nil {
		report.Errors++
	}

	state.CommitFile(path, size, mtime, rowCount)
}

// zedDataColumn discovers which table/column holds the JSON thread blob. Zed's
// threads table has varied the column name (data_json / data / state), so probe
// the schema rather than assume one. Falls back to the conventional
// threads.data_json.
func zedDataColumn(db *sql.DB) (col, table string, ok bool) {
	// Candidate (table, column) pairs in priority order.
	candidates := []struct{ table, col string }{
		{"threads", "data_json"},
		{"threads", "data"},
		{"threads", "state"},
		{"threads", "json"},
	}
	cols := map[string]map[string]bool{} // table -> set of columns
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err == nil {
		defer rows.Close()
		var tables []string
		for rows.Next() {
			var name string
			if rows.Scan(&name) == nil {
				tables = append(tables, name)
			}
		}
		for _, t := range tables {
			cset, err := sqliteTableColumns(db, t)
			if err != nil {
				continue
			}
			cols[t] = cset
		}
	}
	for _, c := range candidates {
		if set, found := cols[c.table]; found && set[c.col] {
			return c.col, c.table, true
		}
	}
	// Last resort: any table that has one of the known blob columns.
	for t, set := range cols {
		for _, name := range []string{"data_json", "data", "state", "json"} {
			if set[name] {
				return name, t, true
			}
		}
	}
	return "", "", false
}

// scanZedThreadBlob decodes one thread blob and appends an event for each
// assistant message that carries usage.
func scanZedThreadBlob(blob []byte, events *[]usage.Event, report *ScanReport) {
	trimmed := strings.TrimSpace(string(blob))
	if trimmed == "" {
		report.LinesSkipped++
		return
	}
	var th zedThread
	if err := json.Unmarshal([]byte(trimmed), &th); err != nil {
		report.Errors++
		report.LinesSkipped++
		return
	}
	for i := range th.Messages {
		ev, ok := zedEventFromMessage(&th, &th.Messages[i])
		if !ok {
			report.LinesSkipped++
			continue
		}
		*events = append(*events, ev)
		report.EventsEmitted++
	}
}

// zedEventFromMessage maps one message to an event. ok=false means the message
// has no usage (skip, not an error).
func zedEventFromMessage(th *zedThread, m *zedMessage) (usage.Event, bool) {
	u := m.usage()
	if u == nil {
		return usage.Event{}, false
	}

	ev := usage.Event{
		Agent:       agentZed,
		MessageID:   m.ID,
		SessionID:   th.ID,
		Project:     th.Summary,
		Model:       m.model(),
		BillingType: usage.BillingAPI,
	}
	u.canonical().apply(&ev)

	ts, tzMin := normalizeTimestamp(m.timestamp())
	ev.Timestamp = ts
	ev.TZOffsetMinutes = tzMin

	if !ev.HasUsage() {
		return usage.Event{}, false
	}

	ev.EnsureID()
	return ev, true
}
