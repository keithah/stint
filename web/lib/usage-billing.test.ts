import { billingBadge, blockProgress, effectiveBilling, hasSubscriptionSavings, subscriptionCovered } from "./usage-billing";

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}

function assertClose(name: string, got: number, want: number, epsilon = 1e-9) {
  if (Math.abs(got - want) > epsilon) {
    throw new Error(`${name}: expected ~${want}, got ${got}`);
  }
}

// effectiveBilling
assertEqual("flat-rate subscription", effectiveBilling({ cost_usd: 10, marginal_usd: 0 }), "subscription");
assertEqual("near-zero marginal still subscription", effectiveBilling({ cost_usd: 10, marginal_usd: 1 }), "subscription");
assertEqual("metered api", effectiveBilling({ cost_usd: 10, marginal_usd: 10 }), "api");
assertEqual("near-full marginal still api", effectiveBilling({ cost_usd: 10, marginal_usd: 9 }), "api");
assertEqual("partial overage is mixed", effectiveBilling({ cost_usd: 10, marginal_usd: 5 }), "mixed");
assertEqual("no equivalent cost is free", effectiveBilling({ cost_usd: 0, marginal_usd: 0 }), "free");
assertEqual("negative clamps to free", effectiveBilling({ cost_usd: -3, marginal_usd: -3 }), "free");

// billingBadge
assertEqual("subscription badge label", billingBadge({ cost_usd: 10, marginal_usd: 0 }).label, "Subscription");
assertEqual("api badge kind", billingBadge({ cost_usd: 10, marginal_usd: 10 }).kind, "api");
if (!billingBadge({ cost_usd: 10, marginal_usd: 0 }).className.includes("moss")) {
  throw new Error("subscription badge should use the moss accent");
}

// hasSubscriptionSavings + subscriptionCovered
assertEqual("savings present", hasSubscriptionSavings({ cost_usd: 10, marginal_usd: 2 }), true);
assertEqual("no savings when equal", hasSubscriptionSavings({ cost_usd: 10, marginal_usd: 10 }), false);
assertEqual("tiny gap ignored", hasSubscriptionSavings({ cost_usd: 10.004, marginal_usd: 10 }), false);
assertClose("covered amount", subscriptionCovered({ cost_usd: 10, marginal_usd: 2 }), 8);
assertClose("covered never negative", subscriptionCovered({ cost_usd: 2, marginal_usd: 10 }), 0);

// blockProgress
const mid = blockProgress({ elapsed_minutes: 150 });
assertClose("halfway fraction", mid.fraction, 0.5);
assertClose("halfway percent", mid.percent, 50);
assertEqual("remaining minutes", mid.remainingMinutes, 150);
assertEqual("total minutes default 300", mid.totalMinutes, 300);

const over = blockProgress({ elapsed_minutes: 999 });
assertClose("over-running clamps to full", over.fraction, 1);
assertEqual("over-running has no negative remainder", over.remainingMinutes, 0);

const negative = blockProgress({ elapsed_minutes: -10 });
assertClose("negative elapsed clamps to zero", negative.fraction, 0);

console.log("usage-billing.test.ts passed");
