package collector

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

const agentKilo = "kilo"

// Kilo keeps usage in TWO places, both scanned here and unified by usage.Dedup:
//
//   - ~/.local/share/kilo/kilo.db — a messages table with per-message usage.
//   - VS Code globalStorage kilocode.kilo-code/tasks/**/*.jsonl — per-task
//     transcripts whose assistant lines carry an Anthropic-shaped usage block
//     (the same shape Cline/Roo write, since Kilo forks from them).
//
// SCHEMA-ONLY: written against Kilo's documented/known shapes; NOT verified
// against real Kilo data on this host. The JSONL parser tolerates flat or
// nested ("message".usage) token objects; the SQLite parser probes columns
// defensively (PRAGMA table_info).

// kiloDBNames are the SQLite filenames Kilo is documented to use.
var kiloDBNames = []string{"kilo.db", "kilo.sqlite", "kilo.sqlite3"}

// kiloMessageTables are the table names Kilo's schema may use.
var kiloMessageTables = []string{"messages", "message", "history"}

var (
	kiloInputCols     = []string{"input_tokens", "prompt_tokens", "input_token_count"}
	kiloOutputCols    = []string{"output_tokens", "completion_tokens", "output_token_count"}
	kiloCacheReadCols = []string{"cache_read_tokens", "cache_read_input_tokens", "cached_tokens", "cache_tokens"}
	kiloCacheWrCols   = []string{"cache_creation_tokens", "cache_write_tokens", "cache_creation_input_tokens"}
	kiloReasonCols    = []string{"reasoning_tokens", "reasoning_token_count"}
	kiloModelCols     = []string{"model", "model_name", "model_id"}
	kiloSessCols      = []string{"session_id", "session", "conversation_id", "task_id"}
	kiloTimeCols      = []string{"created_at", "created", "timestamp", "created_ts", "ts"}
	kiloUsageJSONCols = []string{"usage", "metadata", "meta", "data"}
)

// kiloLine is the subset of a Kilo VS Code task JSONL record we read. The
// assistant lines follow the Anthropic SDK response shape: a "message" object
// with "model" and "usage". A flat top-level "usage" is also honored.
type kiloLine struct {
	Type      string               `json:"type"`
	Role      string               `json:"role"`
	Timestamp string               `json:"timestamp"`
	Ts        int64                `json:"ts"`
	SessionID string               `json:"taskId"`
	Usage     *anthropicUsageBlock `json:"usage"`
	Message   *struct {
		ID    string               `json:"id"`
		Model string               `json:"model"`
		Usage *anthropicUsageBlock `json:"usage"`
	} `json:"message"`
}

// scanKilo implements the Adapter signature for Kilo, scanning both the SQLite
// message store and the VS Code task JSONL transcripts, returning the deduped
// set.
func scanKilo(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	if state == nil {
		state = NewState()
	}
	var (
		events []usage.Event
		report ScanReport
	)
	for _, base := range baseDirs {
		// SQLite source: ~/.local/share/kilo/kilo.db
		dbs, err := kiloDBs(base)
		if err != nil {
			report.Errors++
		}
		for _, path := range dbs {
			report.FilesScanned++
			kiloScanDB(path, state, &events, &report)
		}
		// JSONL source: kilocode.kilo-code/tasks/**/*.jsonl
		files, err := kiloJSONLFiles(base)
		if err != nil {
			report.Errors++
		}
		for _, path := range files {
			report.FilesScanned++
			kiloScanJSONLFile(path, state, &events, &report)
		}
	}
	return usage.Dedup(events), report, nil
}

// kiloDBs returns the Kilo SQLite DB paths under base. A base that is itself a
// DB file is used directly. A missing base yields no paths.
func kiloDBs(base string) ([]string, error) {
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
	for _, name := range kiloDBNames {
		p := filepath.Join(base, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// kiloJSONLFiles returns *.jsonl files under base (recursively). A base that is
// itself a *.jsonl file is used directly. A missing base yields no files.
func kiloJSONLFiles(base string) ([]string, error) {
	info, err := os.Stat(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		if strings.HasSuffix(base, ".jsonl") {
			return []string{base}, nil
		}
		return nil, nil
	}
	var files []string
	_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".jsonl") {
			files = append(files, p)
		}
		return nil
	})
	return files, nil
}

// kiloScanJSONLFile reads the unconsumed tail of one JSONL file, appending
// events and updating report + state.
func kiloScanJSONLFile(path string, state *State, events *[]usage.Event, report *ScanReport) {
	// Kilo VS Code tasks live under tasks/<taskId>/<file>.jsonl; the taskId dir
	// is the natural session fallback.
	defaultSession := filepath.Base(filepath.Dir(path))
	scanJSONLIncremental(path, state, report, func(line []byte, _ int) {
		report.LinesParsed++
		if ev, ok, perr := kiloParseLine(line, defaultSession); perr != nil {
			report.Errors++
			report.LinesSkipped++
		} else if ok {
			*events = append(*events, ev)
			report.EventsEmitted++
		} else {
			report.LinesSkipped++
		}
	})
}

// kiloParseLine parses one JSONL line. ok=false means a valid non-usage line
// (skip, not an error). err!=nil means malformed JSON.
func kiloParseLine(line []byte, defaultSession string) (usage.Event, bool, error) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return usage.Event{}, false, nil
	}
	var kl kiloLine
	if err := json.Unmarshal([]byte(trimmed), &kl); err != nil {
		return usage.Event{}, false, err
	}

	u := kl.Usage
	var model, msgID string
	if kl.Message != nil {
		model = kl.Message.Model
		msgID = kl.Message.ID
		if u == nil {
			u = kl.Message.Usage
		}
	}
	if u == nil {
		return usage.Event{}, false, nil // non-usage line
	}

	// Anthropic shape: Kilo's input_tokens is inclusive of cache-read; subtract
	// so it is not counted twice (Kilo deviates from the standard Anthropic
	// convention that input already excludes cache).
	tu := u.canonical()
	tu.Input -= tu.CacheRead
	if tu.Input < 0 {
		tu.Input = 0
	}

	session := kl.SessionID
	if session == "" {
		session = defaultSession
	}

	// Timestamp: prefer RFC3339 string; else epoch-millis "ts".
	ts, tzMin := normalizeTimestamp(kl.Timestamp)
	if ts == "" && kl.Ts > 0 {
		ts = normalizeUnixMillis(kl.Ts)
	}

	ev := usage.Event{
		Agent:           agentKilo,
		MessageID:       msgID,
		SessionID:       session,
		Model:           model,
		Timestamp:       ts,
		TZOffsetMinutes: tzMin,
		BillingType:     usage.BillingAPI,
	}
	tu.apply(&ev)

	if !ev.HasUsage() {
		return usage.Event{}, false, nil
	}
	ev.EnsureID()
	return ev, true, nil
}

// kiloScanDB opens one DB read-only, resolves the first usable messages table,
// reads rows past the rowid cursor, and appends events. Never returns an error.
func kiloScanDB(path string, state *State, events *[]usage.Event, report *ScanReport) {
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

	var (
		table string
		cols  map[string]bool
	)
	for _, t := range kiloMessageTables {
		c, err := sqliteTableColumns(db, t)
		if err == nil && len(c) > 0 {
			table, cols = t, c
			break
		}
	}
	if table == "" {
		report.Errors++
		return
	}

	inputCol := sqliteFirstPresent(cols, kiloInputCols)
	outputCol := sqliteFirstPresent(cols, kiloOutputCols)
	cacheReadCol := sqliteFirstPresent(cols, kiloCacheReadCols)
	cacheWrCol := sqliteFirstPresent(cols, kiloCacheWrCols)
	reasonCol := sqliteFirstPresent(cols, kiloReasonCols)
	modelCol := sqliteFirstPresent(cols, kiloModelCols)
	sessCol := sqliteFirstPresent(cols, kiloSessCols)
	timeCol := sqliteFirstPresent(cols, kiloTimeCols)
	jsonCol := sqliteFirstPresent(cols, kiloUsageJSONCols)

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

		ev, ok := kiloBuildDBEvent(inTok, outTok, cReadTok, cWriteTok, reasonTok, model, sess, tsRaw, jsonBlob)
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

// kiloBuildDBEvent maps one scanned SQLite row to a usage.Event. ok=false means
// the row carries no usage and should be skipped (not an error).
func kiloBuildDBEvent(inTok, outTok, cReadTok, cWriteTok, reasonTok sql.NullInt64, model, sess, tsRaw, jsonBlob sql.NullString) (usage.Event, bool) {
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
		Agent:           agentKilo,
		SessionID:       sqliteStr(sess),
		Model:           modelName,
		Timestamp:       ts,
		TZOffsetMinutes: tzMin,
		BillingType:     usage.BillingAPI,
	}
	tokenUsage{
		Input:         input,
		Output:        output,
		CacheRead:     cacheRead,
		CacheCreate5m: cacheWrite,
		Reasoning:     reasoning,
	}.apply(&ev)

	if !ev.HasUsage() {
		return usage.Event{}, false
	}
	ev.EnsureID()
	return ev, true
}
