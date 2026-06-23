package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

const agentGemini = "gemini"

// geminiSession is the on-disk shape of a Gemini CLI chat session file. Each
// session is a single JSON object (not JSONL) with a messages array that the
// CLI rewrites as the conversation grows. Only assistant ("gemini") messages
// carry a tokens block; user messages and tool/thought-only messages do not.
type geminiSession struct {
	SessionID   string          `json:"sessionId"`
	ProjectHash string          `json:"projectHash"`
	Project     string          `json:"projectName"`
	Cwd         string          `json:"cwd"`
	StartTime   string          `json:"startTime"`
	Messages    []geminiMessage `json:"messages"`
}

// geminiMessage is one entry in a session's messages array. The tokens block is
// optional; its absence (or all-zero counts) means the line is not a usage row.
type geminiMessage struct {
	ID        string        `json:"id"`
	Timestamp string        `json:"timestamp"`
	Type      string        `json:"type"`
	Model     string        `json:"model"`
	Tokens    *geminiTokens `json:"tokens"`
	// usageMetadata is the alternative (Google GenAI native) token shape some
	// Gemini variants emit; it is read as a fallback when tokens is absent.
	UsageMetadata *geminiUsageMetadata `json:"usageMetadata"`
}

// geminiTokens is the Gemini CLI session token block. The CLI reports input as
// the full prompt count *inclusive* of cached tokens (verified on real data:
// input + output + thoughts == total, with cached <= input), so InputTokens is
// input minus cached to avoid double-counting cache reads.
type geminiTokens struct {
	Input    int `json:"input"`
	Output   int `json:"output"`
	Cached   int `json:"cached"`
	Thoughts int `json:"thoughts"`
	Tool     int `json:"tool"`
	Total    int `json:"total"`
}

// geminiUsageMetadata is the Google GenAI native usage shape (camelCase or
// snake_case). promptTokenCount is inclusive of cachedContentTokenCount.
type geminiUsageMetadata struct {
	PromptTokenCount      int `json:"promptTokenCount"`
	PromptTokenCount2     int `json:"prompt_token_count"`
	CandidatesTokenCount  int `json:"candidatesTokenCount"`
	CandidatesTokenCount2 int `json:"candidates_token_count"`
	CachedContentCount    int `json:"cachedContentTokenCount"`
	CachedContentCount2   int `json:"cached_content_token_count"`
	ThoughtsTokenCount    int `json:"thoughtsTokenCount"`
	ThoughtsTokenCount2   int `json:"thoughts_token_count"`
}

// scanGemini implements the Adapter for Gemini CLI session files. It walks each
// base dir for *.json session files. Because a session file is a single JSON
// object that the CLI rewrites in place (rather than appending lines), the
// incremental State is used at file granularity: a file whose size+mtime match
// the recorded cursor is skipped entirely; a changed file is fully re-parsed,
// and usage.Dedup collapses messages already emitted on a prior scan via their
// stable per-message id. A single malformed file or message is counted and the
// scan keeps going.
func scanGemini(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	if state == nil {
		state = NewState()
	}
	var (
		events []usage.Event
		report ScanReport
	)
	for _, base := range baseDirs {
		files, err := geminiFiles(base)
		if err != nil {
			report.Errors++
			continue
		}
		for _, path := range files {
			report.FilesScanned++
			scanGeminiFile(path, state, &events, &report)
		}
	}
	return usage.Dedup(events), report, nil
}

// geminiFiles returns all *.json session files under base (recursively),
// limited to those inside a chats/ directory or named session*. A missing base
// dir yields no files and no error.
func geminiFiles(base string) ([]string, error) {
	info, err := os.Stat(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		if geminiIsSessionFile(base) {
			return []string{base}, nil
		}
		return nil, nil
	}
	var files []string
	err = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // tolerate unreadable subtrees
		}
		if !d.IsDir() && geminiIsSessionFile(p) {
			files = append(files, p)
		}
		return nil
	})
	return files, err
}

// geminiIsSessionFile reports whether a path looks like a Gemini chat session
// file: a *.json file either named session*.json or living under a chats/ dir.
func geminiIsSessionFile(p string) bool {
	if !strings.HasSuffix(p, ".json") {
		return false
	}
	base := filepath.Base(p)
	if strings.HasPrefix(base, "session") {
		return true
	}
	return strings.Contains(filepath.ToSlash(p), "/chats/")
}

// scanGeminiFile parses one session file if it changed since the last scan,
// appending events and updating report + state. It never returns an error; a
// bad file or message is counted.
func scanGeminiFile(path string, state *State, events *[]usage.Event, report *ScanReport) {
	info, err := os.Stat(path)
	if err != nil {
		report.Errors++
		return
	}
	size := info.Size()
	mtime := info.ModTime().UnixNano()

	// File granularity skip: if size+mtime are unchanged from the recorded
	// cursor, nothing new to read.
	if fs, ok := state.get(path); ok && fs.Size == size && fs.ModTimeNano == mtime {
		return
	}

	b, err := os.ReadFile(path)
	if err != nil {
		report.Errors++
		return
	}

	var sess geminiSession
	if err := json.Unmarshal(b, &sess); err != nil {
		report.Errors++
		report.LinesSkipped++
		// Record the cursor anyway so an unparseable file is not retried every
		// scan; it will be reparsed only when it changes again.
		state.commit(path, size, mtime, size, 0)
		return
	}

	defaultSession := sess.SessionID
	if defaultSession == "" {
		defaultSession = strings.TrimSuffix(filepath.Base(path), ".json")
	}
	project := geminiProject(sess, path)

	for i := range sess.Messages {
		report.LinesParsed++
		ev, ok := parseGeminiMessage(&sess.Messages[i], defaultSession, project, sess.StartTime)
		if ok {
			*events = append(*events, ev)
			report.EventsEmitted++
		} else {
			report.LinesSkipped++
		}
	}

	state.commit(path, size, mtime, size, len(sess.Messages))
}

// parseGeminiMessage maps one session message to an event. ok=false means the
// message carries no usage (user line, tool-only line) and is skipped.
func parseGeminiMessage(m *geminiMessage, defaultSession, project, startTime string) (usage.Event, bool) {
	input, output, cached, thoughts, has := geminiTokenCounts(m)
	if !has {
		return usage.Event{}, false
	}

	// input is inclusive of cached prompt tokens; subtract so cache reads are
	// not double-counted against input.
	in := input - cached
	if in < 0 {
		in = 0
	}

	ev := usage.Event{
		Agent:           agentGemini,
		MessageID:       m.ID,
		Model:           m.Model,
		SessionID:       defaultSession,
		Project:         project,
		InputTokens:     in,
		OutputTokens:    output,
		CacheReadTokens: cached,
		ReasoningTokens: thoughts,
		BillingType:     usage.BillingAPI,
	}

	ts := m.Timestamp
	if ts == "" {
		ts = startTime
	}
	t, tzMin := normalizeTimestamp(ts)
	ev.Timestamp = t
	ev.TZOffsetMinutes = tzMin

	if !ev.HasUsage() {
		return usage.Event{}, false
	}

	ev.EnsureID()
	return ev, true
}

// geminiTokenCounts extracts (input, output, cached, thoughts) from whichever
// token shape the message carries. has=false means no usage block at all.
func geminiTokenCounts(m *geminiMessage) (input, output, cached, thoughts int, has bool) {
	if m.Tokens != nil {
		t := m.Tokens
		return t.Input, t.Output, t.Cached, t.Thoughts, true
	}
	if m.UsageMetadata != nil {
		u := m.UsageMetadata
		input = geminiFirst(u.PromptTokenCount, u.PromptTokenCount2)
		output = geminiFirst(u.CandidatesTokenCount, u.CandidatesTokenCount2)
		cached = geminiFirst(u.CachedContentCount, u.CachedContentCount2)
		thoughts = geminiFirst(u.ThoughtsTokenCount, u.ThoughtsTokenCount2)
		return input, output, cached, thoughts, true
	}
	return 0, 0, 0, 0, false
}

// geminiFirst returns a if non-zero, else b (camelCase vs snake_case fallback).
func geminiFirst(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}

// geminiProject derives a project name: explicit projectName, else cwd
// basename, else the chats/ parent dir, else the projectHash.
func geminiProject(sess geminiSession, path string) string {
	if sess.Project != "" {
		return sess.Project
	}
	if sess.Cwd != "" {
		return filepath.Base(sess.Cwd)
	}
	// Gemini stores sessions under tmp/<projectHash>/chats/<session>.json; the
	// hash dir is the only project signal available.
	dir := filepath.Dir(path)
	if filepath.Base(dir) == "chats" {
		return filepath.Base(filepath.Dir(dir))
	}
	return sess.ProjectHash
}
