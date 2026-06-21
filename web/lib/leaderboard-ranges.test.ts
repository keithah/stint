import { leaderboardRangeIsValid, normalizeLeaderboardRangeInput } from "./leaderboard-ranges";

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}

for (const value of ["last_7_days", "last_30_days", "last_6_months", "last_year", "all_time", "2026", "2026-06"]) {
  assertEqual(`${value} is valid`, leaderboardRangeIsValid(value), true);
}

for (const value of ["", "last_week", "2026-6", "2026-13", "abcd", "2026-00"]) {
  assertEqual(`${value || "blank"} is invalid`, leaderboardRangeIsValid(value), false);
}

assertEqual("normalizes whitespace", normalizeLeaderboardRangeInput(" 2026-06 "), "2026-06");
assertEqual("falls back to selected range", normalizeLeaderboardRangeInput(" ", "last_30_days"), "last_30_days");
assertEqual("rejects invalid custom input", normalizeLeaderboardRangeInput("2026-13", "last_7_days"), "");
