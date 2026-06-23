package collector

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

const agentGoose = "goose"

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

	db, err := openReadOnlySQLite(path)
	if err != nil {
		report.Errors++
		return
	}
	defer db.Close()

	cols, err := sqliteTableColumns(db, "messages")
	if err != nil || len(cols) == 0 {
		report.Errors++
		return
	}

	// Resolve which spellings are present.
	inputCol := sqliteFirstPresent(cols, gooseInputCols)
	outputCol := sqliteFirstPresent(cols, gooseOutputCols)
	cacheCol := sqliteFirstPresent(cols, gooseCacheCols)
	modelCol := sqliteFirstPresent(cols, gooseModelCols)
	sessCol := sqliteFirstPresent(cols, gooseSessCols)
	timeCol := sqliteFirstPresent(cols, gooseTimeCols)
	metaCol := sqliteFirstPresent(cols, gooseMetaCols)

	// Build the SELECT list. rowid is always first; absent columns select NULL.
	sel := []string{"rowid"}
	sel = append(sel, sqliteSelectExpr(inputCol))
	sel = append(sel, sqliteSelectExpr(outputCol))
	sel = append(sel, sqliteSelectExpr(cacheCol))
	sel = append(sel, sqliteSelectExpr(modelCol))
	sel = append(sel, sqliteSelectExpr(sessCol))
	sel = append(sel, sqliteSelectExpr(timeCol))
	sel = append(sel, sqliteSelectExpr(metaCol))

	// Coarse incremental cursor: the highest rowid already emitted.
	cursor := state.Rowid(path)

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

	state.CommitRowid(path, info.Size(), info.ModTime().UnixNano(), maxRowID, lineCount)
}

// gooseBuildEvent maps one scanned row to a usage.Event. ok=false means the row
// carries no usage and should be skipped (not an error).
func gooseBuildEvent(rowID int64, inTok, outTok, cTok sql.NullInt64, model, sess, tsRaw, meta sql.NullString) (usage.Event, bool) {
	input := sqliteInt(inTok)
	output := sqliteInt(outTok)
	cacheRead := sqliteInt(cTok)
	modelName := sqliteStr(model)

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

	ts, tzMin := normalizeTimestamp(sqliteStr(tsRaw))

	ev := usage.Event{
		Agent:           agentGoose,
		SessionID:       sqliteStr(sess),
		Model:           modelName,
		Timestamp:       ts,
		TZOffsetMinutes: tzMin,
		BillingType:     usage.BillingAPI,
	}
	tokenUsage{Input: input, Output: output, CacheRead: cacheRead}.apply(&ev)

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
	input = sqliteFirstPtr(m.InputTokens, m.PromptTokens)
	output = sqliteFirstPtr(m.OutputTokens, m.Completion)
	cacheRead = sqliteFirstPtr(m.CacheRead, m.CacheRead2, m.CachedTokens)
	model = m.Model
	if m.Usage != nil {
		if input == 0 {
			input = sqliteFirstPtr(m.Usage.InputTokens, m.Usage.PromptTokens)
		}
		if output == 0 {
			output = sqliteFirstPtr(m.Usage.OutputTokens, m.Usage.Completion)
		}
		if cacheRead == 0 {
			cacheRead = sqliteFirstPtr(m.Usage.CacheRead, m.Usage.CacheRead2, m.Usage.CachedTokens)
		}
	}
	return input, output, cacheRead, model, true
}

// gooseUTCNote: timestamps are normalized to RFC3339 UTC via
// normalizeTimestamp (shared with the Claude adapter).
