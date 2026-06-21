import { fileExperts } from "./api";

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
    json: async () => ({
      data: [
        {
          total: { decimal: "0.50", digital: "0:30", text: "30 mins", total_seconds: 1800 },
          user: { id: "user-1", is_current_user: true, long_name: "Local Dev", name: "Local" }
        }
      ]
    })
  } as Response;
}) as typeof fetch;

run().catch((error) => {
  globalThis.fetch = originalFetch;
  throw error;
});

async function run() {
  const result = await fileExperts("/tmp/stint/main.go", "stint");
  globalThis.fetch = originalFetch;

  assertEqual("file experts URL", calls[0]?.url, "/api/v1/users/current/file_experts");
  assertEqual("file experts method", calls[0]?.init?.method, "POST");
  assertEqual("file experts body", calls[0]?.init?.body, JSON.stringify({ entity: "/tmp/stint/main.go", project: "stint" }));
  assertEqual("file experts current user", result.data[0]?.user.is_current_user, true);
  assertEqual("file experts total", result.data[0]?.total.total_seconds, 1800);
}

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}
