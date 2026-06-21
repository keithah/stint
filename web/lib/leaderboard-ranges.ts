import type { StatsRange } from "./api";

const fixedLeaderboardRanges = new Set(["last_7_days", "last_30_days", "last_6_months", "last_year", "all_time"]);

export function leaderboardRangeIsValid(value: string) {
  const trimmed = value.trim();
  if (fixedLeaderboardRanges.has(trimmed)) {
    return true;
  }
  if (/^\d{4}$/.test(trimmed)) {
    return true;
  }
  const month = /^(\d{4})-(\d{2})$/.exec(trimmed);
  if (!month) {
    return false;
  }
  const monthNumber = Number(month[2]);
  return monthNumber >= 1 && monthNumber <= 12;
}

export function normalizeLeaderboardRangeInput(customValue: string, fallback: StatsRange = "last_7_days"): StatsRange | "" {
  const trimmed = customValue.trim();
  if (trimmed === "") {
    return fallback;
  }
  return leaderboardRangeIsValid(trimmed) ? (trimmed as StatsRange) : "";
}
