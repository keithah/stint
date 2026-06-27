import { timeoutSignal } from "./http";

class CountingSignal extends EventTarget {
  aborted = false;
  reason: unknown;
  listeners = 0;

  addEventListener(type: string, listener: EventListenerOrEventListenerObject | null, options?: AddEventListenerOptions | boolean) {
    if (type === "abort" && listener) this.listeners++;
    super.addEventListener(type, listener, options);
  }

  removeEventListener(type: string, listener: EventListenerOrEventListenerObject | null, options?: EventListenerOptions | boolean) {
    if (type === "abort" && listener) this.listeners--;
    super.removeEventListener(type, listener, options);
  }
}

const signal = new CountingSignal() as CountingSignal & AbortSignal;
const request = timeoutSignal(1000, signal);

if (signal.listeners !== 1) {
  throw new Error(`expected one abort listener, got ${signal.listeners}`);
}

request.cleanup();
const listenersAfterCleanup: number = signal.listeners;

if (listenersAfterCleanup !== 0) {
  throw new Error(`expected cleanup to remove abort listener, got ${listenersAfterCleanup}`);
}

console.log("http.test.ts passed");
