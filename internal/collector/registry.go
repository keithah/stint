// Package collector scans local AI coding-agent data files, normalizes them to
// canonical usage.Event records, and posts them to the Stint server.
//
// The design is data-driven: each agent is a registry entry pairing an
// AgentSpec (where its data lives, what parser to use) with an Adapter that
// turns files into events. Adding a new agent is a registry entry plus a
// parser, never a refactor of the scan/post pipeline.
package collector

import (
	"os"
	"sort"
	"strings"

	"github.com/keithah/stint/internal/usage"
)

// AgentSpec is the static description of an agent: its id, where its data files
// live by default, and which parser/format the adapter expects.
type AgentSpec struct {
	ID           string   // canonical agent id (matches usage.Event.Agent)
	DefaultPaths []string // default base dirs to scan (may contain ~)
	Format       string   // parser id, e.g. "claude-jsonl"
}

// ScanReport is the per-agent accounting of a scan. All counts are cumulative
// for the scan invocation.
type ScanReport struct {
	FilesScanned  int
	LinesParsed   int
	EventsEmitted int
	LinesSkipped  int
	Errors        int
	Note          string // human note, e.g. "not implemented"
}

// Adapter scans the given base dirs (overrides DefaultPaths when non-empty),
// advancing the incremental State, and returns the deduped events plus a
// report. An adapter must never panic or abort the whole scan on a single bad
// line; it counts the line in the report and keeps going.
type Adapter interface {
	Scan(baseDirs []string, state *State) ([]usage.Event, ScanReport, error)
}

// AdapterFunc adapts a plain function to the Adapter interface.
type AdapterFunc func(baseDirs []string, state *State) ([]usage.Event, ScanReport, error)

// Scan implements Adapter.
func (f AdapterFunc) Scan(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	return f(baseDirs, state)
}

// Entry binds an agent's static spec to its runtime adapter.
type Entry struct {
	Spec    AgentSpec
	Adapter Adapter
}

// Registry maps agent id -> entry.
type Registry map[string]Entry

// stub returns an adapter that emits nothing and reports "not implemented". It
// keeps unimplemented agents discoverable without special-casing them in the
// runner.
func stub() Adapter {
	return AdapterFunc(func(_ []string, _ *State) ([]usage.Event, ScanReport, error) {
		return nil, ScanReport{Note: "not implemented"}, nil
	})
}

// DefaultRegistry returns the built-in registry. Claude Code has a real
// adapter; the rest are stubs whose specs already describe their default
// locations so implementing one is just swapping stub() for a parser.
func DefaultRegistry() Registry {
	r := Registry{}

	r.register(AgentSpec{
		ID:           "claude",
		DefaultPaths: []string{"~/.claude/projects"},
		Format:       "claude-jsonl",
	}, AdapterFunc(scanClaude))
	r.register(AgentSpec{
		ID:           "codex",
		DefaultPaths: []string{"~/.codex/sessions"},
		Format:       "codex-jsonl",
	}, AdapterFunc(scanCodex))
	r.register(AgentSpec{
		ID:           "gemini",
		DefaultPaths: []string{"~/.gemini/tmp", "~/.gemini/antigravity-cli/conversations"},
		Format:       "gemini",
	}, AdapterFunc(scanGemini))
	r.register(AgentSpec{
		ID:           "opencode",
		DefaultPaths: []string{"~/.local/share/opencode"},
		Format:       "opencode-sqlite",
	}, AdapterFunc(scanOpenCode))
	r.register(AgentSpec{
		ID:           "goose",
		DefaultPaths: []string{"~/.local/share/goose/sessions"},
		Format:       "goose-sqlite",
	}, AdapterFunc(scanGoose))
	r.register(AgentSpec{
		ID:           "zed",
		DefaultPaths: []string{"~/.local/share/zed/threads"},
		Format:       "zed-sqlite",
	}, AdapterFunc(scanZed))

	stubs := []AgentSpec{
		{ID: "cursor", DefaultPaths: []string{"~/.cursor"}, Format: "cursor"},
		{ID: "copilot", DefaultPaths: []string{"~/.copilot/otel"}, Format: "copilot"},
		{ID: "amp", DefaultPaths: []string{"~/.amp"}, Format: "amp"},
		{ID: "qwen", DefaultPaths: []string{"~/.qwen"}, Format: "qwen"},
		{ID: "kimi", DefaultPaths: []string{"~/.kimi"}, Format: "kimi"},
		{ID: "kiro", DefaultPaths: []string{"~/.kiro"}, Format: "kiro"},
		{ID: "kilo", DefaultPaths: []string{"~/.kilo"}, Format: "kilo"},
		{ID: "roo", DefaultPaths: []string{"~/.roo"}, Format: "roo"},
		{ID: "cline", DefaultPaths: []string{"~/.cline"}, Format: "cline"},
		{ID: "hermes", DefaultPaths: []string{"~/.hermes"}, Format: "hermes"},
		{ID: "pi-agent", DefaultPaths: []string{"~/.pi-agent"}, Format: "pi-agent"},
		{ID: "openclaw", DefaultPaths: []string{"~/.openclaw"}, Format: "openclaw"},
		{ID: "factory-droid", DefaultPaths: []string{"~/.factory"}, Format: "factory-droid"},
		{ID: "crush", DefaultPaths: []string{"~/.crush"}, Format: "crush"},
		{ID: "octofriend", DefaultPaths: []string{"~/.octofriend"}, Format: "octofriend"},
	}
	for _, s := range stubs {
		r.register(s, stub())
	}
	return r
}

func (r Registry) register(spec AgentSpec, a Adapter) {
	r[spec.ID] = Entry{Spec: spec, Adapter: a}
}

// IDs returns the registered agent ids in stable sorted order.
func (r Registry) IDs() []string {
	ids := make([]string, 0, len(r))
	for id := range r {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// envOverrideVar returns the per-agent base-dir override env var name for an
// agent id, e.g. "claude" -> "STINT_COLLECT_CLAUDE_DIR".
func envOverrideVar(agentID string) string {
	up := strings.ToUpper(agentID)
	up = strings.NewReplacer("-", "_", ".", "_").Replace(up)
	return "STINT_COLLECT_" + up + "_DIR"
}

// BaseDirs resolves the base dirs for an agent: a per-agent env override
// (STINT_COLLECT_<AGENT>_DIR, OS-path-list separated) wins; otherwise the
// spec's DefaultPaths. All ~ prefixes are expanded.
func (e Entry) BaseDirs() []string {
	if v := os.Getenv(envOverrideVar(e.Spec.ID)); v != "" {
		parts := strings.Split(v, string(os.PathListSeparator))
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, ExpandHome(p))
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	out := make([]string, 0, len(e.Spec.DefaultPaths))
	for _, p := range e.Spec.DefaultPaths {
		out = append(out, ExpandHome(p))
	}
	return out
}

// ExpandHome replaces a leading ~ with the user's home directory.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			if p == "~" {
				return home
			}
			return home + p[1:]
		}
	}
	return p
}
