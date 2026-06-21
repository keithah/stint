import { logout } from "./api";

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
    status: 204,
    json: async () => ({})
  } as Response;
}) as typeof fetch;

run().catch((error) => {
  globalThis.fetch = originalFetch;
  throw error;
});

async function run() {
  const result = await logout();
  globalThis.fetch = originalFetch;

  assertEqual("logout returns undefined for empty response", result, undefined);
  assertEqual("logout URL", calls[0]?.url, "/auth/logout");
  assertEqual("logout method", calls[0]?.init?.method, "POST");
  assertEqual("logout sends browser credentials", calls[0]?.init?.credentials, "include");
}

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}
