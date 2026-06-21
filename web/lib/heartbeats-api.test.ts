import { deleteHeartbeats, heartbeatsForDay } from "./api";

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
  await heartbeatsForDay("2026-06-19");
  await deleteHeartbeats("2026-06-19", ["hb-1", "hb-2"]);
  globalThis.fetch = originalFetch;

  assertEqual("heartbeat list URL", calls[0]?.url, "/api/v1/users/current/heartbeats?date=2026-06-19");
  assertEqual("heartbeat list method defaults to GET", calls[0]?.init?.method, undefined);
  assertEqual("heartbeat delete URL", calls[1]?.url, "/api/v1/users/current/heartbeats.bulk");
  assertEqual("heartbeat delete method", calls[1]?.init?.method, "DELETE");
  assertEqual("heartbeat delete body", calls[1]?.init?.body, JSON.stringify({ date: "2026-06-19", ids: ["hb-1", "hb-2"] }));
}

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}
