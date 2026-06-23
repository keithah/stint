package collector

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

// SCHEMA-ONLY adapter: there is no real Pi Agent host data available, so this is
// implemented to the documented/known on-disk format (generic JSONL with a
// per-message usage block carrying the standard input/output/cache/reasoning
// fields) and verified solely with the bundled fixtures.

const agentPiAgent = "pi-agent"

// piAgentLine is the subset of a Pi Agent session JSONL record we read. Each line
// is one JSON object; usage rows carry a usage block. The block accepts the
// common synonym pairs (input/prompt, output/completion, cache_read/cached,
// reasoning/thoughts) and an optional explicit cache_creation write count plus
// 5m/1h split. Non-usage lines lack the block and are skipped.
type piAgentLine struct {
	Type      string        `json:"type"`
	Timestamp string        `json:"timestamp"`
	SessionID string        `json:"sessionId"`
	Cwd       string        `json:"cwd"`
	Model     string        `json:"model"`
	ID        string        `json:"id"`
	Usage     *piAgentUsage `json:"usage"`
	CostUSD   *float64      `json:"costUSD"`
}

// piAgentUsage is the per-message token block. The first non-zero of each synonym
// pair wins. input is treated as inclusive of cache reads, so InputTokens is
// stored as input-cacheRead. It also carries a cache_creation write count plus an
// optional 5m/1h split, which no shared provider block models, so canonical()
// assembles the tokenUsage directly.
type piAgentUsage struct {
	InputTokens      int `json:"input_tokens"`
	PromptTokens     int `json:"prompt_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens"`
	CachedTokens     int `json:"cached_tokens"`
	CacheWriteTokens int `json:"cache_creation_tokens"`
	CacheCreate5m    int `json:"cache_creation_5m_tokens"`
	CacheCreate1h    int `json:"cache_creation_1h_tokens"`
	ReasoningTokens  int `json:"reasoning_tokens"`
	ThoughtsTokens   int `json:"thoughts_tokens"`
}

// canonical resolves the synonym pairs and applies the pi-agent conventions:
// input is inclusive of cache reads (subtract them), and the cache-creation
// count goes to the explicit 5m/1h split when present, else lumps into 5m.
func (u piAgentUsage) canonical() tokenUsage {
	cacheRead := piAgentFirst(u.CacheReadTokens, u.CachedTokens)
	input := piAgentFirst(u.InputTokens, u.PromptTokens) - cacheRead
	if input < 0 {
		input = 0
	}
	t := tokenUsage{
		Input:     input,
		Output:    piAgentFirst(u.OutputTokens, u.CompletionTokens),
		CacheRead: cacheRead,
		Reasoning: piAgentFirst(u.ReasoningTokens, u.ThoughtsTokens),
	}
	if u.CacheCreate5m != 0 || u.CacheCreate1h != 0 {
		t.CacheCreate5m = u.CacheCreate5m
		t.CacheCreate1h = u.CacheCreate1h
	} else {
		t.CacheCreate5m = u.CacheWriteTokens
	}
	return t
}

// scanPiAgent walks each base dir for *.jsonl files (default
// ~/.pi/agent/sessions/**), reads only the unconsumed tail (per State), maps
// usage lines to events, and returns the deduped set. Non-usage and malformed
// lines are counted in the report and skipped.
func scanPiAgent(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	if state == nil {
		state = NewState()
	}
	var (
		events []usage.Event
		report ScanReport
	)
	for _, base := range baseDirs {
		files, err := jsonlFilesUnder(base)
		if err != nil {
			report.Errors++
			continue
		}
		for _, path := range files {
			report.FilesScanned++
			piAgentScanFile(path, base, state, &events, &report)
		}
	}
	return usage.Dedup(events), report, nil
}

// piAgentScanFile reads the unconsumed portion of one file, appending events and
// updating report + state. It never returns an error; bad lines are counted.
func piAgentScanFile(path, base string, state *State, events *[]usage.Event, report *ScanReport) {
	defaultSession := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	project := piAgentProjectFromPath(path, base)

	scanJSONLIncremental(path, state, report, func(line []byte, _ int) {
		report.LinesParsed++
		if ev, ok, perr := piAgentParseLine(line, defaultSession, project); perr != nil {
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

// piAgentParseLine parses one JSONL line. ok=false means a valid line with no
// usage (skip). err!=nil means malformed JSON.
func piAgentParseLine(line []byte, defaultSession, project string) (usage.Event, bool, error) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return usage.Event{}, false, nil
	}
	var pl piAgentLine
	if err := json.Unmarshal([]byte(trimmed), &pl); err != nil {
		return usage.Event{}, false, err
	}
	if pl.Usage == nil {
		return usage.Event{}, false, nil // non-usage line
	}

	ev := usage.Event{
		Agent:       agentPiAgent,
		MessageID:   pl.ID,
		Model:       pl.Model,
		SessionID:   pl.SessionID,
		BillingType: usage.BillingAPI,
	}
	pl.Usage.canonical().apply(&ev)
	if ev.SessionID == "" {
		ev.SessionID = defaultSession
	}
	if pl.Cwd != "" {
		ev.Project = filepath.Base(pl.Cwd)
	} else {
		ev.Project = project
	}
	if pl.CostUSD != nil {
		ev.CostUSDProvided = pl.CostUSD
	}

	ts, tzMin := normalizeTimestamp(pl.Timestamp)
	ev.Timestamp = ts
	ev.TZOffsetMinutes = tzMin

	if !ev.HasUsage() {
		return usage.Event{}, false, nil
	}
	ev.EnsureID()
	return ev, true, nil
}

// piAgentFirst returns a if non-zero, else b (synonym fallback).
func piAgentFirst(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}

// piAgentProjectFromPath derives a project name from the session dir layout
// ~/.pi/agent/sessions/<project>/<session>.jsonl, relative to base. Falls back to
// the immediate parent dir name.
func piAgentProjectFromPath(path, base string) string {
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
