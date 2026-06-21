export function aiRingStyle(percentage: number) {
  const clamped = Math.max(0, Math.min(100, Math.round(percentage)));
  const degrees = Math.round((clamped / 100) * 360);
  return {
    background: `conic-gradient(#00b4d8 ${degrees}deg, #27272a ${degrees}deg)`
  };
}
