import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const source = readFileSync("components/settings/ai-costs-card.tsx", "utf8");
const packageJSON = readFileSync("package.json", "utf8");

assert.match(source, /const canSaveAICosts = costAgent\.trim\(\)\.length > 0 && Number\.isFinite\(inputCost\) && Number\.isFinite\(outputCost\) && inputCost >= 0 && outputCost >= 0;/);
assert.match(source, /agent: costAgent\.trim\(\)/);
assert.match(source, /disabled=\{saveCosts\.isPending \|\| !canSaveAICosts\}/);
assert.match(source, /onChange=\{\(event\) => setInputCost\(Math\.max\(0, Number\(event\.target\.value\)\)\)\}/);
assert.match(source, /onChange=\{\(event\) => setOutputCost\(Math\.max\(0, Number\(event\.target\.value\)\)\)\}/);
assert.match(packageJSON, /app\/settings\/ai-cost-validation\.test\.ts/);
