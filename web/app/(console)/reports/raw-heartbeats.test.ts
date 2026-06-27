import { readFileSync } from "node:fs";
import assert from "node:assert/strict";

const source = readFileSync("app/(console)/reports/page.tsx", "utf8");
const packageJSON = readFileSync("package.json", "utf8");

assert.match(source, /heartbeatsForDay/);
assert.match(source, /deleteHeartbeats/);
assert.match(source, /const \[heartbeatDate, setHeartbeatDate\]/);
assert.match(source, /const \[selectedHeartbeatIDs, setSelectedHeartbeatIDs\]/);
assert.match(source, /queryKey: \["heartbeats", heartbeatDate\]/);
assert.match(source, /mutationFn: \(\) => deleteHeartbeats\(heartbeatDate, selectedHeartbeatIDs\)/);
assert.match(source, /Raw heartbeats/);
assert.match(source, /selectedHeartbeatIDs\.includes\(heartbeat\.id\)/);
assert.match(source, /deleteSelectedHeartbeats\.mutate\(\)/);
assert.match(packageJSON, /scripts\/run-tests\.mjs/);
assert.match(packageJSON, /scripts\/run-tests\.mjs/);
