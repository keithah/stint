import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";

const source = readFileSync(join(process.cwd(), "app/dashboard/page.tsx"), "utf8");
const packageJSON = readFileSync(join(process.cwd(), "package.json"), "utf8");

assert.match(source, /className="grid gap-4 md:grid-cols-5"/);
assert.match(source, /<StatCard label="Today"/);
assert.match(source, /value=\{status\.data\?\.data\.grand_total_text \?\? "0 secs"\}/);
assert.match(source, /detail=\{todayDetail\(status\.data\?\.data\.project, status\.data\?\.data\.language\)\}/);
assert.match(source, /function todayDetail\(project\?: string, language\?: string\)/);
assert.match(packageJSON, /app\/dashboard\/current-day-card\.test\.ts/);
