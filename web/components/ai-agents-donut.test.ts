import { readFileSync } from "node:fs";

const source = readFileSync("components/ai-panel.tsx", "utf8");

assertIncludes("AI panel imports agent donut rows helper", source, "@/lib/ai-agent-donut");
assertIncludes("AI panel renders agents donut", source, "<AgentsDonut agents={ai.agents} />");
assertIncludes("AI panel defines agents donut component", source, "function AgentsDonut");
assertIncludes("AI panel uses local conic gradient for agents", source, "agentDonutGradient");

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected source to include ${needle}`);
  }
}
