import type { StatsRange } from "@/lib/api";
import type { UsageCostMode } from "@/lib/usage-api";

export const rangeOptions: ReadonlyArray<{ value: StatsRange; label: string }> = [
  { value: "last_7_days", label: "7 days" },
  { value: "last_30_days", label: "30 days" },
  { value: "last_6_months", label: "6 months" },
  { value: "last_year", label: "Year" },
  { value: "all_time", label: "All time" }
];

export const costModeOptions: ReadonlyArray<{ value: UsageCostMode; label: string }> = [
  { value: "auto", label: "Auto" },
  { value: "calculate", label: "Calculate" },
  { value: "display", label: "Display" }
];

export function rangeLabel(range: StatsRange): string {
  return rangeOptions.find((o) => o.value === range)?.label ?? rangeOptions[0].label;
}
