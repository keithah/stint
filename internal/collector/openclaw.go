package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

const agentOpenClaw = "openclaw"

// openClawLine is the subset of an OpenClaw transcript JSONL record we read.
//
// OpenClaw writes one JSON object per line under ~/.openclaw/agents/. Like
// Claude Code and Codex, only the lines carrying a usage block are emitted;
// everything else (user/system/tool messages) lacks a usage object and is
// skipped. The usage block mirrors the Anthropic shape (ccusage detects
// openclaw and treats its tokens with Claude semantics): input_tokens is
// INCLUSIVE of cached input, so we subtract the cached count from input and
// route the cached portion to cache_read to avoid double-counting.
type openClawLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	RequestID string `json:"request_id"`
	Message   struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *struct {
			anthropicUsageBlock
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// scanOpenClaw implements the Adapter for OpenClaw transcripts. It walks each
// base dir for *.jsonl files, reads only the unconsumed tail (per State),
// parses each line, maps usage lines to events, and returns the deduped set.
func scanOpenClaw(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	if state == nil {
		state = NewState()
	}
	var (
		events []usage.Event
		report ScanReport
	)
	for _, base := range baseDirs {
		files, err := openClawFiles(base)
		if err != nil {
			report.Errors++
			continue
		}
		for _, path := range files {
			report.FilesScanned++
			scanOpenClawFile(path, base, state, &events, &report)
		}
	}
	return usage.Dedup(events), report, nil
}

// openClawFiles returns all *.jsonl files under base (recursively). A missing
// base dir yields no files and no error.
func openClawFiles(base string) ([]string, error) {
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

// scanOpenClawFile reads the unconsumed portion of one file, appending events
// and updating report + state. It never returns an error; bad lines are counted.
func scanOpenClawFile(path, base string, state *State, events *[]usage.Event, report *ScanReport) {
	defaultSession := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	pathProject := openClawProjectFromPath(path, base)

	scanJSONLIncremental(path, state, report, func(line []byte, _ int) {
		report.LinesParsed++
		if ev, ok, perr := parseOpenClawLine(line, defaultSession, pathProject); perr != nil {
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

// parseOpenClawLine parses one JSONL line. ok=false means it is a valid line
// with no usage (skip, not an error). err!=nil means malformed JSON.
func parseOpenClawLine(line []byte, defaultSession, pathProject string) (usage.Event, bool, error) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return usage.Event{}, false, nil
	}
	var cl openClawLine
	if err := json.Unmarshal([]byte(trimmed), &cl); err != nil {
		return usage.Event{}, false, err
	}
	if cl.Message.Usage == nil {
		return usage.Event{}, false, nil // non-usage line
	}
	u := cl.Message.Usage

	ev := usage.Event{
		Agent:       agentOpenClaw,
		MessageID:   cl.Message.ID,
		RequestID:   cl.RequestID,
		Model:       cl.Message.Model,
		BillingType: usage.BillingSubscription,
	}
	u.canonical().apply(&ev)
	ev.ReasoningTokens = u.ReasoningTokens

	// OpenClaw mirrors Anthropic semantics: input_tokens is inclusive of cached
	// input. Subtract the cached count from input and route it to cache_read so
	// it is not counted twice.
	ev.InputTokens -= u.CacheReadTokens
	if ev.InputTokens < 0 {
		ev.InputTokens = 0
	}

	// Session: explicit field, else file basename.
	ev.SessionID = cl.SessionID
	if ev.SessionID == "" {
		ev.SessionID = defaultSession
	}

	// Project: cwd basename, else the parent dir from the path.
	if cl.Cwd != "" {
		ev.Project = filepath.Base(cl.Cwd)
	} else {
		ev.Project = pathProject
	}

	ts, tzMin := normalizeTimestamp(cl.Timestamp)
	ev.Timestamp = ts
	ev.TZOffsetMinutes = tzMin

	if !ev.HasUsage() {
		return usage.Event{}, false, nil
	}

	ev.EnsureID()
	return ev, true, nil
}

// openClawProjectFromPath derives a fallback project name from the agent
// transcript path (~/.openclaw/agents/<agent>/<session>.jsonl). It returns the
// immediate parent dir name relative to base.
func openClawProjectFromPath(path, base string) string {
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
