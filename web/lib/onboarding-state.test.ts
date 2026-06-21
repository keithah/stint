import { ONBOARDING_STORAGE_KEY, shouldShowOnboarding } from "./onboarding-state";

const _storageKey: string = ONBOARDING_STORAGE_KEY;

if (!shouldShowOnboarding(0, false)) {
  throw new Error("new empty dashboards should show onboarding");
}

if (shouldShowOnboarding(1, false)) {
  throw new Error("dashboards with activity should not show onboarding");
}

if (shouldShowOnboarding(0, true)) {
  throw new Error("dismissed onboarding should stay hidden");
}
