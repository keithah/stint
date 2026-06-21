import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";

const source = readFileSync(join(process.cwd(), "app/insights/page.tsx"), "utf8");
const packageJSON = readFileSync(join(process.cwd(), "package.json"), "utf8");

const requiredBreakdowns = [
  "stats",
  "projects",
  "languages",
  "editors",
  "machines",
  "operating_systems",
  "categories",
  "dependencies",
  "days",
  "hours",
  "weekdays",
  "best_day",
  "daily_average",
  "daily_average_trend",
  "ai_agents",
  "ai_days"
];

for (const breakdown of requiredBreakdowns) {
  assert.match(source, new RegExp(`"${breakdown}"`), `${breakdown} should be available in the insights breakdown selector`);
}

assert.match(source, /type HourlyStat/);
assert.match(source, /const dayRows = breakdown === "days"/);
assert.match(source, /const hourRows = breakdown === "hours"/);
assert.match(source, /const bestDay = breakdown === "best_day"/);
assert.match(source, /const dailyAverage = breakdown === "daily_average"/);
assert.match(source, /<DailyRows rows=\{dayRows\}/);
assert.match(source, /<HourlyRows rows=\{hourRows\}/);
assert.match(source, /<BestDayInsight day=\{bestDay\}/);
assert.match(source, /<DailyAverageInsight average=\{dailyAverage\}/);
assert.match(packageJSON, /app\/insights\/breakdown-coverage\.test\.ts/);
