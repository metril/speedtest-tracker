import '@testing-library/jest-dom/vitest';

// jsdom has no EventSource; components that open one (e.g. useLiveRun) need a
// harmless stand-in so unrelated tests that merely mount them don't crash.
// Tests exercising SSE behavior stub their own via vi.stubGlobal.
if (typeof globalThis.EventSource === 'undefined') {
  class NoopEventSource {
    constructor(readonly url: string) {}
    addEventListener() {}
    removeEventListener() {}
    close() {}
  }
  // @ts-expect-error -- minimal stand-in, not a full EventSource implementation
  globalThis.EventSource = NoopEventSource;
}
