import { formatCents } from "./number-format";

export type AIHeatmapDay = {
  name: string;
  ai_line_changes: number;
  human_line_changes: number;
  estimated_cost_cents: number;
};

export function aiDayPercentage(day: Pick<AIHeatmapDay, "ai_line_changes" | "human_line_changes">) {
  const aiLines = Math.max(0, day.ai_line_changes);
  const humanLines = Math.max(0, day.human_line_changes);
  const totalLines = aiLines + humanLines;
  if (totalLines === 0) {
    return 0;
  }
  return Math.round((aiLines / totalLines) * 100);
}

export function aiHeatmapClass(day: Pick<AIHeatmapDay, "ai_line_changes" | "human_line_changes">) {
  const percentage = aiDayPercentage(day);
  if (percentage === 0) {
    return "border-line bg-white/[0.03]";
  }
  if (percentage >= 100) {
    return "border-cyan-200 bg-cyan-200 shadow-[0_0_18px_rgba(103,232,249,0.55)]";
  }
  if (percentage >= 75) {
    return "border-emerald-300/70 bg-emerald-300";
  }
  if (percentage >= 45) {
    return "border-emerald-400/50 bg-emerald-500/70";
  }
  if (percentage >= 20) {
    return "border-teal-400/40 bg-teal-500/50";
  }
  return "border-cyan-400/30 bg-cyan-500/30";
}

export function aiHeatmapTitle(day: AIHeatmapDay) {
  return `${day.name}: ${aiDayPercentage(day)}% AI, ${day.ai_line_changes.toLocaleString()} AI lines, ${formatCents(day.estimated_cost_cents)}`;
}
