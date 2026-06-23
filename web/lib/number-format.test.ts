import { compactNumber, formatCents, formatUSD } from "./number-format";

function assertEqual<T>(name: string, got: T, want: T) {
  if (got !== want) {
    throw new Error(`${name}: expected ${String(want)}, got ${String(got)}`);
  }
}

// compactNumber
assertEqual("compact thousands", compactNumber(1500), "1.5K");
assertEqual("compact millions", compactNumber(2_000_000), "2M");
assertEqual("compact small", compactNumber(42), "42");

// formatUSD: sub-dollar amounts keep up to 4 fraction digits.
assertEqual("usd zero", formatUSD(0), "$0.00");
assertEqual("usd negative clamps", formatUSD(-5), "$0.00");
assertEqual("usd sub-dollar keeps precision", formatUSD(0.0123), "$0.0123");
assertEqual("usd sub-dollar still shows at least two digits", formatUSD(0.5), "$0.50");
assertEqual("usd dollar-and-up uses two digits", formatUSD(12.5), "$12.50");

// formatCents: integer cents, always two fraction digits.
assertEqual("cents zero", formatCents(0), "$0.00");
assertEqual("cents negative clamps", formatCents(-100), "$0.00");
assertEqual("cents to dollars", formatCents(1234), "$12.34");

console.log("number-format.test.ts passed");
