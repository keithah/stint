export function compactNumber(value: number) {
  const abs = Math.abs(value);
  if (abs >= 1_000_000_000) {
    return `${trimFixed(value / 1_000_000_000)}B`;
  }
  if (abs >= 1_000_000) {
    return `${trimFixed(value / 1_000_000)}M`;
  }
  if (abs >= 1_000) {
    return `${trimFixed(value / 1_000)}K`;
  }
  return new Intl.NumberFormat("en-US").format(value);
}

function trimFixed(value: number) {
  return value.toFixed(2).replace(/\.?0+$/, "");
}

// Format a dollar amount as USD currency. Sub-dollar amounts keep up to 4
// fraction digits so small per-event/per-token costs stay legible; larger
// amounts use the usual 2 digits.
export function formatUSD(value: number) {
  if (!value || value <= 0) {
    return "$0.00";
  }
  return new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", maximumFractionDigits: value < 1 ? 4 : 2 }).format(value);
}

// Format an integer cent amount as USD currency (always 2 fraction digits).
export function formatCents(cents: number) {
  if (cents <= 0) {
    return "$0.00";
  }
  return new Intl.NumberFormat("en-US", { style: "currency", currency: "USD" }).format(cents / 100);
}
