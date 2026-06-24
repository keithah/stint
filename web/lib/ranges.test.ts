import { costModeOptions, rangeLabel, rangeOptions } from "./ranges";

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}

// The shared option arrays back the range/cost-mode controls on every console
// page; these assertions pin the values/labels that those controls render.
assertEqual("five ranges are offered", rangeOptions.length, 5);
assertEqual("first range is 7 days", rangeOptions[0].value, "last_7_days");
assertEqual("first range label", rangeOptions[0].label, "7 days");
assertEqual("last range is all time", rangeOptions[rangeOptions.length - 1].value, "all_time");
assertEqual("all-time label", rangeOptions[rangeOptions.length - 1].label, "All time");

assertEqual("three cost modes are offered", costModeOptions.length, 3);
assertEqual("first cost mode is auto", costModeOptions[0].value, "auto");
assertEqual("calculate cost mode label", costModeOptions[1].label, "Calculate");

assertEqual("rangeLabel resolves a known range", rangeLabel("last_30_days"), "30 days");
assertEqual("rangeLabel falls back to the first label", rangeLabel("nonsense" as never), "7 days");
