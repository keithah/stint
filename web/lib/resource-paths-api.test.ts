import {
  addLeaderboardMember,
  deleteCustomRule,
  deleteGoal,
  deleteLeaderboard,
  deleteOAuthApp,
  deleteShareToken,
  leaderboardEntries,
  removeLeaderboardMember,
  revokeKey,
  updateGoal,
  updateLeaderboard,
  type GoalPayload
} from "./api";

type FetchCall = {
  url: string;
  init?: RequestInit;
};

const calls: FetchCall[] = [];
const originalFetch = globalThis.fetch;

globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
  calls.push({ url: String(input), init });
  return {
    ok: true,
    status: init?.method === "DELETE" ? 204 : 200,
    json: async () => ({ data: { id: "ok" }, board: { id: "ok" }, members: [] })
  } as Response;
}) as typeof fetch;

run().catch((error) => {
  globalThis.fetch = originalFetch;
  throw error;
});

async function run() {
  const goal: GoalPayload = {
    title: "Daily",
    delta: "day",
    seconds: 3600,
    ignore_zero_days: false,
    is_enabled: true,
    is_inverse: false,
    is_snoozed: false
  };

  await updateGoal("goal 123", goal);
  await deleteGoal("goal 123");
  await leaderboardEntries("board 123");
  await updateLeaderboard("board 123", "Team Board", "last_7_days");
  await deleteLeaderboard("board 123");
  await addLeaderboardMember("board 123", "octocat");
  await removeLeaderboardMember("board 123", "user 456");
  await deleteCustomRule("rule 123");
  await revokeKey("key 123");
  await deleteOAuthApp("app 123");
  await deleteShareToken("share 123");
  globalThis.fetch = originalFetch;

  assertEqual("update goal URL", calls[0]?.url, "/api/v1/users/current/goals/goal%20123");
  assertEqual("delete goal URL", calls[1]?.url, "/api/v1/users/current/goals/goal%20123");
  assertEqual("leaderboard entries URL", calls[2]?.url, "/api/v1/users/current/leaderboards/board%20123");
  assertEqual("update leaderboard URL", calls[3]?.url, "/api/v1/users/current/leaderboards/board%20123");
  assertEqual("delete leaderboard URL", calls[4]?.url, "/api/v1/users/current/leaderboards/board%20123");
  assertEqual("add leaderboard member URL", calls[5]?.url, "/api/v1/users/current/leaderboards/board%20123/members");
  assertEqual("remove leaderboard member URL", calls[6]?.url, "/api/v1/users/current/leaderboards/board%20123/members/user%20456");
  assertEqual("delete custom rule URL", calls[7]?.url, "/api/v1/users/current/custom_rules/rule%20123");
  assertEqual("revoke key URL", calls[8]?.url, "/api/v1/api_keys/key%20123");
  assertEqual("delete OAuth app URL", calls[9]?.url, "/api/v1/oauth/apps/app%20123");
  assertEqual("delete share token URL", calls[10]?.url, "/api/v1/users/current/share_tokens/share%20123");
}

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}
