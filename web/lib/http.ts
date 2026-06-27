export function timeoutSignal(timeoutMs: number, signal?: AbortSignal) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(new Error("Request timed out")), timeoutMs);

  const cleanup = () => {
    clearTimeout(timeout);
    signal?.removeEventListener("abort", abortFromSignal);
  };
  const abortFromSignal = () => {
    cleanup();
    controller.abort(signal?.reason);
  };

  if (signal?.aborted) {
    abortFromSignal();
  } else {
    signal?.addEventListener("abort", abortFromSignal, { once: true });
  }

  return { signal: controller.signal, cleanup };
}
