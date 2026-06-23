package collector

import (
	"database/sql"
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/keithah/stint/internal/usage"
)

const agentCursor = "cursor"

// Cursor has no always-on local token log the way Claude/Codex do. Trackers
// (ccusage / tokscale) read what Cursor exposes: (a) the per-event usage CSV
// you export from the Cursor dashboard, and (b) the local Cursor state SQLite
// database (state.vscdb), which sometimes caches usage rows. Cursor token usage
// is therefore only ever as COMPLETE as the export — anything not present in
// the CSV (or cached in the DB) cannot be recovered locally.
//
// Locations resolved here (a base dir may be any of these):
//   - CSV export:  ~/.cursor/*.csv  (or a configured dir / explicit .csv file)
//   - state DB:    ~/Library/Application Support/Cursor/User/globalStorage/state.vscdb (macOS)
//                  ~/.config/Cursor/User/globalStorage/state.vscdb               (Linux)
//
// The CSV path is the authoritative one; the DB probe is best-effort.

// The Cursor dashboard "usage events" CSV export header. Cursor has shipped a
// few header variants; we match columns case-insensitively by normalized name
// so minor wording/punctuation drift does not break parsing. Recognized fields:
//
//	Date                          -> timestamp
//	Model                         -> model
//	Kind                          -> request kind (used to skip non-usage rows)
//	Input (w/ Cache Write)        -> input incl. cache-write (we derive plain input)
//	Input (w/o Cache Write)       -> plain input tokens (preferred when present)
//	Cache Write Tokens            -> cache creation
//	Cache Read Tokens             -> cache read
//	Output Tokens                 -> output
//	Cost ($) / Cost               -> provider-reported cost
//	User / Email                  -> ignored (scrubbed)

// cursorMaxStateRows caps how many state.vscdb rows we will probe to keep a
// large DB from dominating a scan.
const cursorMaxStateRows = 5000

// scanCursor implements the Adapter for Cursor. For each base dir it parses any
// CSV usage export found and probes a Cursor state SQLite DB for usage rows.
func scanCursor(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	if state == nil {
		state = NewState()
	}
	var (
		events []usage.Event
		report ScanReport
	)
	for _, base := range baseDirs {
		csvFiles, dbFiles := cursorResolve(base)
		for _, p := range csvFiles {
			report.FilesScanned++
			cursorScanCSV(p, state, &events, &report)
		}
		for _, p := range dbFiles {
			report.FilesScanned++
			cursorScanDB(p, state, &events, &report)
		}
	}
	return usage.Dedup(events), report, nil
}

// cursorResolve classifies a base into CSV export files and state DB files. A
// base may be a directory (we look for *.csv and state.vscdb under it,
// recursively) or a direct path to a .csv / .vscdb / .db file.
func cursorResolve(base string) (csvFiles, dbFiles []string) {
	info, err := os.Stat(base)
	if err != nil {
		return nil, nil
	}
	if !info.IsDir() {
		switch {
		case strings.HasSuffix(base, ".csv"):
			return []string{base}, nil
		case strings.HasSuffix(base, ".vscdb") || strings.HasSuffix(base, ".db"):
			return nil, []string{base}
		}
		return nil, nil
	}
	_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // tolerate unreadable subtrees
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		switch {
		case strings.HasSuffix(name, ".csv"):
			csvFiles = append(csvFiles, p)
		case name == "state.vscdb" || strings.HasSuffix(name, ".vscdb"):
			dbFiles = append(dbFiles, p)
		}
		return nil
	})
	return csvFiles, dbFiles
}

// cursorScanCSV parses one Cursor usage-export CSV. It reads only the rows past
// the per-file line watermark in State so a re-scan emits nothing new. Bad rows
// are counted, not fatal.
func cursorScanCSV(path string, state *State, events *[]usage.Event, report *ScanReport) {
	info, err := os.Stat(path)
	if err != nil {
		report.Errors++
		return
	}
	f, err := os.Open(path)
	if err != nil {
		report.Errors++
		return
	}
	defer f.Close()

	size := info.Size()
	mtime := info.ModTime().UnixNano()
	// Watermark: number of data rows already emitted. If the file shrank below
	// the recorded size, treat it as a fresh export and start over.
	already := state.RowCount(path, size)

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // tolerate ragged rows
	r.TrimLeadingSpace = true

	header, err := r.Read()
	if err != nil {
		if err != io.EOF {
			report.Errors++
		}
		return
	}
	cols := cursorHeaderIndex(header)
	defaultSession := strings.TrimSuffix(filepath.Base(path), ".csv")

	rowNo := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			report.Errors++
			continue
		}
		rowNo++
		report.LinesParsed++
		if rowNo <= already {
			continue // already emitted in a prior scan
		}
		ev, ok := cursorRowToEvent(rec, cols, defaultSession)
		if !ok {
			report.LinesSkipped++
			continue
		}
		*events = append(*events, ev)
		report.EventsEmitted++
	}

	// Commit the new row count as the watermark.
	if rowNo > already {
		state.CommitRowCount(path, size, mtime, rowNo)
	}
}

// cursorCols holds the resolved column indices for a Cursor CSV (or -1).
type cursorCols struct {
	date         int
	model        int
	kind         int
	inputWith    int // input incl. cache write
	inputWithout int // plain input
	cacheWrite   int
	cacheRead    int
	output       int
	cost         int
}

// cursorHeaderIndex maps the CSV header row to column indices by normalized
// name, tolerating wording/punctuation/case drift across export versions.
func cursorHeaderIndex(header []string) cursorCols {
	c := cursorCols{date: -1, model: -1, kind: -1, inputWith: -1, inputWithout: -1,
		cacheWrite: -1, cacheRead: -1, output: -1, cost: -1}
	for i, h := range header {
		switch n := cursorNorm(h); {
		case n == "date" || n == "timestamp" || n == "time":
			c.date = i
		case n == "model":
			c.model = i
		case n == "kind" || n == "type":
			c.kind = i
		case strings.Contains(n, "input") && strings.Contains(n, "without cache write"):
			c.inputWithout = i
		case strings.Contains(n, "input") && strings.Contains(n, "with cache write"):
			c.inputWith = i
		case strings.Contains(n, "input") && c.inputWithout == -1 && c.inputWith == -1:
			c.inputWithout = i
		case strings.Contains(n, "cache") && strings.Contains(n, "write"):
			c.cacheWrite = i
		case strings.Contains(n, "cache") && strings.Contains(n, "read"):
			c.cacheRead = i
		case strings.Contains(n, "output"):
			c.output = i
		case strings.Contains(n, "cost"):
			c.cost = i
		}
	}
	return c
}

// cursorNorm normalizes a header cell: lowercase, canonicalize the "w/" and
// "w/o" abbreviations (before slashes are stripped), then collapse remaining
// separators/symbols to single spaces. So "Input (w/o Cache Write)" becomes
// "input without cache write" and "Input (w/ Cache Write)" becomes
// "input with cache write".
func cursorNorm(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	pre := strings.NewReplacer("w/o", "without", "w/ ", "with ", "w/", "with")
	s = pre.Replace(s)
	repl := strings.NewReplacer("(", " ", ")", " ", "/", " ", "_", " ", "-", " ",
		".", " ", "$", " ", "#", " ", " ", " ")
	s = repl.Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// cursorRowToEvent maps one CSV data row to an event. ok=false means the row
// carries no usage (e.g. an errored/aborted request) and is skipped.
func cursorRowToEvent(rec []string, c cursorCols, defaultSession string) (usage.Event, bool) {
	get := func(i int) string {
		if i >= 0 && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}

	cacheRead := cursorInt(get(c.cacheRead))
	output := cursorInt(get(c.output))

	// Input + cache-write mapping. The Cursor export carries two input columns:
	//   Input (w/o Cache Write) = plain prompt tokens
	//   Input (w/ Cache Write)  = plain prompt tokens + cache-creation tokens
	// so cache-creation (cache write) is the DIFFERENCE between them. When an
	// explicit cache-write column exists we use it directly instead.
	plainIn := get(c.inputWithout)
	withIn := get(c.inputWith)
	var input, cacheWrite int
	switch {
	case c.cacheWrite >= 0 && get(c.cacheWrite) != "":
		// Explicit cache-write column.
		cacheWrite = cursorInt(get(c.cacheWrite))
		if plainIn != "" {
			input = cursorInt(plainIn)
		} else if withIn != "" {
			input = cursorInt(withIn) - cacheWrite
		}
	case plainIn != "" && withIn != "":
		input = cursorInt(plainIn)
		cacheWrite = cursorInt(withIn) - input
	case plainIn != "":
		input = cursorInt(plainIn)
	case withIn != "":
		input = cursorInt(withIn)
	}
	if input < 0 {
		input = 0
	}
	if cacheWrite < 0 {
		cacheWrite = 0
	}

	ev := usage.Event{
		Agent:               agentCursor,
		SessionID:           defaultSession,
		Model:               get(c.model),
		InputTokens:         input,
		OutputTokens:        output,
		CacheReadTokens:     cacheRead,
		CacheCreate5mTokens: cacheWrite,
		BillingType:         usage.BillingSubscription,
	}

	if cost := get(c.cost); cost != "" {
		if v, err := strconv.ParseFloat(strings.TrimPrefix(cost, "$"), 64); err == nil && v != 0 {
			ev.CostUSDProvided = &v
		}
	}

	ev.Timestamp, ev.TZOffsetMinutes = cursorTimestamp(get(c.date))

	if !ev.HasUsage() {
		return usage.Event{}, false
	}
	ev.EnsureID()
	return ev, true
}

// cursorInt parses a token count that may contain thousands separators.
func cursorInt(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	s = strings.ReplaceAll(s, ",", "")
	n, err := strconv.Atoi(s)
	if err != nil {
		// tolerate a float-formatted integer like "1234.0"
		if f, ferr := strconv.ParseFloat(s, 64); ferr == nil {
			return int(f)
		}
		return 0
	}
	return n
}

// cursorTimestamp parses the CSV date column. Cursor exports an RFC3339-ish
// timestamp; we also accept a few common alternates and epoch millis.
func cursorTimestamp(s string) (string, int) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0
	}
	if ts, tz := normalizeTimestamp(s); ts != s {
		return ts, tz
	}
	// epoch millis?
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
		return normalizeUnixMillis(n), 0
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"01/02/2006 15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC().Format(time.RFC3339), 0
		}
	}
	return s, 0
}

// cursorScanDB probes a Cursor state.vscdb (an SQLite key/value store) for any
// cached usage rows. Cursor stores most state as opaque JSON blobs in an
// ItemTable(key, value) table; only a subset of installs carry usage rows, so
// this is strictly best-effort. We look for a dedicated usage table if present,
// otherwise we do nothing rather than guess at blob shapes.
func cursorScanDB(path string, state *State, events *[]usage.Event, report *ScanReport) {
	info, err := os.Stat(path)
	if err != nil {
		report.Errors++
		return
	}
	size := info.Size()
	mtime := info.ModTime().UnixNano()

	db, err := openReadOnlySQLite(path)
	if err != nil {
		report.Errors++
		return
	}
	defer db.Close()

	// Prefer an explicit usage table if this install has one.
	if cursorHasTable(db, "cursorUsage") {
		cursorScanUsageTable(db, path, state, events, report)
		state.CommitRowCount(path, size, mtime, 0)
		return
	}
	// No recognized usage table: nothing to extract. Leave a watermark so the
	// scan is recorded as having looked at the DB.
	state.CommitRowCount(path, size, mtime, 0)
}

// cursorHasTable reports whether a table exists in the SQLite DB.
func cursorHasTable(db *sql.DB, name string) bool {
	var got string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name,
	).Scan(&got)
	return err == nil && got == name
}

// cursorScanUsageTable reads rows from a dedicated cursorUsage table. The table
// shape (model, input, cache_write, cache_read, output, cost, ts, request_id)
// mirrors the CSV export columns. Per-row failures are counted, not fatal.
func cursorScanUsageTable(db *sql.DB, path string, state *State, events *[]usage.Event, report *ScanReport) {
	rows, err := db.Query(`SELECT request_id, model, input_tokens, cache_write_tokens,
		cache_read_tokens, output_tokens, cost_usd, ts_ms FROM cursorUsage LIMIT ?`,
		cursorMaxStateRows)
	if err != nil {
		report.Errors++
		return
	}
	defer rows.Close()

	for rows.Next() {
		var (
			reqID                                      string
			model                                      string
			input, cacheWrite, cacheRead, output, tsMs int64
			cost                                       sql.NullFloat64
		)
		if err := rows.Scan(&reqID, &model, &input, &cacheWrite, &cacheRead, &output, &cost, &tsMs); err != nil {
			report.Errors++
			report.LinesSkipped++
			continue
		}
		report.LinesParsed++
		ev := usage.Event{
			Agent:               agentCursor,
			RequestID:           reqID,
			Model:               model,
			InputTokens:         int(input),
			OutputTokens:        int(output),
			CacheReadTokens:     int(cacheRead),
			CacheCreate5mTokens: int(cacheWrite),
			BillingType:         usage.BillingSubscription,
		}
		if cost.Valid && cost.Float64 != 0 {
			v := cost.Float64
			ev.CostUSDProvided = &v
		}
		ev.Timestamp = normalizeUnixMillis(tsMs)
		if !ev.HasUsage() {
			report.LinesSkipped++
			continue
		}
		ev.EnsureID()
		*events = append(*events, ev)
		report.EventsEmitted++
	}
	if err := rows.Err(); err != nil {
		report.Errors++
	}
}
