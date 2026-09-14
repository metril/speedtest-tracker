import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useLiveRun } from './useLiveRun';

// FakeEventSource lets the test push events into the hook.
class FakeEventSource {
  static last: FakeEventSource | null = null;
  listeners = new Map<string, ((e: MessageEvent) => void)[]>();
  closed = false;

  constructor(readonly url: string) {
    FakeEventSource.last = this;
  }
  addEventListener(type: string, fn: (e: MessageEvent) => void) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), fn]);
  }
  removeEventListener(type: string, fn: (e: MessageEvent) => void) {
    this.listeners.set(type, (this.listeners.get(type) ?? []).filter((f) => f !== fn));
  }
  close() {
    this.closed = true;
  }
  emit(type: string, data: unknown) {
    for (const fn of this.listeners.get(type) ?? []) {
      fn({ data: JSON.stringify(data) } as MessageEvent);
    }
  }
}

beforeEach(() => {
  vi.stubGlobal('EventSource', FakeEventSource);
});
afterEach(() => {
  vi.unstubAllGlobals();
  FakeEventSource.last = null;
});

describe('useLiveRun', () => {
  it('starts empty and tracks progress events', () => {
    const { result } = renderHook(() => useLiveRun());
    expect(result.current).toBeNull();

    act(() => {
      FakeEventSource.last!.emit('progress', {
        run_id: 5, result_id: 0, target_id: 2, engine: 'ookla',
        phase: 'download', progress: 0.4, bps: 94_000_000, ping_ms: 12.5,
      });
    });

    expect(result.current).toMatchObject({
      runId: 5, targetId: 2, engine: 'ookla', phase: 'download', bps: 94_000_000,
    });
  });

  it('clears when the run reaches a terminal status', () => {
    const { result } = renderHook(() => useLiveRun());
    act(() => {
      FakeEventSource.last!.emit('progress', {
        run_id: 5, target_id: 2, engine: 'fake', phase: 'download', progress: 0.2, bps: 1,
      });
    });
    expect(result.current).not.toBeNull();

    act(() => {
      FakeEventSource.last!.emit('run', { run_id: 5, status: 'done' });
    });
    expect(result.current).toBeNull();
  });

  it('closes the stream on unmount', () => {
    const { unmount } = renderHook(() => useLiveRun());
    const es = FakeEventSource.last!;
    unmount();
    expect(es.closed).toBe(true);
  });

  it('calls onEvent for a result event', () => {
    const onEvent = vi.fn();
    renderHook(() => useLiveRun({ onEvent }));
    act(() => {
      FakeEventSource.last!.emit('result', { id: 1 });
    });
    expect(onEvent).toHaveBeenCalledWith({ type: 'result' });
  });

  it('calls onEvent when a run reaches a terminal status, but not otherwise', () => {
    const onEvent = vi.fn();
    renderHook(() => useLiveRun({ onEvent }));

    act(() => {
      FakeEventSource.last!.emit('run', { run_id: 5, status: 'running' });
    });
    expect(onEvent).not.toHaveBeenCalled();

    act(() => {
      FakeEventSource.last!.emit('run', { run_id: 5, status: 'canceled' });
    });
    expect(onEvent).toHaveBeenCalledWith({ type: 'run', status: 'canceled' });
  });
});
