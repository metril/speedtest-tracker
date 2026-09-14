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

function emit(type: string, data: unknown) {
  FakeEventSource.last!.emit(type, data);
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
      emit('progress', {
        run_id: 5, result_id: 0, target_id: 2, engine: 'ookla',
        phase: 'download', progress: 0.4, bps: 94_000_000, ping_ms: 12.5,
      });
    });

    expect(result.current).toMatchObject({
      runId: 5, targetId: 2, engine: 'ookla', phase: 'download', bps: 94_000_000,
    });
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
      emit('result', { id: 1, target_id: 2 });
    });
    expect(onEvent).toHaveBeenCalledWith(expect.objectContaining({
      type: 'result', result: expect.objectContaining({ id: 1 }),
    }));
  });

  it('calls onEvent when a run reaches a terminal status, but not otherwise', () => {
    const onEvent = vi.fn();
    renderHook(() => useLiveRun({ onEvent }));

    act(() => {
      emit('run', { run_id: 5, status: 'running', targets_total: 1, targets_done: 0 });
    });
    expect(onEvent).not.toHaveBeenCalled();

    act(() => {
      emit('run', { run_id: 5, status: 'canceled', targets_total: 1, targets_done: 1 });
    });
    expect(onEvent).toHaveBeenCalledWith(expect.objectContaining({ type: 'run', status: 'canceled' }));
  });

  it('keeps the finished run visible and marks it finished', async () => {
    const { result } = renderHook(() => useLiveRun());
    act(() => emit('progress', { run_id: 1, target_id: 2, engine: 'fake', phase: 'download', progress: 0.5, bps: 50e6, ping_ms: 9 }));
    act(() => emit('run', { run_id: 1, status: 'done', targets_total: 1, targets_done: 1 }));
    expect(result.current?.finished).toBe(true);
    expect(result.current?.status).toBe('done');
    expect(result.current?.bps).toBe(50e6);
  });

  it('accumulates at most 60 throughput samples and resets them per phase', () => {
    const { result } = renderHook(() => useLiveRun());
    act(() => {
      for (let i = 0; i < 70; i += 1) {
        emit('progress', { run_id: 1, target_id: 2, engine: 'fake', phase: 'download', progress: 0.5, bps: (i + 1) * 1e6, ping_ms: 9 });
      }
    });
    expect(result.current?.samples).toHaveLength(60);
    expect(result.current?.samples.at(-1)).toBe(70e6);
    act(() => emit('progress', { run_id: 1, target_id: 2, engine: 'fake', phase: 'upload', progress: 0.1, bps: 5e6, ping_ms: 9 }));
    expect(result.current?.samples).toEqual([5e6]);
  });

  it('carries the stepper counts from run events', () => {
    const { result } = renderHook(() => useLiveRun());
    act(() => emit('run', { run_id: 3, status: 'running', targets_total: 3, targets_done: 1 }));
    act(() => emit('progress', { run_id: 3, target_id: 9, engine: 'fake', phase: 'ping', progress: 0.2, bps: 0, ping_ms: 12 }));
    expect(result.current?.targetsTotal).toBe(3);
    expect(result.current?.targetsDone).toBe(1);
  });

  it('hands the full result row to onEvent', () => {
    const onEvent = vi.fn();
    renderHook(() => useLiveRun({ onEvent }));
    act(() => emit('result', { id: 5, target_id: 2, download_bps: 1 }));
    expect(onEvent).toHaveBeenCalledWith(expect.objectContaining({
      type: 'result', result: expect.objectContaining({ id: 5 }),
    }));
  });

  it('ignores a queued event for a different run while one is in flight', () => {
    const { result } = renderHook(() => useLiveRun());
    act(() => emit('run', { run_id: 1, status: 'running', targets_total: 1, targets_done: 0 }));
    act(() => emit('progress', { run_id: 1, target_id: 2, engine: 'fake', phase: 'download', progress: 0.5, bps: 10e6, ping_ms: 9 }));

    act(() => emit('run', { run_id: 2, status: 'queued', targets_total: 1, targets_done: 0 }));

    expect(result.current?.runId).toBe(1);
    expect(result.current?.bps).toBe(10e6);
  });

  it('switches to a different run once it starts running or reaches a terminal status', () => {
    const { result } = renderHook(() => useLiveRun());
    act(() => emit('run', { run_id: 1, status: 'running', targets_total: 1, targets_done: 0 }));

    act(() => emit('run', { run_id: 2, status: 'running', targets_total: 1, targets_done: 0 }));
    expect(result.current?.runId).toBe(2);
  });

  it('resets isp and serverName when the target changes within the same run', () => {
    const { result } = renderHook(() => useLiveRun());
    act(() => emit('progress', { run_id: 1, target_id: 2, engine: 'fake', phase: 'download', progress: 0.5, bps: 10e6, ping_ms: 9, server_name: 'fra' }));
    act(() => emit('result', { id: 1, target_id: 2, isp: 'Comcast', server_name: 'fra' }));
    expect(result.current?.isp).toBe('Comcast');

    act(() => emit('progress', { run_id: 1, target_id: 3, engine: 'fake', phase: 'ping', progress: 0.1, bps: 0, ping_ms: 5 }));
    expect(result.current?.isp).toBe('');
    expect(result.current?.serverName).toBe('');
  });

  it('marks the run stale after 60s without any event', () => {
    vi.useFakeTimers();
    try {
      const { result } = renderHook(() => useLiveRun());
      act(() => emit('progress', { run_id: 1, target_id: 2, engine: 'fake', phase: 'download', progress: 0.5, bps: 10e6, ping_ms: 9 }));
      expect(result.current?.finished).toBe(false);

      act(() => { vi.advanceTimersByTime(60_000); });

      expect(result.current?.finished).toBe(true);
      expect(result.current?.status).toBe('stale');
    } finally {
      vi.useRealTimers();
    }
  });

  it('does not go stale if events keep arriving', () => {
    vi.useFakeTimers();
    try {
      const { result } = renderHook(() => useLiveRun());
      act(() => emit('progress', { run_id: 1, target_id: 2, engine: 'fake', phase: 'download', progress: 0.5, bps: 10e6, ping_ms: 9 }));
      act(() => { vi.advanceTimersByTime(45_000); });
      act(() => emit('progress', { run_id: 1, target_id: 2, engine: 'fake', phase: 'download', progress: 0.6, bps: 11e6, ping_ms: 9 }));
      act(() => { vi.advanceTimersByTime(45_000); });

      expect(result.current?.finished).toBe(false);
    } finally {
      vi.useRealTimers();
    }
  });

  it('does not go stale once the run already finished normally', () => {
    vi.useFakeTimers();
    try {
      const { result } = renderHook(() => useLiveRun());
      act(() => emit('run', { run_id: 1, status: 'done', targets_total: 1, targets_done: 1 }));
      act(() => { vi.advanceTimersByTime(60_000); });

      expect(result.current?.status).toBe('done');
    } finally {
      vi.useRealTimers();
    }
  });
});
