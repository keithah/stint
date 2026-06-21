import { publicShareSummaries, publicShareSummariesByToken } from "./api";

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
    json: async () => ({ data: [] })
  } as Response;
}) as typeof fetch;

run().catch((error) => {
  globalThis.fetch = originalFetch;
  throw error;
});

async function run() {
  await publicShareSummaries("keith stint", "stintshare_token", "2026-06-01", "2026-06-19");
  await publicShareSummariesByToken("stintshare_token", "2026-06-01", "2026-06-19");
  globalThis.fetch = originalFetch;

  assertEqual("user-scoped share summaries URL", calls[0]?.url, "/api/v1/users/keith%20stint/share/stintshare_token/summaries?start=2026-06-01&end=2026-06-19");
  assertEqual("token-only share summaries URL", calls[1]?.url, "/api/v1/share/stintshare_token/summaries?start=2026-06-01&end=2026-06-19");
  assertEqual("summaries method defaults to GET", calls[0]?.init?.method, undefined);
}

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}
