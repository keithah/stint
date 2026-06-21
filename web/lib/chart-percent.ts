export function boundedPercent(value: number) {
  if (!Number.isFinite(value)) {
    return 0;
  }
  return Math.max(0, Math.min(100, Math.round(value)));
}

export function minimumVisiblePercent(value: number, minimum = 3) {
  const percent = boundedPercent(value);
  if (percent <= 0) {
    return 0;
  }
  return Math.max(minimum, percent);
}
