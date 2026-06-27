import { readFileSync } from "node:fs";
import assert from "node:assert/strict";

const source = readFileSync("app/(console)/reports/page.tsx", "utf8");
const packageJSON = readFileSync("package.json", "utf8");

assert.match(source, /durationsForDay/);
assert.match(source, /const \[durationDate, setDurationDate\]/);
assert.match(source, /const \[durationSlice, setDurationSlice\]/);
assert.match(source, /queryKey: \["durations", durationDate, durationSlice\]/);
assert.match(source, /queryFn: \(\) => durationsForDay\(durationDate, durationSlice\)/);
assert.match(source, /Duration breakdown/);
assert.match(source, /durationRows\.reduce\(\(sum, row\) => sum \+ row\.duration, 0\)/);
assert.match(source, /formatHeartbeatTime\(row\.time\)/);
assert.match(packageJSON, /scripts\/run-tests\.mjs/);
assert.match(packageJSON, /scripts\/run-tests\.mjs/);
