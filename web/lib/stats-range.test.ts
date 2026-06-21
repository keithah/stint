import { projectDetail, publicShareStatsByToken, publicUserStats, statsForRange, type StatsRange } from "./api";

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
    json: async () => ({ data: { range: "2026-06" } })
  } as Response;
}) as typeof fetch;

run().catch((error) => {
  globalThis.fetch = originalFetch;
  throw error;
});

async function run() {
  const calendarYear: StatsRange = "2026";
  const calendarMonth: StatsRange = "2026-06";

  await statsForRange(calendarYear);
  await publicUserStats("keith stint", calendarMonth);
  await publicShareStatsByToken("stintshare_token", calendarMonth);
  await projectDetail("stint api", calendarMonth);
  globalThis.fetch = originalFetch;

  assertEqual("calendar year stats URL", calls[0]?.url, "/api/v1/users/current/stats/2026");
  assertEqual("calendar month public stats URL", calls[1]?.url, "/api/v1/users/keith%20stint/stats/2026-06");
  assertEqual("calendar month share stats URL", calls[2]?.url, "/api/v1/share/stintshare_token/stats?range=2026-06");
  assertEqual("calendar month project stats URL", calls[3]?.url, "/api/v1/users/current/projects/stint%20api?range=2026-06");
}

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}
