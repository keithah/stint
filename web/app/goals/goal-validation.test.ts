import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const source = readFileSync("app/goals/page.tsx", "utf8");
const packageJSON = readFileSync("package.json", "utf8");

assert.match(source, /function validGoalDraft\(draft: GoalDraft \| null\)/);
assert.match(source, /draft\.seconds >= 0/);
assert.match(source, /Number\(draft\.improveByPercent\) >= 0/);
assert.match(source, /disabled=\{create\.isPending \|\| !validGoalDraft\(createDraft\)\}/);
assert.match(source, /disabled=\{update\.isPending \|\| !validGoalDraft\(editing\)\}/);
assert.match(source, /type="number" min=\{0\} step=\{60\}/);
assert.match(source, /type="number" min=\{0\} step=\{1\}/);
assert.match(source, /const improveByPercent = input\.improveByPercent\.trim\(\);/);
assert.match(source, /improve_by_percent: improveByPercent === "" \? undefined : Number\(improveByPercent\)/);
assert.match(packageJSON, /app\/goals\/goal-validation\.test\.ts/);
