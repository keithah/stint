package collector

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

// SCHEMA-ONLY adapter: there is no real Roo Code host data available, so this is
// implemented to the documented/known on-disk format (Anthropic-shaped assistant
// usage, plus api_req_started metadata lines) and verified solely with the
// bundled fixtures. Cline (cline.go) is Roo's upstream and shares this parser.

const agentRoo = "roo"

// rooLine is the subset of a Roo/Cline task JSONL record we read. Roo stores two
// kinds of usage-bearing lines:
//
//   - an Anthropic-shaped assistant message with message.usage carrying
//     input_tokens / output_tokens / cache_creation_input_tokens (optionally a
//     5m/1h split) / cache_read_input_tokens.
//   - an "api_req_started" ui_messages line whose text payload is a JSON object
//     carrying tokensIn/tokensOut/cacheWrites/cacheReads and an optional cost.
//
// Every other line lacks usage and is skipped.
type rooLine struct {
	Type    string   `json:"type"`
	Say     string   `json:"say"`
	Ts      int64    `json:"ts"`
	Text    string   `json:"text"`
	CostUSD *float64 `json:"costUSD"`
	Cost    *float64 `json:"cost"`
	Model   string   `json:"model"`
	Message *struct {
		ID    string          `json:"id"`
		Model string          `json:"model"`
		Usage *anthropicUsage `json:"usage"`
	} `json:"message"`
}

// anthropicUsage is the Anthropic-shaped token block emitted by Roo and Cline
// (and Claude). cache_creation_input_tokens is the lumped 5m+1h write count; a
// cache_creation sub-object carries the split when present.
type anthropicUsage struct {
	InputTokens         int `json:"input_tokens"`
	OutputTokens        int `json:"output_tokens"`
	CacheCreationTokens int `json:"cache_creation_input_tokens"`
	CacheReadTokens     int `json:"cache_read_input_tokens"`
	CacheCreation       *struct {
		Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
		Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
}

// rooApiReqStarted is the JSON payload embedded (as a string) in the text field
// of an api_req_started ui_messages line. It carries flat token counts and an
// optional provider-reported cost.
type rooApiReqStarted struct {
	TokensIn    int      `json:"tokensIn"`
	TokensOut   int      `json:"tokensOut"`
	CacheWrites int      `json:"cacheWrites"`
	CacheReads  int      `json:"cacheReads"`
	Cost        *float64 `json:"cost"`
	Request     string   `json:"request"`
}

// scanRoo walks each base dir for *.jsonl files (default the VS Code
// globalStorage rooveterinaryinc.roo-cline/tasks/**), reads only the unconsumed
// tail (per State), maps usage lines to events, and returns the deduped set.
func scanRoo(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	return scanAnthropicTasks(baseDirs, state, agentRoo)
}

// scanAnthropicTasks is the shared scan body for the Roo-family adapters (Roo and
// its upstream Cline), parameterized by agent id. It walks *.jsonl files,
// tail-reads, parses Anthropic-shaped usage, and returns the deduped set.
func scanAnthropicTasks(baseDirs []string, state *State, agent string) ([]usage.Event, ScanReport, error) {
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
			anthropicScanFile(path, base, state, agent, &events, &report)
		}
	}
	return usage.Dedup(events), report, nil
}

// anthropicScanFile reads the unconsumed portion of one file, appending events
// and updating report + state. It never returns an error; bad lines are counted.
func anthropicScanFile(path, base string, state *State, agent string, events *[]usage.Event, report *ScanReport) {
	defaultSession := anthropicTaskSession(path, base)
	project := filepath.Base(filepath.Dir(path))

	scanJSONLIncremental(path, state, report, func(line []byte, _ int) {
		report.LinesParsed++
		if ev, ok, perr := anthropicParseLine(line, agent, defaultSession, project); perr != nil {
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

// anthropicParseLine parses one Roo/Cline JSONL line. ok=false means a valid line
// with no usage (skip). err!=nil means malformed JSON.
func anthropicParseLine(line []byte, agent, defaultSession, project string) (usage.Event, bool, error) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return usage.Event{}, false, nil
	}
	var rl rooLine
	if err := json.Unmarshal([]byte(trimmed), &rl); err != nil {
		return usage.Event{}, false, err
	}

	ev := usage.Event{
		Agent:       agent,
		SessionID:   defaultSession,
		Project:     project,
		BillingType: usage.BillingAPI,
	}
	if rl.Ts > 0 {
		ev.Timestamp = normalizeUnixMillis(rl.Ts)
	}

	switch {
	case rl.Message != nil && rl.Message.Usage != nil:
		u := rl.Message.Usage
		ev.MessageID = rl.Message.ID
		ev.Model = anthropicFirstStr(rl.Message.Model, rl.Model)
		ev.InputTokens = u.InputTokens
		ev.OutputTokens = u.OutputTokens
		ev.CacheReadTokens = u.CacheReadTokens
		// Prefer the explicit 5m/1h split; otherwise lump the cache_creation
		// write count entirely into the 5m bucket.
		if u.CacheCreation != nil && (u.CacheCreation.Ephemeral5m != 0 || u.CacheCreation.Ephemeral1h != 0) {
			ev.CacheCreate5mTokens = u.CacheCreation.Ephemeral5m
			ev.CacheCreate1hTokens = u.CacheCreation.Ephemeral1h
		} else {
			ev.CacheCreate5mTokens = u.CacheCreationTokens
		}
		if rl.CostUSD != nil {
			ev.CostUSDProvided = rl.CostUSD
		} else if rl.Cost != nil {
			ev.CostUSDProvided = rl.Cost
		}
	case rl.Say == "api_req_started" || rl.Type == "api_req_started":
		var meta rooApiReqStarted
		if rl.Text == "" || json.Unmarshal([]byte(rl.Text), &meta) != nil {
			return usage.Event{}, false, nil // metadata line we cannot parse: skip
		}
		ev.Model = rl.Model
		ev.InputTokens = meta.TokensIn
		ev.OutputTokens = meta.TokensOut
		ev.CacheCreate5mTokens = meta.CacheWrites
		ev.CacheReadTokens = meta.CacheReads
		if meta.Cost != nil {
			ev.CostUSDProvided = meta.Cost
		}
	default:
		return usage.Event{}, false, nil // non-usage line
	}

	if !ev.HasUsage() {
		return usage.Event{}, false, nil
	}
	ev.EnsureID()
	return ev, true, nil
}

// anthropicTaskSession derives the task/session id from the parent dir name of a
// VS Code globalStorage tasks/<taskId>/<file>.jsonl path, falling back to the
// file basename.
func anthropicTaskSession(path, base string) string {
	dir := filepath.Base(filepath.Dir(path))
	if dir != "" && dir != "." && dir != string(filepath.Separator) {
		if _, err := strconv.Atoi(dir); err == nil || dir != filepath.Base(base) {
			return dir
		}
	}
	return strings.TrimSuffix(filepath.Base(path), ".jsonl")
}

// anthropicFirstStr returns a if non-empty, else b.
func anthropicFirstStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
