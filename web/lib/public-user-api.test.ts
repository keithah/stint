import { publicUserProfile, publicUserStats, publicUserSummaries } from "./api";

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
    status: 200,
    json: async () => ({ data: { id: "user-1", username: "keith stint", name: "Keith" } })
  } as Response;
}) as typeof fetch;

run().catch((error) => {
  globalThis.fetch = originalFetch;
  throw error;
});

async function run() {
  await publicUserProfile("keith stint");
  await publicUserStats("keith stint", "last_30_days");
  await publicUserSummaries("keith stint", "2026-06-01", "2026-06-19");
  globalThis.fetch = originalFetch;

  assertEqual("public profile URL", calls[0]?.url, "/api/v1/users/keith%20stint");
  assertEqual("public stats URL", calls[1]?.url, "/api/v1/users/keith%20stint/stats/last_30_days");
  assertEqual("public summaries URL", calls[2]?.url, "/api/v1/users/keith%20stint/summaries?start=2026-06-01&end=2026-06-19");
  assertEqual("public profile method defaults to GET", calls[0]?.init?.method, undefined);
}

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}
