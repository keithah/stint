package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

// SCHEMA-ONLY adapter: there is no real Qwen Code host data available, so this
// is implemented to the documented/known on-disk format (Qwen Code mirrors the
// Gemini/OpenAI usage shape) and verified solely with the bundled fixtures.

const agentQwen = "qwen"

// qwenLine is the subset of a Qwen Code transcript JSONL record we read. Qwen
// Code logs one JSON object per line; usage rows carry either a usageMetadata
// block (Google GenAI native shape) or a usage block (prompt/completion/cached/
// thoughts or input/output/cached/reasoning). Non-usage lines lack both and are
// skipped.
type qwenLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd"`
	Message   struct {
		ID    string `json:"id"`
		Model string `json:"model"`
	} `json:"message"`
	Model         string             `json:"model"`
	Usage         *qwenUsage         `json:"usage"`
	UsageMetadata *qwenUsageMetadata `json:"usageMetadata"`
}

// qwenUsage is the flat usage block. It accepts both the OpenAI-style
// (prompt/completion/cached/reasoning) and the input/output/cached/thoughts
// naming. The first non-zero of each synonym pair wins. input is treated as
// inclusive of cached, so it maps onto geminiUsageBlock (input-minus-cached) via
// block() rather than the OpenAI block.
type qwenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	InputTokens      int `json:"input_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CachedTokens     int `json:"cached_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens"`
	ReasoningTokens  int `json:"reasoning_tokens"`
	ThoughtsTokens   int `json:"thoughts_tokens"`
}

// block resolves the synonym pairs and maps onto the shared geminiUsageBlock so
// canonical() does the input-minus-cached and reasoning mapping.
func (u qwenUsage) block() geminiUsageBlock {
	return geminiUsageBlock{
		PromptTokenCount:        qwenFirst(u.InputTokens, u.PromptTokens),
		CandidatesTokenCount:    qwenFirst(u.OutputTokens, u.CompletionTokens),
		CachedContentTokenCount: qwenFirst(u.CacheReadTokens, u.CachedTokens),
		ThoughtsTokenCount:      qwenFirst(u.ReasoningTokens, u.ThoughtsTokens),
	}
}

// qwenUsageMetadata is the Google GenAI native usage shape (camelCase or
// snake_case). promptTokenCount is inclusive of cachedContentTokenCount.
type qwenUsageMetadata struct {
	PromptTokenCount      int `json:"promptTokenCount"`
	PromptTokenCount2     int `json:"prompt_token_count"`
	CandidatesTokenCount  int `json:"candidatesTokenCount"`
	CandidatesTokenCount2 int `json:"candidates_token_count"`
	CachedContentCount    int `json:"cachedContentTokenCount"`
	CachedContentCount2   int `json:"cached_content_token_count"`
	ThoughtsTokenCount    int `json:"thoughtsTokenCount"`
	ThoughtsTokenCount2   int `json:"thoughts_token_count"`
}

// block maps the snake/camel usageMetadata onto the shared geminiUsageBlock.
func (u qwenUsageMetadata) block() geminiUsageBlock {
	return geminiUsageBlock{
		PromptTokenCount:        qwenFirst(u.PromptTokenCount, u.PromptTokenCount2),
		CandidatesTokenCount:    qwenFirst(u.CandidatesTokenCount, u.CandidatesTokenCount2),
		CachedContentTokenCount: qwenFirst(u.CachedContentCount, u.CachedContentCount2),
		ThoughtsTokenCount:      qwenFirst(u.ThoughtsTokenCount, u.ThoughtsTokenCount2),
	}
}

// scanQwen walks each base dir for *.jsonl files (default ~/.qwen/projects/**),
// reads only the unconsumed tail (per State), maps usage lines to events, and
// returns the deduped set. Non-usage and malformed lines are counted in the
// report and skipped.
func scanQwen(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	if state == nil {
		state = NewState()
	}
	var (
		events []usage.Event
		report ScanReport
	)
	for _, base := range baseDirs {
		files, err := qwenFiles(base)
		if err != nil {
			report.Errors++
			continue
		}
		for _, path := range files {
			report.FilesScanned++
			qwenScanFile(path, base, state, &events, &report)
		}
	}
	return usage.Dedup(events), report, nil
}

// qwenFiles returns all *.jsonl files under base (recursively). A missing base
// dir yields no files and no error.
func qwenFiles(base string) ([]string, error) {
	return jsonlFilesUnder(base)
}

// jsonlFilesUnder returns all *.jsonl files under base (recursively). A missing
// base dir yields no files and no error; a base that is itself a *.jsonl file is
// returned directly. Shared by the schema-only JSONL adapters (qwen, roo, cline,
// pi-agent) so each does not reinvent the walk.
func jsonlFilesUnder(base string) ([]string, error) {
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

// qwenScanFile reads the unconsumed portion of one file, appending events and
// updating report + state. It never returns an error; bad lines are counted.
func qwenScanFile(path, base string, state *State, events *[]usage.Event, report *ScanReport) {
	defaultSession := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	pathProject := qwenProjectFromPath(path, base)

	scanJSONLIncremental(path, state, report, func(line []byte, _ int) {
		report.LinesParsed++
		if ev, ok, perr := qwenParseLine(line, defaultSession, pathProject); perr != nil {
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

// qwenParseLine parses one JSONL line. ok=false means a valid line with no
// usage (skip). err!=nil means malformed JSON.
func qwenParseLine(line []byte, defaultSession, pathProject string) (usage.Event, bool, error) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return usage.Event{}, false, nil
	}
	var ql qwenLine
	if err := json.Unmarshal([]byte(trimmed), &ql); err != nil {
		return usage.Event{}, false, err
	}

	block, has := qwenMessageBlock(&ql)
	if !has {
		return usage.Event{}, false, nil // non-usage line
	}

	model := ql.Message.Model
	if model == "" {
		model = ql.Model
	}

	// canonical() does the input-minus-cached split (input is inclusive of cached
	// prompt tokens) and the reasoning mapping.
	ev := usage.Event{
		Agent:       agentQwen,
		MessageID:   ql.Message.ID,
		Model:       model,
		SessionID:   ql.SessionID,
		BillingType: usage.BillingAPI,
	}
	block.canonical().apply(&ev)
	if ev.SessionID == "" {
		ev.SessionID = defaultSession
	}
	if ql.Cwd != "" {
		ev.Project = filepath.Base(ql.Cwd)
	} else {
		ev.Project = pathProject
	}

	ts, tzMin := normalizeTimestamp(ql.Timestamp)
	ev.Timestamp = ts
	ev.TZOffsetMinutes = tzMin

	if !ev.HasUsage() {
		return usage.Event{}, false, nil
	}
	ev.EnsureID()
	return ev, true, nil
}

// qwenMessageBlock returns the usage block for whichever shape the line carries.
// has=false means no usage block at all.
func qwenMessageBlock(ql *qwenLine) (geminiUsageBlock, bool) {
	if ql.Usage != nil {
		return ql.Usage.block(), true
	}
	if ql.UsageMetadata != nil {
		return ql.UsageMetadata.block(), true
	}
	return geminiUsageBlock{}, false
}

// qwenFirst returns a if non-zero, else b (synonym fallback).
func qwenFirst(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}

// qwenProjectFromPath derives a project name from the encoded project dir in
// ~/.qwen/projects/<encoded>/<session>.jsonl, relative to base. Falls back to
// the immediate parent dir name.
func qwenProjectFromPath(path, base string) string {
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
