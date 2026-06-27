import { readFileSync } from "node:fs";

const source = readFileSync("app/(console)/dashboard/page.tsx", "utf8");

assertIncludes("dashboard reuses the normal 30-day stats query key for AI trend", source, 'queryKey: ["stats", "last_30_days"]');
assertIncludes("dashboard fetches last 30 days for AI trend", source, 'queryFn: () => statsForRange("last_30_days")');
assertIncludes("dashboard feeds AI trend chart from fixed range", source, "days={aiTrend.data?.data.ai?.days ?? []}");
assertIncludes("dashboard labels AI trend as 30-day", source, 'title="AI vs Human 30-Day Trend"');

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected source to include ${needle}`);
  }
}
