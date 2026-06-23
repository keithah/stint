package collector

import "github.com/keithah/stint/internal/usage"

// SCHEMA-ONLY adapter: there is no real Cline host data available, so this is
// implemented to the documented/known on-disk format and verified solely with
// the bundled fixtures. Cline is the upstream of Roo Code and writes the same
// Anthropic-shaped usage (message.usage with input/output/cache_creation/
// cache_read, plus api_req_started metadata lines), so it shares Roo's parser
// via scanAnthropicTasks (see roo.go).

const agentCline = "cline"

// scanCline walks each base dir for *.jsonl files (default the VS Code
// globalStorage saoudrizwan.claude-dev/tasks/**), tail-reads per State, parses
// the shared Anthropic-shaped usage, and returns the deduped set. Non-usage and
// malformed lines are counted in the report and skipped.
func scanCline(baseDirs []string, state *State) ([]usage.Event, ScanReport, error) {
	return scanAnthropicTasks(baseDirs, state, agentCline)
}
