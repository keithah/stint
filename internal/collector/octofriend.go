package collector

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

const agentOctofriend = "octofriend"

// octofriendDBNames are the SQLite database filenames Octofriend is documented
// to use for its message store (~/.local/share/octofriend/sqlite.db). The first
// match under a base dir is scanned.
var octofriendDBNames = []string{"sqlite.db", "octofriend.db"}

// octofriendMessageTables are the table names Octofriend's schema is documented
// to (or may) use for per-message usage. The adapter probes each in turn.
var octofriendMessageTables = []string{"messages", "message", "history"}

// Column-name spellings Octofriend's schema is documented to (or may) use. The
// adapter probes the actual table columns and binds whichever spelling is
// present, so a naming drift degrades a field to 0 rather than failing.
var (
	octofriendInputCols     = []string{"input_tokens", "prompt_tokens", "input_token_count"}
	octofriendOutputCols    = []string{"output_tokens", "completion_tokens", "output_token_count"}
	octofriendCacheReadCols = []string{"cache_read_tokens", "cache_read_input_tokens", "cached_tokens", "cache_tokens"}
	octofriendCacheWrCols   = []string{"cache_creation_tokens", "cache_write_tokens", "cache_creation_input_tokens"}
	octofriendReasonCols    = []string{"reasoning_tokens", "reasoning_token_count"}
	octofriendModelCols     = []string{"model", "model_name", "model_id"}
	octofriendSessCols      = []string{"session_id", "session", "conversation_id", "thread_id"}
	octofriendTimeCols      = []string{"created_at", "created", "timestamp", "created_ts", "ts"}
	octofriendUsageJSONCols = []string{"usage", "metadata", "meta", "data"}
)

// octofriendUsageMeta is the subset of a per-message JSON usage/metadata blob
// the adapter reads when Octofriend stores tokens inside a JSON column rather
// than dedicated columns. Both a flat shape and a nested "usage" object are
// accepted; dedicated columns take precedence when both are present.
type octofriendUsageMeta struct {
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

// scanOctofriend implements the Adapter signature for Octofriend. It locates the
// Octofriend SQLite DB under the base dirs, reads message rows past the recorded
// max-rowid cursor, maps per-message token usage to events, and returns the
// deduped set. A single bad row never aborts the scan; anomalies are counted.
//
// SCHEMA-ONLY: written against Octofriend's documented SQLite schema; NOT
// verified against a real ~/.local/share/octofriend/sqlite.db on this host.
// Defensive column probing (PRAGMA table_info) hedges against naming drift.
func scanOctofriend(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	if state == nil {
		state = NewState()
	}
	var (
		events []usage.Event
		report ScanReport
	)
	for _, base := range baseDirs {
		dbs, err := octofriendDBs(base)
		if err != nil {
			report.Errors++
			continue
		}
		for _, path := range dbs {
			report.FilesScanned++
			octofriendScanDB(path, state, &events, &report)
		}
	}
	return usage.Dedup(events), report, nil
}

// octofriendDBs returns the Octofriend DB paths under base. A base that is
// itself a *.db/*.sqlite file is used directly. A missing base yields no paths.
func octofriendDBs(base string) ([]string, error) {
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
	for _, name := range octofriendDBNames {
		p := filepath.Join(base, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// octofriendScanDB opens one DB read-only, resolves the first usable messages
// table and its column layout, reads rows past the rowid cursor, and appends
// events. It never returns an error; failures are counted in the report.
func octofriendScanDB(path string, state *State, events *[]usage.Event, report *ScanReport) {
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

	table, cols := octofriendResolveTable(db)
	if table == "" {
		report.Errors++
		return
	}

	inputCol := sqliteFirstPresent(cols, octofriendInputCols)
	outputCol := sqliteFirstPresent(cols, octofriendOutputCols)
	cacheReadCol := sqliteFirstPresent(cols, octofriendCacheReadCols)
	cacheWrCol := sqliteFirstPresent(cols, octofriendCacheWrCols)
	reasonCol := sqliteFirstPresent(cols, octofriendReasonCols)
	modelCol := sqliteFirstPresent(cols, octofriendModelCols)
	sessCol := sqliteFirstPresent(cols, octofriendSessCols)
	timeCol := sqliteFirstPresent(cols, octofriendTimeCols)
	jsonCol := sqliteFirstPresent(cols, octofriendUsageJSONCols)

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

		ev, ok := octofriendBuildEvent(inTok, outTok, cReadTok, cWriteTok, reasonTok, model, sess, tsRaw, jsonBlob)
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

// octofriendResolveTable returns the first documented messages table that exists
// and has columns, plus its column set. ("", nil) means no usable table.
func octofriendResolveTable(db *sql.DB) (string, map[string]bool) {
	for _, t := range octofriendMessageTables {
		cols, err := sqliteTableColumns(db, t)
		if err == nil && len(cols) > 0 {
			return t, cols
		}
	}
	return "", nil
}

// octofriendBuildEvent maps one scanned row to a usage.Event. ok=false means the
// row carries no usage and should be skipped (not an error). Dedicated columns
// take precedence; the JSON blob fills any field left at zero.
func octofriendBuildEvent(inTok, outTok, cReadTok, cWriteTok, reasonTok sql.NullInt64, model, sess, tsRaw, jsonBlob sql.NullString) (usage.Event, bool) {
	input := sqliteInt(inTok)
	output := sqliteInt(outTok)
	cacheRead := sqliteInt(cReadTok)
	cacheWrite := sqliteInt(cWriteTok)
	reasoning := sqliteInt(reasonTok)
	modelName := sqliteStr(model)

	if jsonBlob.Valid && strings.TrimSpace(jsonBlob.String) != "" {
		if mi, mo, mcr, mcw, mr, mm, ok := octofriendParseMeta(jsonBlob.String); ok {
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
		Agent:               agentOctofriend,
		SessionID:           sqliteStr(sess),
		Model:               modelName,
		InputTokens:         input,
		OutputTokens:        output,
		CacheReadTokens:     cacheRead,
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

// octofriendParseMeta extracts token counts and model from a JSON usage blob,
// honoring either a flat shape or a nested "usage" object. ok=false means the
// blob was not valid JSON.
func octofriendParseMeta(s string) (input, output, cacheRead, cacheWrite, reasoning int, model string, ok bool) {
	var m octofriendUsageMeta
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
