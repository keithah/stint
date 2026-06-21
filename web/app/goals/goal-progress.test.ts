import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const source = readFileSync("app/goals/page.tsx", "utf8");
const packageJSON = readFileSync("package.json", "utf8");

assert.match(source, /import \{ boundedPercent \} from "@\/lib\/chart-percent";/);
assert.match(source, /style=\{\{ width: `\$\{boundedPercent\(item\.percent\)\}%` \}\}/);
assert.doesNotMatch(source, /style=\{\{ width: `\$\{item\.percent\}%` \}\}/);
assert.match(packageJSON, /app\/goals\/goal-progress\.test\.ts/);
