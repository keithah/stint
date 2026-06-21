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
