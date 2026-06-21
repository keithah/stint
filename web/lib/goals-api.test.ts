import { getGoal } from "./api";

type FetchCall = {
  url: string;
  init?: RequestInit;
};

const calls: FetchCall[] = [];
const originalFetch = globalThis.fetch;
const originalLocalStorage = globalThis.localStorage;

Object.defineProperty(globalThis, "localStorage", {
  configurable: true,
  value: { getItem: () => null }
});

globalThis.fetch = (async (url: string | URL | Request, init?: RequestInit) => {
  calls.push({ url: String(url), init });
  return {
    ok: true,
    status: 200,
    json: async () => ({
      data: {
        id: "goal-123",
        title: "Daily Coding",
        delta: "day",
        seconds: 3600,
        ignore_zero_days: false,
        is_enabled: true,
        is_inverse: false,
        is_snoozed: false
      }
    })
  } as Response;
}) as typeof fetch;

async function run() {
  const result = await getGoal("goal 123");
  if (calls[0]?.url !== "/api/v1/users/current/goals/goal%20123") {
    throw new Error(`expected encoded single-goal URL, got ${calls[0]?.url}`);
  }
  if (result.data.id !== "goal-123") {
    throw new Error(`expected goal payload, got ${JSON.stringify(result)}`);
  }
}

run()
  .finally(() => {
    globalThis.fetch = originalFetch;
    Object.defineProperty(globalThis, "localStorage", { configurable: true, value: originalLocalStorage });
  })
  .catch((error) => {
    throw error;
  });
