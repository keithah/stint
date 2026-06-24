import { readFileSync } from "node:fs";
import assert from "node:assert/strict";

const source = readFileSync("components/settings/diagnostics-card.tsx", "utf8");
const apiSource = readFileSync("lib/api.ts", "utf8");
const packageJSON = readFileSync("package.json", "utf8");

assert.match(apiSource, /export type ServerMeta/);
assert.match(apiSource, /serverMeta/);
assert.match(apiSource, /\/api\/v1\/meta/);
assert.match(source, /serverMeta/);
assert.match(source, /queryKey: \["server-meta"\]/);
assert.match(source, /Server diagnostics/);
assert.match(source, /meta\.data\?\.data\.api_url/);
assert.match(source, /meta\.data\?\.data\.hostname/);
assert.match(source, /meta\.data\?\.data\.ip/);
assert.match(packageJSON, /lib\/meta-api\.test\.ts/);
assert.match(packageJSON, /app\/\(console\)\/settings\/meta-diagnostics\.test\.ts/);
