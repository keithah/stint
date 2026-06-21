import type { AIStat } from "./api";
import { compactNumber } from "./number-format";

export type AIAgentDonutRow = {
  name: string;
  value: number;
  label: string;
};

export function aiAgentDonutRows(agents: AIStat[]): AIAgentDonutRow[] {
  const rows = agents
    .filter((agent) => agent.ai_line_changes > 0)
    .map((agent) => ({
      name: agent.name,
      value: agent.ai_line_changes,
      label: `${compactNumber(agent.ai_line_changes)} lines`
    }))
    .sort((a, b) => {
      if (a.value === b.value) {
        return a.name.localeCompare(b.name);
      }
      return b.value - a.value;
    });
  return rows.length ? rows : [{ name: "No AI agent data", value: 0, label: "0 lines" }];
}
