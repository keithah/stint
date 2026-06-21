import { boundedPercent, minimumVisiblePercent } from "./chart-percent";

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}

assertEqual("bounded percent rejects negative values", boundedPercent(-15), 0);
assertEqual("bounded percent rounds fractional values", boundedPercent(44.6), 45);
assertEqual("bounded percent clamps overflow", boundedPercent(135), 100);
assertEqual("bounded percent rejects non-finite values", boundedPercent(Number.POSITIVE_INFINITY), 0);

assertEqual("minimum visible percent preserves zero", minimumVisiblePercent(0), 0);
assertEqual("minimum visible percent raises positive slivers", minimumVisiblePercent(1), 3);
assertEqual("minimum visible percent clamps overflow", minimumVisiblePercent(250), 100);
