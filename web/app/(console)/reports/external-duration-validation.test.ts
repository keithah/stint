import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const source = readFileSync("app/(console)/reports/page.tsx", "utf8");
const packageJSON = readFileSync("package.json", "utf8");

assert.match(source, /const canCreateExternalDuration = externalEntity\.trim\(\)\.length > 0 && Number\.isFinite\(externalMinutes\) && externalMinutes > 0;/);
assert.match(source, /entity: externalEntity\.trim\(\)/);
assert.match(source, /project: externalProject\.trim\(\) \|\| undefined/);
assert.match(source, /language: externalLanguage\.trim\(\) \|\| undefined/);
assert.match(source, /onChange=\{\(event\) => setExternalMinutes\(Math\.max\(1, Number\(event\.target\.value\) \|\| 1\)\)\}/);
assert.match(source, /disabled=\{createExternal\.isPending \|\| !canCreateExternalDuration\}/);
assert.match(packageJSON, /app\/\(console\)\/reports\/external-duration-validation\.test\.ts/);
