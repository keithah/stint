import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";

const reportsSource = readFileSync(join(process.cwd(), "app/(console)/reports/page.tsx"), "utf8");
const eventsSource = readFileSync(join(process.cwd(), "lib/job-events.ts"), "utf8");

assert.match(reportsSource, /useJobEvents\(\)/);
assert.doesNotMatch(reportsSource, /hasPendingDumps|refetchInterval/);
assert.match(eventsSource, /\/api\/v1\/users\/current\/events/);
assert.match(eventsSource, /queryKey: \["me"\]/);
assert.match(eventsSource, /addEventListener\("data_dumps"/);
assert.match(eventsSource, /addEventListener\("custom_rules_progress"/);
assert.match(eventsSource, /queryKey: \["data-dumps"\]/);
assert.match(eventsSource, /queryKey: \["custom-rules-progress"\]/);
