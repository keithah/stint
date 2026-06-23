import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const source = readFileSync("components/settings/custom-rules-card.tsx", "utf8");
const packageJSON = readFileSync("package.json", "utf8");

assert.match(source, /import \{ boundedPercent \} from "@\/lib\/chart-percent";/);
assert.match(source, /style=\{\{ width: `\$\{boundedPercent\(ruleProgress\.data\?\.data\.percent_complete \?\? 0\)\}%` \}\}/);
assert.match(packageJSON, /app\/settings\/progress-width\.test\.ts/);
