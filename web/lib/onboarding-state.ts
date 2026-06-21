export const ONBOARDING_STORAGE_KEY = "stint:onboarding-dismissed:v1";

export function shouldShowOnboarding(totalSeconds: number | undefined, dismissed: boolean) {
  return !dismissed && (totalSeconds ?? 0) === 0;
}
