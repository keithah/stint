import { readFileSync } from "node:fs";

const chartSource = readFileSync("components/dashboard-charts.tsx", "utf8");
const dashboardSource = readFileSync("app/(console)/dashboard/page.tsx", "utf8");
const projectSource = readFileSync("app/(console)/projects/[name]/page.tsx", "utf8");

assertIncludes("ActivityBars accepts a title prop", chartSource, "title = \"Last 7 Days\"");
assertIncludes("ActivityBars renders the provided title", chartSource, "{title}");
assertIncludes("dashboard passes selected range title", dashboardSource, 'title={`${activeRange.label} Activity`}');
assertIncludes("project detail passes selected range title", projectSource, 'title={`${activeRange.label} Activity`}');

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected source to include ${needle}`);
  }
}
