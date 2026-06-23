package collector

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/keithah/stint/internal/usage"
)

const agentGoose = "goose"

// gooseSQLiteDriver is the pure-Go SQLite driver registered by
// modernc.org/sqlite under the name "sqlite".
const gooseSQLiteDriver = "sqlite"

// gooseDBNames are the SQLite database filenames Goose is documented to use for
// its session/message store. The first match under a base dir is scanned.
var gooseDBNames = []string{"sessions.db"}

// gooseTokenColumns enumerates the column-name spellings Goose's schema is
// documented to (or may) use for each mapped token field. The adapter probes
// the actual table columns and binds whichever spelling is present, so a schema
// drift on naming degrades to 0 rather than failing the scan.
//
// NOTE: This adapter is written against Goose's *documented* SQLite schema and
// has NOT been verified against a real Goose sessions DB on this host.
var (
	gooseInputCols  = []string{"input_tokens", "prompt_tokens", "input_token_count"}
	gooseOutputCols = []string{"output_tokens", "completion_tokens", "output_token_count"}
	gooseCacheCols  = []string{"cache_read_tokens", "cache_read_input_tokens", "cached_tokens", "cache_tokens"}
	gooseModelCols  = []string{"model", "model_name"}
	gooseSessCols   = []string{"session_id", "session", "conversation_id"}
	gooseTimeCols   = []string{"created_at", "created", "timestamp", "created_ts", "ts"}
	gooseMetaCols   = []string{"metadata", "usage", "meta", "data"}
)

// gooseUsageMeta is the subset of a per-message JSON metadata/usage blob the
// adapter reads. Goose may carry token usage (including cached tokens) inside a
// JSON column rather than dedicated columns; either source is honored, with
// dedicated columns taking precedence when both are present.
type gooseUsageMeta struct {
	InputTokens  *int   `json:"input_tokens"`
	OutputTokens *int   `json:"output_tokens"`
	PromptTokens *int   `json:"prompt_tokens"`
	Completion   *int   `json:"completion_tokens"`
	CacheRead    *int   `json:"cache_read_tokens"`
	CacheRead2   *int   `json:"cache_read_input_tokens"`
	CachedTokens *int   `json:"cached_tokens"`
	Model        string `json:"model"`

	// Some providers nest counts under a "usage" object.
	Usage *struct {
		InputTokens  *int `json:"input_tokens"`
		OutputTokens *int `json:"output_tokens"`
		PromptTokens *int `json:"prompt_tokens"`
		Completion   *int `json:"completion_tokens"`
		CacheRead    *int `json:"cache_read_tokens"`
		CacheRead2   *int `json:"cache_read_input_tokens"`
		CachedTokens *int `json:"cached_tokens"`
	} `json:"usage"`
}

// scanGoose implements the Adapter signature for Goose. It locates each Goose
// sessions SQLite DB under the base dirs, reads message rows newer than the
// recorded cursor (max rowid), maps per-message token usage to events, and
// returns the deduped set. It never aborts the whole scan on a single bad row;
// anomalies are counted in the report. eventId dedup is the backstop for the
// coarse incremental cursor.
func scanGoose(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	if state == nil {
		state = NewState()
	}
	var (
		events []usage.Event
		report ScanReport
	)
	for _, base := range baseDirs {
		dbs, err := gooseDBs(base)
		if err != nil {
			report.Errors++
			continue
		}
		for _, path := range dbs {
			report.FilesScanned++
			scanGooseDB(path, state, &events, &report)
		}
	}
	return usage.Dedup(events), report, nil
}

// gooseDBs returns the Goose sessions DB paths under base. A base that is itself
// a *.db file is used directly. A missing base yields no paths and no error.
func gooseDBs(base string) ([]string, error) {
	info, err := os.Stat(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		if strings.HasSuffix(base, ".db") {
			return []string{base}, nil
		}
		return nil, nil
	}
	var paths []string
	for _, name := range gooseDBNames {
		p := filepath.Join(base, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			paths = append(paths, p)
		}
	}
	// Fall back to any *.db under base if the well-known name is absent.
	if len(paths) == 0 {
		_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() && strings.HasSuffix(d.Name(), ".db") {
				paths = append(paths, p)
			}
			return nil
		})
	}
	return paths, nil
}

// scanGooseDB opens one DB, resolves the messages table's column layout, reads
// rows past the recorded cursor, and appends events. It never returns an error;
// failures are counted in the report.
func scanGooseDB(path string, state *State, events *[]usage.Event, report *ScanReport) {
	info, err := os.Stat(path)
	if err != nil {
		report.Errors++
		return
	}

	db, err := sql.Open(gooseSQLiteDriver, "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		report.Errors++
		return
	}
	defer db.Close()

	cols, err := gooseTableColumns(db, "messages")
	if err != nil || len(cols) == 0 {
		report.Errors++
		return
	}

	// Resolve which spellings are present.
	inputCol := gooseFirstPresent(cols, gooseInputCols)
	outputCol := gooseFirstPresent(cols, gooseOutputCols)
	cacheCol := gooseFirstPresent(cols, gooseCacheCols)
	modelCol := gooseFirstPresent(cols, gooseModelCols)
	sessCol := gooseFirstPresent(cols, gooseSessCols)
	timeCol := gooseFirstPresent(cols, gooseTimeCols)
	metaCol := gooseFirstPresent(cols, gooseMetaCols)

	// Build the SELECT list. rowid is always first; absent columns select NULL.
	sel := []string{"rowid"}
	sel = append(sel, gooseSelectExpr(inputCol))
	sel = append(sel, gooseSelectExpr(outputCol))
	sel = append(sel, gooseSelectExpr(cacheCol))
	sel = append(sel, gooseSelectExpr(modelCol))
	sel = append(sel, gooseSelectExpr(sessCol))
	sel = append(sel, gooseSelectExpr(timeCol))
	sel = append(sel, gooseSelectExpr(metaCol))

	// Coarse incremental cursor: reuse FileState.Offset as the max rowid seen.
	cursor, _ := state.resume(path, info.Size(), info.ModTime().UnixNano())

	query := "SELECT " + strings.Join(sel, ", ") +
		" FROM messages WHERE rowid > ? ORDER BY rowid ASC"
	rows, err := db.Query(query, cursor)
	if err != nil {
		report.Errors++
		return
	}
	defer rows.Close()

	maxRowID := cursor
	var lineCount int
	for rows.Next() {
		var (
			rowID  int64
			inTok  sql.NullInt64
			outTok sql.NullInt64
			cTok   sql.NullInt64
			model  sql.NullString
			sess   sql.NullString
			tsRaw  sql.NullString
			meta   sql.NullString
		)
		if err := rows.Scan(&rowID, &inTok, &outTok, &cTok, &model, &sess, &tsRaw, &meta); err != nil {
			report.Errors++
			report.LinesSkipped++
			continue
		}
		report.LinesParsed++
		lineCount++
		if rowID > maxRowID {
			maxRowID = rowID
		}

		ev, ok := gooseBuildEvent(rowID, inTok, outTok, cTok, model, sess, tsRaw, meta)
		if !ok {
			report.LinesSkipped++
			continue
		}
		*events = append(*events, ev)
		report.EventsEmitted++
	}
	if err := rows.Err(); err != nil {
		report.Errors++
	}

	state.commit(path, info.Size(), info.ModTime().UnixNano(), maxRowID, lineCount)
}

// gooseBuildEvent maps one scanned row to a usage.Event. ok=false means the row
// carries no usage and should be skipped (not an error).
func gooseBuildEvent(rowID int64, inTok, outTok, cTok sql.NullInt64, model, sess, tsRaw, meta sql.NullString) (usage.Event, bool) {
	input := gooseInt(inTok)
	output := gooseInt(outTok)
	cacheRead := gooseInt(cTok)
	modelName := gooseStr(model)

	// Fold in any JSON metadata/usage blob; dedicated columns take precedence.
	if meta.Valid && strings.TrimSpace(meta.String) != "" {
		if mi, mo, mc, mm, ok := gooseParseMeta(meta.String); ok {
			if input == 0 {
				input = mi
			}
			if output == 0 {
				output = mo
			}
			if cacheRead == 0 {
				cacheRead = mc
			}
			if modelName == "" {
				modelName = mm
			}
		}
	}

	ts, tzMin := normalizeTimestamp(gooseStr(tsRaw))

	ev := usage.Event{
		Agent:           agentGoose,
		SessionID:       gooseStr(sess),
		Model:           modelName,
		InputTokens:     input,
		OutputTokens:    output,
		CacheReadTokens: cacheRead,
		Timestamp:       ts,
		TZOffsetMinutes: tzMin,
		BillingType:     usage.BillingAPI,
	}

	if !ev.HasUsage() {
		return usage.Event{}, false
	}
	ev.EnsureID()
	return ev, true
}

// gooseParseMeta extracts input/output/cache-read tokens and model from a JSON
// metadata/usage blob, honoring either a flat shape or a nested "usage" object.
// ok=false means the blob was not valid JSON.
func gooseParseMeta(s string) (input, output, cacheRead int, model string, ok bool) {
	var m gooseUsageMeta
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return 0, 0, 0, "", false
	}
	input = gooseFirstPtr(m.InputTokens, m.PromptTokens)
	output = gooseFirstPtr(m.OutputTokens, m.Completion)
	cacheRead = gooseFirstPtr(m.CacheRead, m.CacheRead2, m.CachedTokens)
	model = m.Model
	if m.Usage != nil {
		if input == 0 {
			input = gooseFirstPtr(m.Usage.InputTokens, m.Usage.PromptTokens)
		}
		if output == 0 {
			output = gooseFirstPtr(m.Usage.OutputTokens, m.Usage.Completion)
		}
		if cacheRead == 0 {
			cacheRead = gooseFirstPtr(m.Usage.CacheRead, m.Usage.CacheRead2, m.Usage.CachedTokens)
		}
	}
	return input, output, cacheRead, model, true
}

// gooseUTCNote: timestamps are normalized to RFC3339 UTC via
// normalizeTimestamp (shared with the Claude adapter).

// gooseTableColumns returns the lower-cased column names of a table via
// PRAGMA table_info. A missing table yields an empty set (no error from the
// PRAGMA itself), which the caller treats as "no usable table".
func gooseTableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   sql.NullString
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols[strings.ToLower(name)] = true
	}
	return cols, rows.Err()
}

// gooseFirstPresent returns the first candidate column present in cols, or "".
func gooseFirstPresent(cols map[string]bool, candidates []string) string {
	for _, c := range candidates {
		if cols[strings.ToLower(c)] {
			return c
		}
	}
	return ""
}

// gooseSelectExpr returns a quoted column reference, or NULL when the column is
// absent so the positional Scan layout stays fixed.
func gooseSelectExpr(col string) string {
	if col == "" {
		return "NULL"
	}
	return `"` + strings.ReplaceAll(col, `"`, `""`) + `"`
}

func gooseInt(v sql.NullInt64) int {
	if !v.Valid {
		return 0
	}
	return int(v.Int64)
}

func gooseStr(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

// gooseFirstPtr returns the first non-nil pointer's value, or 0.
func gooseFirstPtr(ptrs ...*int) int {
	for _, p := range ptrs {
		if p != nil {
			return *p
		}
	}
	return 0
}
