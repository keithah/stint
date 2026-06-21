import { createExternalDurationsBulk, deleteExternalDurationsBulk, type ExternalDuration } from "./api";

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
    json: async () => ({ data: { ok: true } })
  } as Response;
}) as typeof fetch;

run().catch((error) => {
  globalThis.fetch = originalFetch;
  throw error;
});

async function run() {
  const duration: Omit<ExternalDuration, "id" | "created_at"> = {
    external_id: "calendar-1",
    provider: "calendar",
    entity: "Design review",
    type: "meeting",
    start_time: 1781887600,
    end_time: 1781889400,
    project: "stint"
  };

  await createExternalDurationsBulk([duration]);
  await deleteExternalDurationsBulk(["11111111-1111-4111-8111-111111111111"]);

  globalThis.fetch = originalFetch;

  assertEqual("bulk create URL", calls[0]?.url, "/api/v1/users/current/external_durations.bulk");
  assertEqual("bulk create method", calls[0]?.init?.method, "POST");
  assertEqual("bulk create body", calls[0]?.init?.body, JSON.stringify([duration]));
  assertEqual("bulk delete URL", calls[1]?.url, "/api/v1/users/current/external_durations.bulk");
  assertEqual("bulk delete method", calls[1]?.init?.method, "DELETE");
  assertEqual("bulk delete body", calls[1]?.init?.body, JSON.stringify({ ids: ["11111111-1111-4111-8111-111111111111"] }));
}

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}
