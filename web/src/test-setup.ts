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

// jsdom has no ResizeObserver, which @tanstack/react-virtual optionally uses
// to react to container resizes. A no-op keeps virtualised tables mountable.
if (typeof globalThis.ResizeObserver === 'undefined') {
  class NoopResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  globalThis.ResizeObserver = NoopResizeObserver as unknown as typeof ResizeObserver;
}
