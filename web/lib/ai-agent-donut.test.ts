import { aiAgentDonutRows } from "./ai-agent-donut";
import type { AIStat } from "./api";

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}

const agents: AIStat[] = [
  { name: "Claude", ai_seconds: 0, ai_line_changes: 40, human_line_changes: 0, ai_input_tokens: 0, ai_output_tokens: 0, ai_prompt_length: 0, session_count: 1, estimated_cost_cents: 25 },
  { name: "Codex", ai_seconds: 0, ai_line_changes: 3015, human_line_changes: 0, ai_input_tokens: 0, ai_output_tokens: 0, ai_prompt_length: 0, session_count: 3, estimated_cost_cents: 90 },
  { name: "Cursor", ai_seconds: 0, ai_line_changes: 0, human_line_changes: 0, ai_input_tokens: 0, ai_output_tokens: 0, ai_prompt_length: 0, session_count: 1, estimated_cost_cents: 0 }
];

const rows = aiAgentDonutRows(agents);

assertEqual("agent rows are sorted by AI lines", rows[0].name, "Codex");
assertEqual("agent row value uses AI line changes", rows[0].value, 3015);
assertEqual("agent row label uses compact line count", rows[0].label, "3.02K lines");
assertEqual("zero-line agents are omitted", rows.length, 2);

const emptyRows = aiAgentDonutRows([]);
assertEqual("empty state keeps a renderable row", emptyRows[0].name, "No AI agent data");
assertEqual("empty state has zero value", emptyRows[0].value, 0);
