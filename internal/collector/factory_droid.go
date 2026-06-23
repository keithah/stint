package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

const agentFactoryDroid = "factory-droid"

// SCHEMA-ONLY: no host data exists for Factory Droid. This adapter is
// implemented to the documented/known Factory session format and verified
// against fixtures only.
//
// Factory Droid writes one JSON object per line under
// ~/.factory/sessions/**/*.jsonl. Assistant messages carry a generic per-message
// usage block with input/output/cache/reasoning counts (+ model). input_tokens
// is treated as the prompt count exclusive of cache reads; the cache-creation
// count has no 5m/1h split, so it is preserved in the 5m bucket. Lines without a
// usage block (user/system/tool messages) are skipped.
type factoryDroidLine struct {
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
			ReasoningTokens int      `json:"reasoning_tokens"`
			CostUSD         *float64 `json:"cost_usd"`
		} `json:"usage"`
	} `json:"message"`
}

// scanFactoryDroid implements the Factory Droid adapter. It walks each base dir
// for *.jsonl files, reads only the unconsumed tail (per State), parses each
// line, maps usage lines to events, and returns the deduped set.
func scanFactoryDroid(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	if state == nil {
		state = NewState()
	}
	var (
		events []usage.Event
		report ScanReport
	)
	for _, base := range baseDirs {
		files, err := factoryDroidFiles(base)
		if err != nil {
			report.Errors++
			continue
		}
		for _, path := range files {
			report.FilesScanned++
			scanFactoryDroidFile(path, base, state, &events, &report)
		}
	}
	return usage.Dedup(events), report, nil
}

// factoryDroidFiles returns all *.jsonl files under base (recursively). A
// missing base dir yields no files and no error.
func factoryDroidFiles(base string) ([]string, error) {
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

// scanFactoryDroidFile reads the unconsumed portion of one file, appending
// events and updating report + state. It never returns an error; bad lines are
// counted.
func scanFactoryDroidFile(path, base string, state *State, events *[]usage.Event, report *ScanReport) {
	defaultSession := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	pathProject := factoryDroidProjectFromPath(path, base)

	scanJSONLIncremental(path, state, report, func(line []byte, _ int) {
		report.LinesParsed++
		if ev, ok, perr := parseFactoryDroidLine(line, defaultSession, pathProject); perr != nil {
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

// parseFactoryDroidLine parses one JSONL line. ok=false means it is a valid
// line with no usage (skip, not an error). err!=nil means malformed JSON.
func parseFactoryDroidLine(line []byte, defaultSession, pathProject string) (usage.Event, bool, error) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return usage.Event{}, false, nil
	}
	var cl factoryDroidLine
	if err := json.Unmarshal([]byte(trimmed), &cl); err != nil {
		return usage.Event{}, false, err
	}
	if cl.Message.Usage == nil {
		return usage.Event{}, false, nil // non-usage line
	}
	u := cl.Message.Usage

	ev := usage.Event{
		Agent:       agentFactoryDroid,
		MessageID:   cl.Message.ID,
		RequestID:   cl.RequestID,
		Model:       cl.Message.Model,
		BillingType: usage.BillingAPI,
	}
	u.canonical().apply(&ev)
	ev.ReasoningTokens = u.ReasoningTokens
	if u.CostUSD != nil {
		ev.CostUSDProvided = u.CostUSD
	}

	ev.SessionID = cl.SessionID
	if ev.SessionID == "" {
		ev.SessionID = defaultSession
	}

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

// factoryDroidProjectFromPath derives a fallback project name from the session
// path (~/.factory/sessions/<project>/<session>.jsonl), returning the first
// path segment under base or the immediate parent dir name.
func factoryDroidProjectFromPath(path, base string) string {
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
