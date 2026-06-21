import { aiRingStyle } from "./ai-ring";

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}

assertEqual("ring clamps negative percentages", aiRingStyle(-10).background, "conic-gradient(#00b4d8 0deg, #27272a 0deg)");
assertEqual("ring converts percentage to degrees", aiRingStyle(25).background, "conic-gradient(#00b4d8 90deg, #27272a 90deg)");
assertEqual("ring clamps percentages above 100", aiRingStyle(150).background, "conic-gradient(#00b4d8 360deg, #27272a 360deg)");
