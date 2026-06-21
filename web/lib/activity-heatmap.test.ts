import { activityHeatmapClass, activityHeatmapLevel, activityHeatmapTitle } from "./activity-heatmap";

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}

assertEqual("empty day has no intensity", activityHeatmapLevel({ total_seconds: 0 }, 3600), 0);
assertEqual("negative totals are clamped", activityHeatmapLevel({ total_seconds: -30 }, 3600), 0);
assertEqual("quarter day is low-medium intensity", activityHeatmapLevel({ total_seconds: 900 }, 3600), 2);
assertEqual("full day reaches max intensity", activityHeatmapLevel({ total_seconds: 3600 }, 3600), 4);
assertEqual("missing max does not divide by zero", activityHeatmapLevel({ total_seconds: 60 }, 0), 4);

const maxClass = activityHeatmapClass({ total_seconds: 3600 }, 3600);
if (!maxClass.includes("bg-accent")) {
  throw new Error(`max activity should use accent background, got ${maxClass}`);
}

assertEqual(
  "title includes date and readable total",
  activityHeatmapTitle({ date: "2026-06-19", text: "1 hr", total_seconds: 3600 }),
  "2026-06-19: 1 hr"
);
