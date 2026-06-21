import { readFileSync } from "node:fs";

const source = readFileSync("components/dashboard-charts.tsx", "utf8");

assertIncludes("AIHumanByDay accepts a title prop", source, 'title = "AI vs Human by Day"');
assertIncludes("AIHumanByDay renders the provided title", source, "{title}");

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected source to include ${needle}`);
  }
}
