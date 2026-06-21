import { readFileSync } from "node:fs";

const source = readFileSync("components/ai-panel.tsx", "utf8");

assertIncludes("AI panel renders cost tracker heading", source, "Cost tracker");
assertIncludes("AI panel reads cost rows from metrics", source, "ai.costs");
assertIncludes("AI panel shows daily cost period", source, "Daily");
assertIncludes("AI panel shows weekly cost period", source, "Weekly");
assertIncludes("AI panel shows monthly cost period", source, "Monthly");

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected source to include ${needle}`);
  }
}
