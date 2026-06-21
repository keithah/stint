import { currentLeaderboardEntry, isCurrentLeaderboardUser } from "./leaderboard-current-user";
import type { LeaderboardEntry } from "./api";

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}

const rows: LeaderboardEntry[] = [
  { user_id: "u1", username: "octocat", display_name: "Octo Cat", total_seconds: 7200, text: "2 hrs", rank: 1 },
  { user_id: "u2", username: "Keith", display_name: "Keith", total_seconds: 3600, text: "1 hr", rank: 2 }
];

assertEqual("current entry matches username case-insensitively", currentLeaderboardEntry(rows, "keith")?.rank, 2);
assertEqual("missing current entry returns undefined", currentLeaderboardEntry(rows, "missing"), undefined);
assertEqual("empty username is never current", isCurrentLeaderboardUser(rows[0], ""), false);
assertEqual("matching row is current case-insensitively", isCurrentLeaderboardUser(rows[1], "KEITH"), true);
