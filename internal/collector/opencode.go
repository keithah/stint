package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

const agentOpenCode = "opencode"

// openCodeDBName is the SQLite database file OpenCode keeps under its base dir.
const openCodeDBName = "opencode.db"

// openCodeMessage is the subset of the JSON blob stored in message.data that
// carries usage. OpenCode persists one row per message in the `message` table;
// assistant messages hold a `tokens` object plus model/provider/cwd/cost.
// Non-assistant rows (user/system) lack tokens and are skipped.
type openCodeMessage struct {
	Role       string   `json:"role"`
	ModelID    string   `json:"modelID"`
	ProviderID string   `json:"providerID"`
	Cost       *float64 `json:"cost"`
	Time       struct {
		Created   int64 `json:"created"`
		Completed int64 `json:"completed"`
	} `json:"time"`
	Path struct {
		Cwd  string `json:"cwd"`
		Root string `json:"root"`
	} `json:"path"`
	Tokens *struct {
		Input     int `json:"input"`
		Output    int `json:"output"`
		Reasoning int `json:"reasoning"`
		Cache     struct {
			Read  int `json:"read"`
			Write int `json:"write"`
		} `json:"cache"`
	} `json:"tokens"`
}

// scanOpenCode implements the Adapter for OpenCode. OpenCode is authoritative in
// its SQLite database (~/.local/share/opencode/opencode.db): the `message` table
// holds every message keyed by id, with the usage payload as a JSON blob in the
// `data` column. We read each assistant message's tokens, map them to events,
// and use the message id as MessageID so dedup collapses repeats.
//
// Incremental state is coarse: per DB file we remember the max time_created
// already emitted (in FileState.Lines) plus the DB size (in FileState.Size). A
// rescan only reads rows newer than the watermark, and if the DB shrank we
// restart. eventId dedup is the backstop.
func scanOpenCode(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	if state == nil {
		state = NewState()
	}
	var (
		events []usage.Event
		report ScanReport
	)
	for _, base := range baseDirs {
		dbPath := openCodeDBPath(base)
		if dbPath == "" {
			continue
		}
		openCodeScanDB(dbPath, state, &events, &report)
	}
	return usage.Dedup(events), report, nil
}

// openCodeDBPath resolves the opencode.db path for a base dir. The base may be
// the data dir (we append opencode.db) or the db file itself. A missing file
// yields "".
func openCodeDBPath(base string) string {
	info, err := os.Stat(base)
	if err != nil {
		// base might be a not-yet-existing dir; nothing to scan.
		if os.IsNotExist(err) {
			return ""
		}
		return ""
	}
	if !info.IsDir() {
		if strings.HasSuffix(base, ".db") {
			return base
		}
		return ""
	}
	cand := filepath.Join(base, openCodeDBName)
	if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
		return cand
	}
	return ""
}

// openCodeScanDB opens the DB read-only, reads assistant messages past the
// state watermark, and appends events. It never returns an error; per-row
// failures are counted in the report.
func openCodeScanDB(dbPath string, state *State, events *[]usage.Event, report *ScanReport) {
	info, err := os.Stat(dbPath)
	if err != nil {
		report.Errors++
		return
	}
	report.FilesScanned++

	size := info.Size()
	mtime := info.ModTime().UnixNano()

	// Watermark: the max time_created we have already emitted. If the DB shrank
	// below the recorded size, treat it as reset and start from zero.
	watermark := state.MaxUnixMillis(dbPath, size)

	db, err := openReadOnlySQLite(dbPath)
	if err != nil {
		report.Errors++
		return
	}
	defer db.Close()

	// Strictly-greater bound so a rescan with an unchanged DB emits nothing.
	// Rows that tie the watermark millisecond are the rare boundary case the
	// eventId dedup backstop covers; we never lose rows because the first scan
	// of any timestamp already emitted them.
	rows, err := db.Query(
		`SELECT id, session_id, time_created, data FROM message WHERE time_created > ? ORDER BY time_created`,
		watermark,
	)
	if err != nil {
		report.Errors++
		return
	}
	defer rows.Close()

	maxSeen := watermark
	for rows.Next() {
		var (
			id        string
			sessionID string
			created   int64
			data      string
		)
		if err := rows.Scan(&id, &sessionID, &created, &data); err != nil {
			report.Errors++
			continue
		}
		report.LinesParsed++
		if created > maxSeen {
			maxSeen = created
		}
		ev, ok, perr := parseOpenCodeMessage(id, sessionID, created, []byte(data))
		if perr != nil {
			report.Errors++
			report.LinesSkipped++
			continue
		}
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

	// Commit the watermark. We store maxSeen (millis) and the DB size so a
	// rescan skips rows we have already emitted and detects truncation/reset.
	// Re-using the exact maxSeen as the next lower bound (>=) means rows that
	// share a millisecond timestamp are re-read, but eventId dedup absorbs them.
	state.CommitUnixMillis(dbPath, size, mtime, maxSeen)
}

// parseOpenCodeMessage maps one message row's JSON blob to an event. ok=false
// means it is a valid non-usage row (skip, not an error); err!=nil means the
// blob did not parse.
func parseOpenCodeMessage(id, sessionID string, created int64, data []byte) (usage.Event, bool, error) {
	var m openCodeMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return usage.Event{}, false, err
	}
	if m.Tokens == nil {
		return usage.Event{}, false, nil // user/system or other non-usage row
	}
	tk := m.Tokens

	ev := usage.Event{
		Agent:           agentOpenCode,
		MessageID:       id,
		SessionID:       sessionID,
		Model:           openCodeModel(m.ProviderID, m.ModelID),
		InputTokens:     tk.Input,
		OutputTokens:    tk.Output,
		ReasoningTokens: tk.Reasoning,
		CacheReadTokens: tk.Cache.Read,
		// OpenCode reports a single cache-write count with no 5m/1h split;
		// preserve it in the 5m bucket (matching the Claude lumping convention).
		CacheCreate5mTokens: tk.Cache.Write,
		BillingType:         usage.BillingAPI,
	}

	// Project: cwd basename, else root basename.
	if m.Path.Cwd != "" {
		ev.Project = filepath.Base(m.Path.Cwd)
	} else if m.Path.Root != "" {
		ev.Project = filepath.Base(m.Path.Root)
	}

	// Provider-reported cost, when non-zero.
	if m.Cost != nil && *m.Cost != 0 {
		c := *m.Cost
		ev.CostUSDProvided = &c
	}

	// Timestamp: prefer the message's own created field; fall back to the row's
	// time_created. Both are epoch milliseconds.
	ms := m.Time.Created
	if ms == 0 {
		ms = created
	}
	ev.Timestamp = normalizeUnixMillis(ms)

	if !ev.HasUsage() {
		return usage.Event{}, false, nil
	}

	ev.EnsureID()
	return ev, true, nil
}

// openCodeModel joins provider and model into a stable model identifier. When
// only one is present it returns that; an empty pair yields "".
func openCodeModel(provider, model string) string {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	switch {
	case provider != "" && model != "":
		return provider + "/" + model
	case model != "":
		return model
	default:
		return provider
	}
}
