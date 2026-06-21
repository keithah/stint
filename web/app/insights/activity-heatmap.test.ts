import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";

const source = readFileSync(join(process.cwd(), "app/insights/page.tsx"), "utf8");
const packageJSON = readFileSync(join(process.cwd(), "package.json"), "utf8");

assert.match(source, /activityHeatmapClass/);
assert.match(source, /activityHeatmapTitle/);
assert.match(source, /function ActivityHeatmap/);
assert.match(source, /<ActivityHeatmap days=\{stats\.data\?\.data\.days \?\? \[\]\} \/>/);
assert.match(source, /Coding heatmap/);
assert.match(source, /Less/);
assert.match(source, /More/);
assert.match(packageJSON, /lib\/activity-heatmap\.test\.ts/);
assert.match(packageJSON, /app\/insights\/activity-heatmap\.test\.ts/);
