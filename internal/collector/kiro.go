package collector

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

const agentKiro = "kiro"

// Kiro keeps usage in TWO places, both scanned here and unified by usage.Dedup:
//
//   - ~/.kiro/sessions/cli/**/*.jsonl  — per-session CLI transcripts (JSONL).
//   - ~/.local/share/kiro-cli/data.sqlite3 — a messages table with usage.
//
// SCHEMA-ONLY: written against Kiro's documented/known shapes; NOT verified
// against real Kiro data on this host. The JSONL parser tolerates flat or
// nested ("usage") token objects; the SQLite parser probes columns defensively.

// kiroDBNames are the SQLite filenames Kiro CLI is documented to use.
var kiroDBNames = []string{"data.sqlite3", "data.sqlite", "kiro.db"}

// kiroMessageTables are the table names Kiro CLI's schema may use.
var kiroMessageTables = []string{"messages", "message", "history", "conversation_messages"}

var (
	kiroInputCols     = []string{"input_tokens", "prompt_tokens", "input_token_count"}
	kiroOutputCols    = []string{"output_tokens", "completion_tokens", "output_token_count"}
	kiroCacheReadCols = []string{"cache_read_tokens", "cache_read_input_tokens", "cached_tokens", "cache_tokens"}
	kiroCacheWrCols   = []string{"cache_creation_tokens", "cache_write_tokens", "cache_creation_input_tokens"}
	kiroReasonCols    = []string{"reasoning_tokens", "reasoning_token_count"}
	kiroModelCols     = []string{"model", "model_name", "model_id"}
	kiroSessCols      = []string{"session_id", "session", "conversation_id"}
	kiroTimeCols      = []string{"created_at", "created", "timestamp", "created_ts", "ts"}
	kiroUsageJSONCols = []string{"usage", "metadata", "meta", "data"}
)

// kiroLine is the subset of a Kiro CLI session JSONL record we read. Kiro writes
// one JSON object per line; only lines carrying a usage block are emitted. The
// usage block follows the Anthropic shape (input inclusive of cache-read).
type kiroLine struct {
	Type      string     `json:"type"`
	Role      string     `json:"role"`
	Timestamp string     `json:"timestamp"`
	SessionID string     `json:"session_id"`
	MessageID string     `json:"message_id"`
	Model     string     `json:"model"`
	Usage     *kiroUsage `json:"usage"`
	// Some writers nest the model/usage inside a "message" object (Anthropic
	// SDK echo). Honored as a fallback.
	Message *struct {
		ID    string     `json:"id"`
		Model string     `json:"model"`
		Usage *kiroUsage `json:"usage"`
	} `json:"message"`
}

// kiroUsage is the Anthropic-shaped token object.
type kiroUsage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	CacheCreation   int `json:"cache_creation_input_tokens"`
	CacheReadTokens int `json:"cache_read_input_tokens"`
	ReasoningTokens int `json:"reasoning_tokens"`
}

// scanKiro implements the Adapter signature for Kiro, scanning both the CLI
// JSONL transcripts and the SQLite message store and returning the deduped set.
func scanKiro(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	if state == nil {
		state = NewState()
	}
	var (
		events []usage.Event
		report ScanReport
	)
	for _, base := range baseDirs {
		// JSONL source: ~/.kiro/sessions/cli/**/*.jsonl
		files, err := kiroJSONLFiles(base)
		if err != nil {
			report.Errors++
		}
		for _, path := range files {
			report.FilesScanned++
			kiroScanJSONLFile(path, state, &events, &report)
		}
		// SQLite source: ~/.local/share/kiro-cli/data.sqlite3
		dbs, err := kiroDBs(base)
		if err != nil {
			report.Errors++
		}
		for _, path := range dbs {
			report.FilesScanned++
			kiroScanDB(path, state, &events, &report)
		}
	}
	return usage.Dedup(events), report, nil
}

// kiroJSONLFiles returns *.jsonl files under base (recursively). A base that is
// itself a *.jsonl file is used directly. A missing base yields no files.
func kiroJSONLFiles(base string) ([]string, error) {
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

// kiroDBs returns the Kiro CLI SQLite DB paths under base. A base that is itself
// a DB file is used directly; a *.jsonl base yields no DBs (it is the JSONL
// source). A missing base yields no paths.
func kiroDBs(base string) ([]string, error) {
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
	for _, name := range kiroDBNames {
		p := filepath.Join(base, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// kiroScanJSONLFile reads the unconsumed tail of one JSONL file, appending
// events and updating report + state.
func kiroScanJSONLFile(path string, state *State, events *[]usage.Event, report *ScanReport) {
	defaultSession := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	scanJSONLIncremental(path, state, report, func(line []byte, _ int) {
		report.LinesParsed++
		if ev, ok, perr := kiroParseLine(line, defaultSession); perr != nil {
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

// kiroParseLine parses one JSONL line. ok=false means a valid non-usage line
// (skip, not an error). err!=nil means malformed JSON.
func kiroParseLine(line []byte, defaultSession string) (usage.Event, bool, error) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return usage.Event{}, false, nil
	}
	var kl kiroLine
	if err := json.Unmarshal([]byte(trimmed), &kl); err != nil {
		return usage.Event{}, false, err
	}

	u := kl.Usage
	model := kl.Model
	msgID := kl.MessageID
	if u == nil && kl.Message != nil {
		u = kl.Message.Usage
		if model == "" {
			model = kl.Message.Model
		}
		if msgID == "" {
			msgID = kl.Message.ID
		}
	}
	if u == nil {
		return usage.Event{}, false, nil // non-usage line
	}

	// Anthropic shape: input_tokens is inclusive of cache-read; subtract so it
	// is not counted twice.
	input := u.InputTokens - u.CacheReadTokens
	if input < 0 {
		input = 0
	}

	session := kl.SessionID
	if session == "" {
		session = defaultSession
	}

	ts, tzMin := normalizeTimestamp(kl.Timestamp)

	ev := usage.Event{
		Agent:               agentKiro,
		MessageID:           msgID,
		SessionID:           session,
		Model:               model,
		InputTokens:         input,
		OutputTokens:        u.OutputTokens,
		CacheReadTokens:     u.CacheReadTokens,
		CacheCreate5mTokens: u.CacheCreation,
		ReasoningTokens:     u.ReasoningTokens,
		Timestamp:           ts,
		TZOffsetMinutes:     tzMin,
		BillingType:         usage.BillingAPI,
	}

	if !ev.HasUsage() {
		return usage.Event{}, false, nil
	}
	ev.EnsureID()
	return ev, true, nil
}

// kiroScanDB opens one DB read-only, resolves the first usable messages table,
// reads rows past the rowid cursor, and appends events. Never returns an error.
func kiroScanDB(path string, state *State, events *[]usage.Event, report *ScanReport) {
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
	for _, t := range kiroMessageTables {
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

	inputCol := sqliteFirstPresent(cols, kiroInputCols)
	outputCol := sqliteFirstPresent(cols, kiroOutputCols)
	cacheReadCol := sqliteFirstPresent(cols, kiroCacheReadCols)
	cacheWrCol := sqliteFirstPresent(cols, kiroCacheWrCols)
	reasonCol := sqliteFirstPresent(cols, kiroReasonCols)
	modelCol := sqliteFirstPresent(cols, kiroModelCols)
	sessCol := sqliteFirstPresent(cols, kiroSessCols)
	timeCol := sqliteFirstPresent(cols, kiroTimeCols)
	jsonCol := sqliteFirstPresent(cols, kiroUsageJSONCols)

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

		ev, ok := kiroBuildDBEvent(inTok, outTok, cReadTok, cWriteTok, reasonTok, model, sess, tsRaw, jsonBlob)
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

// kiroBuildDBEvent maps one scanned SQLite row to a usage.Event. ok=false means
// the row carries no usage and should be skipped (not an error).
func kiroBuildDBEvent(inTok, outTok, cReadTok, cWriteTok, reasonTok sql.NullInt64, model, sess, tsRaw, jsonBlob sql.NullString) (usage.Event, bool) {
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
		Agent:               agentKiro,
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
