import { listEditors } from "./api";

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
    json: async () => ({ data: [{ name: "VS Code", key: "vscode", version: "1.89+" }] })
  } as Response;
}) as typeof fetch;

run().catch((error) => {
  globalThis.fetch = originalFetch;
  throw error;
});

async function run() {
  const result = await listEditors();
  globalThis.fetch = originalFetch;

  assertEqual("editors URL", calls[0]?.url, "/api/v1/editors");
  assertEqual("editors method defaults to GET", calls[0]?.init?.method, undefined);
  assertEqual("editor name", result.data[0]?.name, "VS Code");
  assertEqual("editor key", result.data[0]?.key, "vscode");
  assertEqual("editor version", result.data[0]?.version, "1.89+");
}

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}
