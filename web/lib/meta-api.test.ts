import { serverMeta } from "./api";

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
    json: async () => ({ data: { api_url: "http://localhost:8080/api/v1", base_url: "http://localhost:8080", hostname: "devbox", ip: "127.0.0.1", version: "dev" } })
  } as Response;
}) as typeof fetch;

run().catch((error) => {
  globalThis.fetch = originalFetch;
  throw error;
});

async function run() {
  const result = await serverMeta();
  globalThis.fetch = originalFetch;

  assertEqual("meta URL", calls[0]?.url, "/api/v1/meta");
  assertEqual("meta method defaults to GET", calls[0]?.init?.method, undefined);
  assertEqual("meta API URL", result.data.api_url, "http://localhost:8080/api/v1");
  assertEqual("meta hostname", result.data.hostname, "devbox");
}

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}
