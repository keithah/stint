package collector

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

const agentHermes = "hermes"

// hermesDBNames are the SQLite database filenames Hermes is documented to use
// for its state/message store (~/.hermes/state.db). The first match under a
// base dir is scanned.
var hermesDBNames = []string{"state.db"}

// hermesMessageTables are the table names Hermes' schema is documented to (or
// may) use for per-message usage. The adapter probes each in turn and scans the
// first that exists and carries usable columns.
var hermesMessageTables = []string{"messages", "events"}

// Column-name spellings Hermes' schema is documented to (or may) use for each
// mapped field. The adapter probes the actual table columns (PRAGMA table_info)
// and binds whichever spelling is present, so a naming drift degrades a field
// to 0 rather than failing the scan.
var (
	hermesInputCols     = []string{"input_tokens", "prompt_tokens", "input_token_count"}
	hermesOutputCols    = []string{"output_tokens", "completion_tokens", "output_token_count"}
	hermesCacheReadCols = []string{"cache_read_tokens", "cache_read_input_tokens", "cached_tokens", "cache_tokens"}
	hermesCacheWrCols   = []string{"cache_creation_tokens", "cache_write_tokens", "cache_creation_input_tokens"}
	hermesReasonCols    = []string{"reasoning_tokens", "reasoning_token_count"}
	hermesModelCols     = []string{"model", "model_name", "model_id"}
	hermesSessCols      = []string{"session_id", "session", "conversation_id", "chat_id"}
	hermesTimeCols      = []string{"created_at", "created", "timestamp", "created_ts", "ts"}
	hermesUsageJSONCols = []string{"usage", "metadata", "meta", "data"}
)

// hermesUsageMeta is the subset of a per-message JSON usage/metadata blob the
// adapter reads. Hermes may carry token usage inside a JSON column rather than
// dedicated columns; either source is honored, with dedicated columns taking
// precedence when both are present. Both a flat shape and a nested "usage"
// object are accepted.
type hermesUsageMeta struct {
	InputTokens   *int   `json:"input_tokens"`
	PromptTokens  *int   `json:"prompt_tokens"`
	OutputTokens  *int   `json:"output_tokens"`
	Completion    *int   `json:"completion_tokens"`
	CacheRead     *int   `json:"cache_read_tokens"`
	CacheRead2    *int   `json:"cache_read_input_tokens"`
	CachedTokens  *int   `json:"cached_tokens"`
	CacheCreate   *int   `json:"cache_creation_tokens"`
	CacheCreate2  *int   `json:"cache_creation_input_tokens"`
	ReasoningToks *int   `json:"reasoning_tokens"`
	Model         string `json:"model"`
	Usage         *struct {
		InputTokens   *int `json:"input_tokens"`
		PromptTokens  *int `json:"prompt_tokens"`
		OutputTokens  *int `json:"output_tokens"`
		Completion    *int `json:"completion_tokens"`
		CacheRead     *int `json:"cache_read_tokens"`
		CacheRead2    *int `json:"cache_read_input_tokens"`
		CachedTokens  *int `json:"cached_tokens"`
		CacheCreate   *int `json:"cache_creation_tokens"`
		CacheCreate2  *int `json:"cache_creation_input_tokens"`
		ReasoningToks *int `json:"reasoning_tokens"`
	} `json:"usage"`
}

// scanHermes implements the Adapter signature for Hermes. It locates each Hermes
// state SQLite DB under the base dirs, reads message rows past the recorded
// max-rowid cursor, maps per-message token usage to events, and returns the
// deduped set. A single bad row never aborts the scan; anomalies are counted.
//
// SCHEMA-ONLY: written against Hermes' documented SQLite schema; NOT verified
// against a real ~/.hermes/state.db on this host. The defensive column probing
// (PRAGMA table_info) is the hedge against naming drift.
func scanHermes(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	if state == nil {
		state = NewState()
	}
	var (
		events []usage.Event
		report ScanReport
	)
	for _, base := range baseDirs {
		dbs, err := hermesDBs(base)
		if err != nil {
			report.Errors++
			continue
		}
		for _, path := range dbs {
			report.FilesScanned++
			hermesScanDB(path, state, &events, &report)
		}
	}
	return usage.Dedup(events), report, nil
}

// hermesDBs returns the Hermes state DB paths under base. A base that is itself
// a *.db/*.sqlite file is used directly. A missing base yields no paths.
func hermesDBs(base string) ([]string, error) {
	info, err := os.Stat(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		if hermesIsDBFile(base) {
			return []string{base}, nil
		}
		return nil, nil
	}
	var paths []string
	for _, name := range hermesDBNames {
		p := filepath.Join(base, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

func hermesIsDBFile(p string) bool {
	return strings.HasSuffix(p, ".db") || strings.HasSuffix(p, ".sqlite") || strings.HasSuffix(p, ".sqlite3")
}

// hermesScanDB opens one DB read-only, resolves the first usable messages table
// and its column layout, reads rows past the recorded rowid cursor, and appends
// events. It never returns an error; failures are counted in the report.
func hermesScanDB(path string, state *State, events *[]usage.Event, report *ScanReport) {
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

	table, cols := hermesResolveTable(db)
	if table == "" {
		report.Errors++
		return
	}

	inputCol := sqliteFirstPresent(cols, hermesInputCols)
	outputCol := sqliteFirstPresent(cols, hermesOutputCols)
	cacheReadCol := sqliteFirstPresent(cols, hermesCacheReadCols)
	cacheWrCol := sqliteFirstPresent(cols, hermesCacheWrCols)
	reasonCol := sqliteFirstPresent(cols, hermesReasonCols)
	modelCol := sqliteFirstPresent(cols, hermesModelCols)
	sessCol := sqliteFirstPresent(cols, hermesSessCols)
	timeCol := sqliteFirstPresent(cols, hermesTimeCols)
	jsonCol := sqliteFirstPresent(cols, hermesUsageJSONCols)

	sel := []string{"rowid"}
	for _, c := range []string{inputCol, outputCol, cacheReadCol, cacheWrCol, reasonCol, modelCol, sessCol, timeCol, jsonCol} {
		sel = append(sel, sqliteSelectExpr(c))
	}

	cursor := state.Rowid(path)
	query := "SELECT " + strings.Join(sel, ", ") +
		" FROM " + sqliteQuoteIdent(table) + " WHERE rowid > ? ORDER BY rowid ASC"
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
			rowID     int64
			inTok     sql.NullInt64
			outTok    sql.NullInt64
			cReadTok  sql.NullInt64
			cWriteTok sql.NullInt64
			reasonTok sql.NullInt64
			model     sql.NullString
			sess      sql.NullString
			tsRaw     sql.NullString
			jsonBlob  sql.NullString
		)
		if err := rows.Scan(&rowID, &inTok, &outTok, &cReadTok, &cWriteTok, &reasonTok, &model, &sess, &tsRaw, &jsonBlob); err != nil {
			report.Errors++
			report.LinesSkipped++
			continue
		}
		report.LinesParsed++
		lineCount++
		if rowID > maxRowID {
			maxRowID = rowID
		}

		ev, ok := hermesBuildEvent(inTok, outTok, cReadTok, cWriteTok, reasonTok, model, sess, tsRaw, jsonBlob)
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

// hermesResolveTable returns the first documented messages table that exists and
// has columns, plus its column set. ("", nil) means no usable table.
func hermesResolveTable(db *sql.DB) (string, map[string]bool) {
	for _, t := range hermesMessageTables {
		cols, err := sqliteTableColumns(db, t)
		if err == nil && len(cols) > 0 {
			return t, cols
		}
	}
	return "", nil
}

// hermesBuildEvent maps one scanned row to a usage.Event. ok=false means the row
// carries no usage and should be skipped (not an error). Dedicated columns take
// precedence over the JSON blob; the blob fills any field left at zero.
func hermesBuildEvent(inTok, outTok, cReadTok, cWriteTok, reasonTok sql.NullInt64, model, sess, tsRaw, jsonBlob sql.NullString) (usage.Event, bool) {
	input := sqliteInt(inTok)
	output := sqliteInt(outTok)
	cacheRead := sqliteInt(cReadTok)
	cacheWrite := sqliteInt(cWriteTok)
	reasoning := sqliteInt(reasonTok)
	modelName := sqliteStr(model)

	if jsonBlob.Valid && strings.TrimSpace(jsonBlob.String) != "" {
		if mi, mo, mcr, mcw, mr, mm, ok := hermesParseMeta(jsonBlob.String); ok {
			if input == 0 {
				input = mi
			}
			if output == 0 {
				output = mo
			}
			if cacheRead == 0 {
				cacheRead = mcr
			}
			if cacheWrite == 0 {
				cacheWrite = mcw
			}
			if reasoning == 0 {
				reasoning = mr
			}
			if modelName == "" {
				modelName = mm
			}
		}
	}

	ts, tzMin := normalizeTimestamp(sqliteStr(tsRaw))

	ev := usage.Event{
		Agent:           agentHermes,
		SessionID:       sqliteStr(sess),
		Model:           modelName,
		InputTokens:     input,
		OutputTokens:    output,
		CacheReadTokens: cacheRead,
		// Single cache-creation count with no 5m/1h split; preserve in the 5m
		// bucket (matching the Claude lumping convention).
		CacheCreate5mTokens: cacheWrite,
		ReasoningTokens:     reasoning,
		Timestamp:           ts,
		TZOffsetMinutes:     tzMin,
		BillingType:         usage.BillingAPI,
	}

	if !ev.HasUsage() {
		return usage.Event{}, false
	}
	ev.EnsureID()
	return ev, true
}

// hermesParseMeta extracts token counts and model from a JSON usage/metadata
// blob, honoring either a flat shape or a nested "usage" object. ok=false means
// the blob was not valid JSON.
func hermesParseMeta(s string) (input, output, cacheRead, cacheWrite, reasoning int, model string, ok bool) {
	var m hermesUsageMeta
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return 0, 0, 0, 0, 0, "", false
	}
	input = sqliteFirstPtr(m.InputTokens, m.PromptTokens)
	output = sqliteFirstPtr(m.OutputTokens, m.Completion)
	cacheRead = sqliteFirstPtr(m.CacheRead, m.CacheRead2, m.CachedTokens)
	cacheWrite = sqliteFirstPtr(m.CacheCreate, m.CacheCreate2)
	reasoning = sqliteFirstPtr(m.ReasoningToks)
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
		if cacheWrite == 0 {
			cacheWrite = sqliteFirstPtr(m.Usage.CacheCreate, m.Usage.CacheCreate2)
		}
		if reasoning == 0 {
			reasoning = sqliteFirstPtr(m.Usage.ReasoningToks)
		}
	}
	return input, output, cacheRead, cacheWrite, reasoning, model, true
}

// --- Shared SQLite column helpers ---
//
// These mirror the private goose* helpers (which we may not edit) so the new
// SQLite adapters (hermes/octofriend/kiro/kilo) share one defensive
// column-probing implementation. They live here because hermes is the first of
// the new SQLite adapters alphabetically.

// sqliteTableColumns returns the lower-cased column names of a table via PRAGMA
// table_info. A missing table yields an empty set (no error from the PRAGMA
// itself), which callers treat as "no usable table".
func sqliteTableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query("PRAGMA table_info(" + sqliteQuoteIdent(table) + ")")
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

// sqliteFirstPresent returns the first candidate column present in cols, or "".
func sqliteFirstPresent(cols map[string]bool, candidates []string) string {
	for _, c := range candidates {
		if cols[strings.ToLower(c)] {
			return c
		}
	}
	return ""
}

// sqliteSelectExpr returns a quoted column reference, or NULL when the column is
// absent so the positional Scan layout stays fixed.
func sqliteSelectExpr(col string) string {
	if col == "" {
		return "NULL"
	}
	return sqliteQuoteIdent(col)
}

// sqliteQuoteIdent double-quotes a SQLite identifier, escaping embedded quotes.
func sqliteQuoteIdent(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

func sqliteInt(v sql.NullInt64) int {
	if !v.Valid {
		return 0
	}
	return int(v.Int64)
}

func sqliteStr(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

// sqliteFirstPtr returns the first non-nil pointer's value, or 0.
func sqliteFirstPtr(ptrs ...*int) int {
	for _, p := range ptrs {
		if p != nil {
			return *p
		}
	}
	return 0
}
