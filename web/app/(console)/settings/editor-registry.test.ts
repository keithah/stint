import { readFileSync } from "node:fs";
import assert from "node:assert/strict";

const source = readFileSync("components/settings/editors-card.tsx", "utf8");
const apiSource = readFileSync("lib/api.ts", "utf8");
const packageJSON = readFileSync("package.json", "utf8");

assert.match(apiSource, /export type EditorMetadata/);
assert.match(apiSource, /listEditors/);
assert.match(apiSource, /\/api\/v1\/editors/);
assert.match(source, /listEditors/);
assert.match(source, /queryKey: \["editors"\]/);
assert.match(source, /Supported editors/);
assert.match(source, /editors\.data\?\.data/);
assert.match(source, /editor\.version/);
assert.match(packageJSON, /scripts\/run-tests\.mjs/);
assert.match(packageJSON, /scripts\/run-tests\.mjs/);
