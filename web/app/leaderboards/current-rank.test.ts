import { readFileSync } from "node:fs";

const source = readFileSync("app/leaderboards/page.tsx", "utf8");

assertIncludes("leaderboards page imports current rank helpers", source, "@/lib/leaderboard-current-user");
assertIncludes("public leaderboard renders explicit current rank", source, "<CurrentRank entry={currentPublicEntry} />");
assertIncludes("ranking rows use current user helper", source, "isCurrentLeaderboardUser(row, currentUsername)");

function assertIncludes(name: string, sourceText: string, needle: string) {
  if (!sourceText.includes(needle)) {
    throw new Error(`${name}: expected page source to include ${needle}`);
  }
}
