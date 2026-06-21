import type { LeaderboardEntry } from "./api";

export function isCurrentLeaderboardUser(row: LeaderboardEntry, currentUsername?: string) {
  const username = currentUsername?.trim();
  return Boolean(username) && row.username.toLowerCase() === username?.toLowerCase();
}

export function currentLeaderboardEntry(rows: LeaderboardEntry[], currentUsername?: string) {
  return rows.find((row) => isCurrentLeaderboardUser(row, currentUsername));
}
