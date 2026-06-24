import { readFileSync } from "node:fs";

const dashboardSource = readFileSync("app/(console)/dashboard/page.tsx", "utf8");
const chartSource = readFileSync("components/dashboard-charts.tsx", "utf8");

assertIncludes("dashboard renders its hero status header", dashboardSource, "<HeroHeader");
assertIncludes("dashboard header carries a stable ops class", dashboardSource, "ops-dashboard-header");
assertIncludes("dashboard header surfaces cache freshness", dashboardSource, "freshnessLabel(data)");
assertIncludes("dashboard header links to settings setup", dashboardSource, 'href="/settings"');
assertIncludes("chart module uses a shared ops panel frame", chartSource, "const chartPanelClass");
assertIncludes("chart panels receive ops density styling", chartSource, "ops-chart-panel");

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected source to include ${needle}`);
  }
}
