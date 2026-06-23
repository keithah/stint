package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

const agentAmp = "amp"

// SCHEMA-ONLY: no host data exists for Amp. This adapter is implemented to the
// documented/known Amp thread format and verified against fixtures only.
//
// Amp (Sourcegraph) stores each thread as a single JSON object under
// ~/.local/share/amp/threads/**/*.json. The object holds a messages array; the
// CLI rewrites the whole file in place as the conversation grows, so this is a
// whole-file JSON adapter (the State size+mtime skip pattern, like gemini.go):
// an unchanged file is skipped entirely and a changed one is fully re-parsed,
// with usage.Dedup collapsing messages already emitted via their stable id.
//
// Only assistant messages carry a usage/token block (input/output/cache
// tokens + model); user/tool messages do not and are skipped.
type ampThread struct {
	ThreadID  string       `json:"id"`
	ThreadID2 string       `json:"threadId"`
	Cwd       string       `json:"cwd"`
	Project   string       `json:"projectName"`
	Created   string       `json:"createdAt"`
	Messages  []ampMessage `json:"messages"`
}

// ampMessage is one entry in a thread's messages array. usage is optional; its
// absence (or all-zero counts) means the message is not a usage row.
type ampMessage struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Model     string    `json:"model"`
	Timestamp string    `json:"timestamp"`
	Usage     *ampUsage `json:"usage"`
}

// ampUsage mirrors the Anthropic-shaped token block Amp records on assistant
// messages: input_tokens is the prompt count (exclusive of cache reads here),
// with separate cache-creation and cache-read counts, plus a reasoning count
// and an optional provider-reported cost.
type ampUsage struct {
	anthropicUsageBlock
	ReasoningTokens int      `json:"reasoning_tokens"`
	CostUSD         *float64 `json:"cost_usd"`
}

// scanAmp implements the Amp adapter. It walks each base dir for *.json thread
// files, skips unchanged files via the State size+mtime cursor, parses the
// rest, maps assistant usage messages to events, and returns the deduped set.
func scanAmp(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	if state == nil {
		state = NewState()
	}
	var (
		events []usage.Event
		report ScanReport
	)
	for _, base := range baseDirs {
		files, err := ampFiles(base)
		if err != nil {
			report.Errors++
			continue
		}
		for _, path := range files {
			report.FilesScanned++
			scanAmpFile(path, state, &events, &report)
		}
	}
	return usage.Dedup(events), report, nil
}

// ampFiles returns all *.json files under base (recursively). A missing base
// dir yields no files and no error.
func ampFiles(base string) ([]string, error) {
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

// scanAmpFile parses one thread file if it changed since the last scan,
// appending events and updating report + state. It never returns an error; a
// bad file or message is counted.
func scanAmpFile(path string, state *State, events *[]usage.Event, report *ScanReport) {
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

	var thread ampThread
	if err := json.Unmarshal(b, &thread); err != nil {
		report.Errors++
		report.LinesSkipped++
		// Record the cursor so an unparseable file is not retried every scan.
		state.CommitFile(path, size, mtime, 0)
		return
	}

	session := thread.ThreadID
	if session == "" {
		session = thread.ThreadID2
	}
	if session == "" {
		session = strings.TrimSuffix(filepath.Base(path), ".json")
	}
	project := ampProject(thread, path)

	for i := range thread.Messages {
		report.LinesParsed++
		ev, ok := parseAmpMessage(&thread.Messages[i], session, project, thread.Created)
		if ok {
			*events = append(*events, ev)
			report.EventsEmitted++
		} else {
			report.LinesSkipped++
		}
	}

	state.CommitFile(path, size, mtime, len(thread.Messages))
}

// parseAmpMessage maps one thread message to an event. ok=false means the
// message carries no usage (user/tool line) and is skipped.
func parseAmpMessage(m *ampMessage, session, project, created string) (usage.Event, bool) {
	if m.Usage == nil {
		return usage.Event{}, false
	}
	u := m.Usage

	ev := usage.Event{
		Agent:       agentAmp,
		MessageID:   m.ID,
		Model:       m.Model,
		SessionID:   session,
		Project:     project,
		BillingType: usage.BillingAPI,
	}
	u.canonical().apply(&ev)
	ev.ReasoningTokens = u.ReasoningTokens
	if u.CostUSD != nil {
		ev.CostUSDProvided = u.CostUSD
	}

	ts := m.Timestamp
	if ts == "" {
		ts = created
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

// ampProject derives a project name: explicit projectName, else cwd basename,
// else the immediate parent dir name.
func ampProject(thread ampThread, path string) string {
	if thread.Project != "" {
		return thread.Project
	}
	if thread.Cwd != "" {
		return filepath.Base(thread.Cwd)
	}
	return filepath.Base(filepath.Dir(path))
}
