package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

const agentCodex = "codex"

// codexLine is the subset of a Codex (OpenAI) rollout JSONL record we read.
//
// Codex writes one JSON object per line. The records we care about are:
//   - "session_meta" / "turn_context": carry the cwd (project) and the active
//     model. These appear before the usage events they describe, so the scanner
//     tracks the most recently seen model/cwd and applies them to later usage.
//   - "event_msg" with payload.type "token_count": carries the token usage. Its
//     payload.info holds two blocks: total_token_usage (cumulative for the whole
//     session, monotonically increasing) and last_token_usage (the delta for the
//     turn that just completed). We emit from last_token_usage so summing across
//     events does not double-count the cumulative totals.
//
// Every other line lacks a usable token_count payload and is skipped.
type codexLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

// codexSessionMeta is the payload of a "session_meta" line.
type codexSessionMeta struct {
	ID    string `json:"id"`
	Cwd   string `json:"cwd"`
	Model string `json:"model"`
}

// codexTurnContext is the payload of a "turn_context" line. Codex records the
// active model and cwd here, refreshed each turn.
type codexTurnContext struct {
	Cwd   string `json:"cwd"`
	Model string `json:"model"`
}

// codexTokenCount is the payload of an "event_msg" token_count line. info may be
// null on some events (e.g. pure rate-limit pings); those carry no usage. The
// usage blocks follow the OpenAI shape, decoded via openAIUsageBlock. We emit
// from last_token_usage (the per-turn delta) rather than total_token_usage (the
// cumulative session total) so summing events does not double-count.
type codexTokenCount struct {
	Type string `json:"type"`
	Info *struct {
		LastTokenUsage  *openAIUsageBlock `json:"last_token_usage"`
		TotalTokenUsage *openAIUsageBlock `json:"total_token_usage"`
	} `json:"info"`
}

// codexState tracks the model/cwd carried by session_meta / turn_context lines so
// later token_count events can be attributed. It is per-file scan state, distinct
// from the incremental *State offset tracking.
type codexState struct {
	model string
	cwd   string
}

// scanCodex implements the Adapter for Codex rollout transcripts. It walks each
// base dir for *.jsonl files, reads only the unconsumed tail (per State), parses
// each line, maps token_count lines to events, and returns the deduped set.
func scanCodex(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	if state == nil {
		state = NewState()
	}
	var (
		events []usage.Event
		report ScanReport
	)
	for _, base := range baseDirs {
		files, err := codexFiles(base)
		if err != nil {
			report.Errors++
			continue
		}
		for _, path := range files {
			report.FilesScanned++
			scanCodexFile(path, base, state, &events, &report)
		}
	}
	return usage.Dedup(events), report, nil
}

// codexFiles returns all *.jsonl files under base (recursively). A missing base
// dir yields no files and no error.
func codexFiles(base string) ([]string, error) {
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

// scanCodexFile reads the unconsumed portion of one file, appending events and
// updating report + state. It never returns an error; bad lines are counted.
//
// Because model/cwd live on earlier lines than the token_count events they
// describe, codexState is rebuilt from offset 0 only when the scan starts there.
// When resuming mid-file the carried context starts empty; usage is still
// emitted (model/project simply default), which is acceptable for incremental
// tails and keeps the scan resilient.
func scanCodexFile(path, base string, state *State, events *[]usage.Event, report *ScanReport) {
	defaultSession := codexSessionFromName(path)
	pathProject := codexProjectFromPath(path, base)
	cs := &codexState{}

	scanJSONLIncremental(path, state, report, func(line []byte, _ int) {
		report.LinesParsed++
		if ev, ok, perr := parseCodexLine(line, defaultSession, pathProject, cs); perr != nil {
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

// parseCodexLine parses one JSONL line. It updates cs with any model/cwd context
// from session_meta / turn_context lines. ok=false means it is a valid line with
// no usage (skip, not an error). err!=nil means malformed JSON.
func parseCodexLine(line []byte, defaultSession, pathProject string, cs *codexState) (usage.Event, bool, error) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return usage.Event{}, false, nil
	}
	var cl codexLine
	if err := json.Unmarshal([]byte(trimmed), &cl); err != nil {
		return usage.Event{}, false, err
	}

	switch cl.Type {
	case "session_meta":
		var meta codexSessionMeta
		if err := json.Unmarshal(cl.Payload, &meta); err == nil {
			if meta.Cwd != "" {
				cs.cwd = meta.Cwd
			}
			if meta.Model != "" {
				cs.model = meta.Model
			}
		}
		return usage.Event{}, false, nil
	case "turn_context":
		var tc codexTurnContext
		if err := json.Unmarshal(cl.Payload, &tc); err == nil {
			if tc.Cwd != "" {
				cs.cwd = tc.Cwd
			}
			if tc.Model != "" {
				cs.model = tc.Model
			}
		}
		return usage.Event{}, false, nil
	case "event_msg":
		// fall through to token_count handling below
	default:
		return usage.Event{}, false, nil
	}

	var tc codexTokenCount
	if err := json.Unmarshal(cl.Payload, &tc); err != nil {
		// A non-object payload on an event_msg is not malformed JSON at the
		// line level (the line parsed); treat as a skip.
		return usage.Event{}, false, nil
	}
	if tc.Type != "token_count" || tc.Info == nil || tc.Info.LastTokenUsage == nil {
		return usage.Event{}, false, nil // non-usage event_msg (rate-limit ping, etc.)
	}
	// OpenAI's input_tokens is the TOTAL including cached tokens; canonical()
	// splits them so cache reads are not counted twice.
	ev := usage.Event{
		Agent:       agentCodex,
		SessionID:   defaultSession,
		Model:       cs.model,
		BillingType: usage.BillingAPI,
	}
	tc.Info.LastTokenUsage.canonical().apply(&ev)

	if cs.cwd != "" {
		ev.Project = filepath.Base(cs.cwd)
	} else {
		ev.Project = pathProject
	}

	ts, tzMin := normalizeTimestamp(cl.Timestamp)
	ev.Timestamp = ts
	ev.TZOffsetMinutes = tzMin

	// A usage block with all zeros is not a real usage event.
	if !ev.HasUsage() {
		return usage.Event{}, false, nil
	}

	ev.EnsureID()
	return ev, true, nil
}

// codexSessionFromName derives a session id from a rollout filename of the form
// rollout-<timestamp>-<uuid>.jsonl, returning the trailing UUID when present and
// otherwise the basename without extension.
func codexSessionFromName(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	const prefix = "rollout-"
	if strings.HasPrefix(name, prefix) {
		rest := name[len(prefix):]
		// The UUID is the last 5 dash-separated groups; the timestamp uses dashes
		// too, so split and take the final UUID-shaped tail.
		parts := strings.Split(rest, "-")
		if len(parts) >= 5 {
			return strings.Join(parts[len(parts)-5:], "-")
		}
	}
	return name
}

// codexProjectFromPath derives a fallback project name from the date-partitioned
// rollout path (~/.codex/sessions/YYYY/MM/DD/<file>.jsonl). There is no project
// dir in the path, so it falls back to the immediate parent dir name.
func codexProjectFromPath(path, base string) string {
	// The rollout path has no project dir, so the project always falls back to
	// the immediate parent dir name.
	return filepath.Base(filepath.Dir(path))
}
