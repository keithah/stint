import { readFileSync } from "node:fs";

const source = readFileSync("components/ai-panel.tsx", "utf8");

assertIncludes("AI panel imports ring style helper", source, "@/lib/ai-ring");
assertIncludes("AI panel renders AI coding ring", source, "<AICodingRing percentage={ai.ai_percentage} />");
assertIncludes("AI panel defines ring component", source, "function AICodingRing");
assertIncludes("AI ring uses helper style", source, "aiRingStyle(percentage)");

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected source to include ${needle}`);
  }
}
