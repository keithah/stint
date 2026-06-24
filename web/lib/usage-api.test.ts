import { deleteBillingPref, listBillingPrefs, upsertBillingPref, usageExport, usageSummary } from "./usage-api";

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
    json: async () => ({ data: {} })
  } as Response;
}) as typeof fetch;

run().catch((error) => {
  globalThis.fetch = originalFetch;
  throw error;
});

async function run() {
  await usageSummary("last_30_days");
  await usageSummary("last_7_days", "calculate");
  await usageSummary("last_30_days", "auto", "claude code");
  await usageExport("2026-06-01", "2026-06-22");
  await listBillingPrefs();
  await upsertBillingPref({ agent: "claude-code", billing_type: "subscription" });
  await deleteBillingPref("my agent");
  globalThis.fetch = originalFetch;

  assertEqual(
    "summary defaults cost_mode to auto",
    calls[0]?.url,
    "/api/v1/users/current/usage_events/summary?range=last_30_days&cost_mode=auto"
  );
  assertEqual("summary sends credentials", calls[0]?.init?.credentials, "include");
  assertEqual(
    "summary honors explicit cost mode",
    calls[1]?.url,
    "/api/v1/users/current/usage_events/summary?range=last_7_days&cost_mode=calculate"
  );
  assertEqual(
    "summary scopes to a url-encoded agent when provided",
    calls[2]?.url,
    "/api/v1/users/current/usage_events/summary?range=last_30_days&cost_mode=auto&agent=claude+code"
  );
  assertEqual(
    "export passes start and end",
    calls[3]?.url,
    "/api/v1/users/current/usage_events?start=2026-06-01&end=2026-06-22"
  );

  assertEqual(
    "list billing prefs hits the prefs endpoint",
    calls[4]?.url,
    "/api/v1/users/current/billing_prefs"
  );
  assertEqual("upsert billing pref uses PUT", calls[5]?.init?.method, "PUT");
  assertEqual(
    "upsert billing pref sends the pref body",
    calls[5]?.init?.body,
    JSON.stringify({ agent: "claude-code", billing_type: "subscription" })
  );
  assertEqual("delete billing pref uses DELETE", calls[6]?.init?.method, "DELETE");
  assertEqual(
    "delete billing pref encodes the agent in the path",
    calls[6]?.url,
    "/api/v1/users/current/billing_prefs/my%20agent"
  );

  console.log("usage-api.test.ts passed");
}

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}
