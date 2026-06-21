export type ActivityHeatmapDay = {
  date?: string;
  text?: string;
  total_seconds: number;
};

const levelClasses = [
  "border-white/5 bg-white/[0.03]",
  "border-emerald-400/10 bg-emerald-500/20",
  "border-emerald-400/20 bg-emerald-500/40",
  "border-accent/30 bg-accent/60",
  "border-accent/50 bg-accent shadow-[0_0_14px_rgba(52,211,153,0.22)]"
];

export function activityHeatmapLevel(day: Pick<ActivityHeatmapDay, "total_seconds">, maxSeconds: number) {
  const seconds = Math.max(0, day.total_seconds);
  if (seconds === 0) {
    return 0;
  }
  const max = Math.max(1, maxSeconds);
  return Math.max(1, Math.min(4, Math.floor((seconds / max) * 4) + 1));
}

export function activityHeatmapClass(day: Pick<ActivityHeatmapDay, "total_seconds">, maxSeconds: number) {
  return levelClasses[activityHeatmapLevel(day, maxSeconds)];
}

export function activityHeatmapTitle(day: ActivityHeatmapDay) {
  return `${day.date ?? "Unknown day"}: ${day.text ?? formatShortDuration(day.total_seconds)}`;
}

function formatShortDuration(seconds: number) {
  const safeSeconds = Math.max(0, Math.round(seconds));
  const hours = Math.floor(safeSeconds / 3600);
  const minutes = Math.floor((safeSeconds % 3600) / 60);
  if (hours > 0 && minutes > 0) {
    return `${hours} hrs ${minutes} mins`;
  }
  if (hours > 0) {
    return `${hours} hrs`;
  }
  if (minutes > 0) {
    return `${minutes} mins`;
  }
  return `${safeSeconds} secs`;
}
