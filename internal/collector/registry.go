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

// DefaultRegistry returns the built-in registry. Every supported agent has a
// real adapter; each AgentSpec describes its default data locations (overridable
// per-agent via STINT_COLLECT_<AGENT>_DIR). Adapters whose formats have not yet
// been verified against real on-disk data are marked "schema-only" in their
// source.
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
	r.register(AgentSpec{
		ID:           "cursor",
		DefaultPaths: []string{"~/.cursor", "~/Library/Application Support/Cursor/User/globalStorage", "~/.config/Cursor/User/globalStorage"},
		Format:       "cursor",
	}, AdapterFunc(scanCursor))
	r.register(AgentSpec{
		ID:           "copilot",
		DefaultPaths: []string{"~/.copilot/otel"},
		Format:       "copilot-otel",
	}, AdapterFunc(scanCopilot))
	r.register(AgentSpec{
		ID:           "openclaw",
		DefaultPaths: []string{"~/.openclaw/agents"},
		Format:       "openclaw-jsonl",
	}, AdapterFunc(scanOpenClaw))

	// VS Code globalStorage roots (local + remote + macOS) for editor-extension
	// agents; the adapter appends its extension's tasks/ subdir.
	codeGlobalStorage := func(ext string) []string {
		return []string{
			"~/.config/Code/User/globalStorage/" + ext,
			"~/.vscode-server/data/User/globalStorage/" + ext,
			"~/Library/Application Support/Code/User/globalStorage/" + ext,
		}
	}

	r.register(AgentSpec{ID: "qwen", DefaultPaths: []string{"~/.qwen/projects"}, Format: "qwen-jsonl"}, AdapterFunc(scanQwen))
	r.register(AgentSpec{ID: "roo", DefaultPaths: codeGlobalStorage("rooveterinaryinc.roo-cline/tasks"), Format: "roo-jsonl"}, AdapterFunc(scanRoo))
	r.register(AgentSpec{ID: "cline", DefaultPaths: codeGlobalStorage("saoudrizwan.claude-dev/tasks"), Format: "cline-jsonl"}, AdapterFunc(scanCline))
	r.register(AgentSpec{ID: "pi-agent", DefaultPaths: []string{"~/.pi/agent/sessions"}, Format: "pi-agent-jsonl"}, AdapterFunc(scanPiAgent))
	r.register(AgentSpec{ID: "amp", DefaultPaths: []string{"~/.local/share/amp/threads"}, Format: "amp-json"}, AdapterFunc(scanAmp))
	r.register(AgentSpec{ID: "crush", DefaultPaths: []string{"~/.local/share/crush"}, Format: "crush-json"}, AdapterFunc(scanCrush))
	r.register(AgentSpec{ID: "kimi", DefaultPaths: []string{"~/.kimi/sessions", "~/.kimi-code/sessions"}, Format: "kimi-jsonl"}, AdapterFunc(scanKimi))
	r.register(AgentSpec{ID: "factory-droid", DefaultPaths: []string{"~/.factory/sessions"}, Format: "factory-droid-jsonl"}, AdapterFunc(scanFactoryDroid))
	r.register(AgentSpec{ID: "hermes", DefaultPaths: []string{"~/.hermes"}, Format: "hermes-sqlite"}, AdapterFunc(scanHermes))
	r.register(AgentSpec{ID: "octofriend", DefaultPaths: []string{"~/.local/share/octofriend"}, Format: "octofriend-sqlite"}, AdapterFunc(scanOctofriend))
	r.register(AgentSpec{ID: "kiro", DefaultPaths: []string{"~/.kiro/sessions/cli", "~/.local/share/kiro-cli"}, Format: "kiro-mixed"}, AdapterFunc(scanKiro))
	r.register(AgentSpec{ID: "kilo", DefaultPaths: append([]string{"~/.local/share/kilo"}, codeGlobalStorage("kilocode.kilo-code/tasks")...), Format: "kilo-mixed"}, AdapterFunc(scanKilo))

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
