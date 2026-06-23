package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

const agentCrush = "crush"

// SCHEMA-ONLY: no host data exists for Crush. This adapter is implemented to
// the documented/known Crush session format and verified against fixtures only.
//
// Crush (Charm) stores data under ~/.local/share/crush/: a projects.json index
// plus per-session JSON files. Each session file is a single JSON object with a
// messages array; the CLI rewrites the whole file in place, so this is a
// whole-file JSON adapter (the State size+mtime skip pattern, like gemini.go).
//
// projects.json is an index, not usage, and is skipped (it carries no messages
// array). Only assistant messages carry a token-usage block (+ model); user and
// tool messages do not and are skipped.
type crushSession struct {
	SessionID string         `json:"id"`
	Title     string         `json:"title"`
	Cwd       string         `json:"cwd"`
	Project   string         `json:"project"`
	Created   string         `json:"createdAt"`
	Messages  []crushMessage `json:"messages"`
}

// crushMessage is one entry in a session's messages array. tokens is optional;
// its absence (or all-zero counts) means the message is not a usage row.
type crushMessage struct {
	ID        string       `json:"id"`
	Role      string       `json:"role"`
	Model     string       `json:"model"`
	Timestamp string       `json:"timestamp"`
	Tokens    *crushTokens `json:"tokens"`
}

// crushTokens mirrors the Anthropic-shaped token block Crush records on
// assistant messages. input is the prompt count exclusive of cache reads here.
type crushTokens struct {
	Input         int      `json:"input"`
	Output        int      `json:"output"`
	CacheCreation int      `json:"cacheCreation"`
	CacheRead     int      `json:"cacheRead"`
	Reasoning     int      `json:"reasoning"`
	CostUSD       *float64 `json:"costUSD"`
}

// scanCrush implements the Crush adapter. It walks each base dir for *.json
// files, skips unchanged files via the State size+mtime cursor, parses the
// rest, maps assistant usage messages to events, and returns the deduped set.
func scanCrush(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	if state == nil {
		state = NewState()
	}
	var (
		events []usage.Event
		report ScanReport
	)
	for _, base := range baseDirs {
		files, err := crushFiles(base)
		if err != nil {
			report.Errors++
			continue
		}
		for _, path := range files {
			report.FilesScanned++
			scanCrushFile(path, state, &events, &report)
		}
	}
	return usage.Dedup(events), report, nil
}

// crushFiles returns all *.json files under base (recursively). A missing base
// dir yields no files and no error.
func crushFiles(base string) ([]string, error) {
	info, err := os.Stat(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		if strings.HasSuffix(base, ".json") {
			return []string{base}, nil
		}
		return nil, nil
	}
	var files []string
	err = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // tolerate unreadable subtrees
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".json") {
			files = append(files, p)
		}
		return nil
	})
	return files, err
}

// scanCrushFile parses one session file if it changed since the last scan,
// appending events and updating report + state. It never returns an error; a
// bad file or message is counted. A file with no messages array (e.g. the
// projects.json index) yields no events.
func scanCrushFile(path string, state *State, events *[]usage.Event, report *ScanReport) {
	info, err := os.Stat(path)
	if err != nil {
		report.Errors++
		return
	}
	size := info.Size()
	mtime := info.ModTime().UnixNano()

	if state.FileUnchanged(path, size, mtime) {
		return
	}

	b, err := os.ReadFile(path)
	if err != nil {
		report.Errors++
		return
	}

	var sess crushSession
	if err := json.Unmarshal(b, &sess); err != nil {
		report.Errors++
		report.LinesSkipped++
		// Record the cursor so an unparseable file is not retried every scan.
		state.CommitFile(path, size, mtime, 0)
		return
	}

	session := sess.SessionID
	if session == "" {
		session = strings.TrimSuffix(filepath.Base(path), ".json")
	}
	project := crushProject(sess, path)

	for i := range sess.Messages {
		report.LinesParsed++
		ev, ok := parseCrushMessage(&sess.Messages[i], session, project, sess.Created)
		if ok {
			*events = append(*events, ev)
			report.EventsEmitted++
		} else {
			report.LinesSkipped++
		}
	}

	state.CommitFile(path, size, mtime, len(sess.Messages))
}

// parseCrushMessage maps one session message to an event. ok=false means the
// message carries no usage (user/tool line) and is skipped.
func parseCrushMessage(m *crushMessage, session, project, created string) (usage.Event, bool) {
	if m.Tokens == nil {
		return usage.Event{}, false
	}
	t := m.Tokens

	ev := usage.Event{
		Agent:           agentCrush,
		MessageID:       m.ID,
		Model:           m.Model,
		SessionID:       session,
		Project:         project,
		InputTokens:     t.Input,
		OutputTokens:    t.Output,
		CacheReadTokens: t.CacheRead,
		ReasoningTokens: t.Reasoning,
		// Crush reports a single cache-creation count with no 5m/1h split;
		// preserve it in the 5m bucket (matching the Claude lumping convention).
		CacheCreate5mTokens: t.CacheCreation,
		BillingType:         usage.BillingAPI,
	}
	if t.CostUSD != nil {
		ev.CostUSDProvided = t.CostUSD
	}

	ts := m.Timestamp
	if ts == "" {
		ts = created
	}
	tt, tzMin := normalizeTimestamp(ts)
	ev.Timestamp = tt
	ev.TZOffsetMinutes = tzMin

	if !ev.HasUsage() {
		return usage.Event{}, false
	}

	ev.EnsureID()
	return ev, true
}

// crushProject derives a project name: explicit project, else cwd basename,
// else the immediate parent dir name.
func crushProject(sess crushSession, path string) string {
	if sess.Project != "" {
		return sess.Project
	}
	if sess.Cwd != "" {
		return filepath.Base(sess.Cwd)
	}
	return filepath.Base(filepath.Dir(path))
}
