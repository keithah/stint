import { aiDayPercentage, aiHeatmapClass, aiHeatmapTitle } from "./ai-heatmap";

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}

assertEqual("empty day percentage", aiDayPercentage({ ai_line_changes: 0, human_line_changes: 0 }), 0);
assertEqual("mixed day percentage", aiDayPercentage({ ai_line_changes: 3, human_line_changes: 1 }), 75);
assertEqual("negative line values are ignored", aiDayPercentage({ ai_line_changes: -1, human_line_changes: 4 }), 0);

const fullAIClass = aiHeatmapClass({ ai_line_changes: 8, human_line_changes: 0 });
if (!fullAIClass.includes("shadow-[0_0_18px")) {
  throw new Error(`full AI day should glow, got ${fullAIClass}`);
}

const title = aiHeatmapTitle({
  name: "2026-06-19",
  ai_line_changes: 8,
  human_line_changes: 2,
  estimated_cost_cents: 123
});
assertEqual("title includes AI percentage", title, "2026-06-19: 80% AI, 8 AI lines, $1.23");
