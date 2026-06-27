import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const source = readFileSync("components/settings/custom-rules-card.tsx", "utf8");
const packageJSON = readFileSync("package.json", "utf8");

assert.match(source, /const canSaveCustomRule = ruleSourceValue\.trim\(\)\.length > 0 && Number\.isFinite\(rulePriority\) && rulePriority >= 1 && \(ruleAction === "delete" \|\| ruleDestinationValue\.trim\(\)\.length > 0\);/);
assert.match(source, /source_value: ruleSourceValue\.trim\(\)/);
assert.match(source, /destination_value: ruleDestinationValue\.trim\(\)/);
assert.match(source, /onChange=\{\(event\) => setRulePriority\(Math\.max\(1, Number\(event\.target\.value\) \|\| 1\)\)\}/);
assert.match(source, /disabled=\{saveRule\.isPending \|\| !canSaveCustomRule\}/);
assert.match(packageJSON, /scripts\/run-tests\.mjs/);
