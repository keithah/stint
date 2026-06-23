package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/keithah/stint/internal/usage"
)

const agentClaude = "claude"

// claudeLine is the subset of a Claude Code transcript JSONL record we read.
// Non-assistant/usage lines simply lack a message.usage block and are skipped.
type claudeLine struct {
	Type      string   `json:"type"`
	Timestamp string   `json:"timestamp"`
	SessionID string   `json:"sessionId"`
	Cwd       string   `json:"cwd"`
	RequestID string   `json:"requestId"`
	CostUSD   *float64 `json:"costUSD"`
	CostUSD2  *float64 `json:"cost_usd"`
	Message   struct {
		ID    string               `json:"id"`
		Model string               `json:"model"`
		Usage *anthropicUsageBlock `json:"usage"`
	} `json:"message"`
}

// scanClaude implements the Adapter for Claude Code transcripts. It walks each
// base dir for *.jsonl files, reads only the unconsumed tail (per State),
// parses each line, maps usage lines to events, and returns the deduped set.
func scanClaude(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	if state == nil {
		state = NewState()
	}
	var (
		events []usage.Event
		report ScanReport
	)
	for _, base := range baseDirs {
		files, err := claudeFiles(base)
		if err != nil {
			report.Errors++
			continue
		}
		for _, path := range files {
			report.FilesScanned++
			scanClaudeFile(path, base, state, &events, &report)
		}
	}
	// Claude writes the same message+request several times as streaming usage
	// grows; usage.Dedup keeps the row with the largest OutputTokens, so the
	// final complete count wins. This is handled generically in usage.Dedup for
	// every id-keyed agent, not just Claude.
	return usage.Dedup(events), report, nil
}

// claudeFiles returns all *.jsonl files under base (recursively). A missing
// base dir yields no files and no error.
func claudeFiles(base string) ([]string, error) {
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
	err = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // tolerate unreadable subtrees
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".jsonl") {
			files = append(files, p)
		}
		return nil
	})
	return files, err
}

// scanClaudeFile reads the unconsumed portion of one file, appending events and
// updating report + state. It never returns an error; bad lines are counted.
func scanClaudeFile(path, base string, state *State, events *[]usage.Event, report *ScanReport) {
	defaultSession := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	pathProject := claudeProjectFromPath(path, base)

	scanJSONLIncremental(path, state, report, func(line []byte, _ int) {
		report.LinesParsed++
		if ev, ok, perr := parseClaudeLine(line, defaultSession, pathProject); perr != nil {
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

// parseClaudeLine parses one JSONL line. ok=false means it is a valid line with
// no usage (skip, not an error). err!=nil means malformed JSON.
func parseClaudeLine(line []byte, defaultSession, pathProject string) (usage.Event, bool, error) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return usage.Event{}, false, nil
	}
	var cl claudeLine
	if err := json.Unmarshal([]byte(trimmed), &cl); err != nil {
		return usage.Event{}, false, err
	}
	if cl.Message.Usage == nil {
		return usage.Event{}, false, nil // non-usage line
	}

	ev := usage.Event{
		Agent:       agentClaude,
		MessageID:   cl.Message.ID,
		RequestID:   cl.RequestID,
		Model:       cl.Message.Model,
		BillingType: usage.BillingSubscription,
	}
	cl.Message.Usage.canonical().apply(&ev)

	// Session: explicit field, else file basename.
	ev.SessionID = cl.SessionID
	if ev.SessionID == "" {
		ev.SessionID = defaultSession
	}

	// Project: cwd basename, else the encoded project dir from the path.
	if cl.Cwd != "" {
		ev.Project = filepath.Base(cl.Cwd)
	} else {
		ev.Project = pathProject
	}

	// Timestamp -> RFC3339 UTC, preserving original offset minutes.
	ts, tzMin := normalizeTimestamp(cl.Timestamp)
	ev.Timestamp = ts
	ev.TZOffsetMinutes = tzMin

	// Provider-reported cost (rare in transcripts).
	if cl.CostUSD != nil {
		ev.CostUSDProvided = cl.CostUSD
	} else if cl.CostUSD2 != nil {
		ev.CostUSDProvided = cl.CostUSD2
	}

	// A usage block with all zeros and no cost is not a real usage event.
	if !ev.HasUsage() {
		return usage.Event{}, false, nil
	}

	ev.EnsureID()
	return ev, true, nil
}

// claudeProjectFromPath derives a project name from the encoded project dir in
// ~/.claude/projects/<encoded>/<session>.jsonl, relative to base. Falls back to
// the immediate parent dir name.
func claudeProjectFromPath(path, base string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return filepath.Base(filepath.Dir(path))
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) >= 2 {
		return parts[0]
	}
	return filepath.Base(filepath.Dir(path))
}

// normalizeTimestamp returns an RFC3339 UTC timestamp and the original offset
// in minutes. A 'Z'/UTC source yields offset 0. Unparseable input is returned
// verbatim with offset 0.
func normalizeTimestamp(ts string) (string, int) {
	if ts == "" {
		return "", 0
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		// Try RFC3339 with nanoseconds explicitly (time.RFC3339 already
		// handles fractional seconds, but be defensive).
		t, err = time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return ts, 0
		}
	}
	_, offsetSec := t.Zone()
	return t.UTC().Format(time.RFC3339), offsetSec / 60
}

// normalizeUnixMillis formats a Unix epoch in milliseconds as an RFC3339 UTC
// timestamp. A non-positive value yields "" (no timestamp).
func normalizeUnixMillis(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

// normalizeUnixNanos formats a Unix epoch in nanoseconds as an RFC3339 UTC
// timestamp. A non-positive value yields "" (no timestamp).
func normalizeUnixNanos(ns int64) string {
	if ns <= 0 {
		return ""
	}
	return time.Unix(0, ns).UTC().Format(time.RFC3339)
}
