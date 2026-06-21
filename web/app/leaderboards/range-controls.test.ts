import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const source = readFileSync("app/leaderboards/page.tsx", "utf8");
const packageJSON = readFileSync("package.json", "utf8");

assert.match(source, /leaderboardRangeIsValid/);
assert.match(source, /normalizeLeaderboardRangeInput/);
assert.match(source, /value="all_time"/);
assert.match(source, /Create custom range/);
assert.match(source, /Edit custom range/);
assert.match(source, /placeholder="YYYY or YYYY-MM"/);
assert.match(packageJSON, /lib\/leaderboard-ranges\.test\.ts/);
assert.match(packageJSON, /app\/leaderboards\/range-controls\.test\.ts/);
