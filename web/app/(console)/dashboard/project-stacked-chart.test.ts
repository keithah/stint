import { readFileSync } from "node:fs";

const dashboardSource = readFileSync("app/(console)/dashboard/page.tsx", "utf8");
const chartSource = readFileSync("components/dashboard-charts.tsx", "utf8");

assertIncludes("dashboard imports project stacked area chart", dashboardSource, "ProjectStackedArea");
assertIncludes("dashboard renders project stacked area chart", dashboardSource, "<ProjectStackedArea days={data?.days ?? []} />");
assertIncludes("chart module exports project stacked area chart", chartSource, "export function ProjectStackedArea");
assertIncludes("project stacked chart uses local stacked svg", chartSource, "StackedBarSvg");
assertIncludes("project stacked chart indexes rows before segment lookup", chartSource, "sliceTotalByName");

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected source to include ${needle}`);
  }
}
