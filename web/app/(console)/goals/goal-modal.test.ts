import { readFileSync } from "node:fs";

const source = readFileSync("app/(console)/goals/page.tsx", "utf8");

assertIncludes("goals page tracks create modal state", source, "createModalOpen");
assertIncludes("goals page opens create modal from header action", source, "setCreateModalOpen(true)");
assertIncludes("goals page renders create modal", source, "<GoalModal mode=\"create\"");
assertIncludes("goals page renders edit modal", source, "<GoalModal mode=\"edit\"");
assertIncludes("goal modal exposes dialog semantics", source, 'role="dialog"');
assertIncludes("goal modal labels itself as modal", source, 'aria-modal="true"');

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected source to include ${needle}`);
  }
}
