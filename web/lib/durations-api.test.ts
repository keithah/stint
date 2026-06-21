import { durationsForDay } from "./api";

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
    json: async () => ({ data: [{ name: "stint", time: 1781887600, duration: 600 }] })
  } as Response;
}) as typeof fetch;

run().catch((error) => {
  globalThis.fetch = originalFetch;
  throw error;
});

async function run() {
  const result = await durationsForDay("2026-06-19", "language");
  globalThis.fetch = originalFetch;

  assertEqual("durations URL", calls[0]?.url, "/api/v1/users/current/durations?date=2026-06-19&slice_by=language");
  assertEqual("durations method defaults to GET", calls[0]?.init?.method, undefined);
  assertEqual("duration name", result.data[0]?.name, "stint");
  assertEqual("duration seconds", result.data[0]?.duration, 600);
}

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}
