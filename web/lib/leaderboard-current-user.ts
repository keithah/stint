import type { LeaderboardEntry } from "./api";

export function isCurrentLeaderboardUser(row: LeaderboardEntry, currentUsername?: string) {
  const username = normalizeLeaderboardUsername(currentUsername);
  return Boolean(username) && row.username.toLowerCase() === username;
}

export function currentLeaderboardEntry(rows: LeaderboardEntry[], currentUsername?: string) {
  const username = normalizeLeaderboardUsername(currentUsername);
  if (!username) {
    return undefined;
  }
  return rows.find((row) => row.username.toLowerCase() === username);
}

function normalizeLeaderboardUsername(username?: string) {
  return username?.trim().toLowerCase();
}
