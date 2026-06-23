package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

const agentKimi = "kimi"

// SCHEMA-ONLY: no host data exists for Kimi. This adapter is implemented to the
// documented/known Kimi/Moonshot session format and verified against fixtures
// only.
//
// Kimi (Moonshot) writes one JSON object per line under
// ~/.kimi/sessions/**/*.jsonl and ~/.kimi-code/sessions/**/*.jsonl. Its usage
// block mirrors the OpenAI Chat Completions shape:
//
//	usage: { prompt_tokens, completion_tokens, cached_tokens }
//
// prompt_tokens is INCLUSIVE of cached_tokens, so InputTokens is derived as
// prompt_tokens - cached_tokens (cached -> CacheRead) to avoid double-counting,
// completion_tokens -> OutputTokens. Lines without a usage block (user/system/
// tool messages) are skipped.
type kimiLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	ID        string `json:"id"`
	Model     string `json:"model"`
	Usage     *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		CachedTokens     int `json:"cached_tokens"`
		// prompt_tokens_details.cached_tokens is the alternative OpenAI nesting.
		PromptTokensDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

// kimiBaseDirs are the two default session roots Kimi/Moonshot writes to.
var kimiBaseDirs = []string{
	ExpandHome("~/.kimi/sessions"),
	ExpandHome("~/.kimi-code/sessions"),
}

// scanKimi implements the Kimi adapter. It walks each base dir for *.jsonl
// files, reads only the unconsumed tail (per State), parses each line, maps
// usage lines to events, and returns the deduped set.
func scanKimi(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	if state == nil {
		state = NewState()
	}
	if len(baseDirs) == 0 {
		baseDirs = kimiBaseDirs
	}
	var (
		events []usage.Event
		report ScanReport
	)
	for _, base := range baseDirs {
		files, err := kimiFiles(base)
		if err != nil {
			report.Errors++
			continue
		}
		for _, path := range files {
			report.FilesScanned++
			scanKimiFile(path, base, state, &events, &report)
		}
	}
	return usage.Dedup(events), report, nil
}

// kimiFiles returns all *.jsonl files under base (recursively). A missing base
// dir yields no files and no error.
func kimiFiles(base string) ([]string, error) {
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

// scanKimiFile reads the unconsumed portion of one file, appending events and
// updating report + state. It never returns an error; bad lines are counted.
func scanKimiFile(path, base string, state *State, events *[]usage.Event, report *ScanReport) {
	defaultSession := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	pathProject := kimiProjectFromPath(path, base)

	scanJSONLIncremental(path, state, report, func(line []byte, _ int) {
		report.LinesParsed++
		if ev, ok, perr := parseKimiLine(line, defaultSession, pathProject); perr != nil {
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

// parseKimiLine parses one JSONL line. ok=false means it is a valid line with
// no usage (skip, not an error). err!=nil means malformed JSON.
func parseKimiLine(line []byte, defaultSession, pathProject string) (usage.Event, bool, error) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return usage.Event{}, false, nil
	}
	var cl kimiLine
	if err := json.Unmarshal([]byte(trimmed), &cl); err != nil {
		return usage.Event{}, false, err
	}
	if cl.Usage == nil {
		return usage.Event{}, false, nil // non-usage line
	}
	u := cl.Usage

	cached := u.CachedTokens
	if cached == 0 && u.PromptTokensDetails != nil {
		cached = u.PromptTokensDetails.CachedTokens
	}

	// OpenAI shape: prompt_tokens is the TOTAL including cached tokens. Subtract
	// the cached count so cache reads are not counted twice.
	input := u.PromptTokens - cached
	if input < 0 {
		input = 0
	}

	ev := usage.Event{
		Agent:           agentKimi,
		MessageID:       cl.ID,
		Model:           cl.Model,
		InputTokens:     input,
		OutputTokens:    u.CompletionTokens,
		CacheReadTokens: cached,
		BillingType:     usage.BillingAPI,
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

// kimiProjectFromPath derives a fallback project name from the session path
// (~/.kimi/sessions/<project>/<session>.jsonl), returning the first path
// segment under base or the immediate parent dir name.
func kimiProjectFromPath(path, base string) string {
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
