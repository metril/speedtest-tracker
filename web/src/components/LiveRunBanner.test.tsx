import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { act } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { LiveRunBanner } from './LiveRunBanner';

// FakeEventSource mirrors the one in useLiveRun.test.ts so this component
// test can drive the same SSE events end to end.
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

function renderBanner() {
  const client = new QueryClient();
  const spy = vi.spyOn(client, 'invalidateQueries');
  render(
    <QueryClientProvider client={client}>
      <LiveRunBanner />
    </QueryClientProvider>,
  );
  return { spy };
}

beforeEach(() => {
  vi.stubGlobal('EventSource', FakeEventSource);
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  FakeEventSource.last = null;
});

describe('LiveRunBanner', () => {
  it('renders nothing when no run is in flight', () => {
    renderBanner();
    expect(screen.queryByText('Cancel')).not.toBeInTheDocument();
  });

  it('shows the running phase and throughput on a progress event', () => {
    renderBanner();
    act(() => {
      FakeEventSource.last!.emit('progress', {
        run_id: 5, target_id: 2, engine: 'ookla', phase: 'download', progress: 0.4, bps: 94_000_000, ping_ms: 12,
      });
    });
    expect(screen.getByText('download')).toBeInTheDocument();
    expect(screen.getByText('94.0 Mbps')).toBeInTheDocument();
  });

  it('invalidates results/runs/targets caches on a result event', () => {
    const { spy } = renderBanner();
    act(() => {
      FakeEventSource.last!.emit('progress', {
        run_id: 5, target_id: 2, engine: 'ookla', phase: 'download', progress: 0.4, bps: 1, ping_ms: 1,
      });
    });
    spy.mockClear();

    act(() => {
      FakeEventSource.last!.emit('result', { id: 9 });
    });

    expect(spy).toHaveBeenCalledWith({ queryKey: ['results'] });
  });

  it('invalidates results/runs/targets caches when a run reaches a terminal status', () => {
    const { spy } = renderBanner();
    act(() => {
      FakeEventSource.last!.emit('progress', {
        run_id: 5, target_id: 2, engine: 'ookla', phase: 'download', progress: 0.4, bps: 1, ping_ms: 1,
      });
    });
    spy.mockClear();

    act(() => {
      FakeEventSource.last!.emit('run', { run_id: 5, status: 'done' });
    });

    expect(spy).toHaveBeenCalledWith({ queryKey: ['results'] });
    expect(spy).toHaveBeenCalledWith({ queryKey: ['runs'] });
    expect(spy).toHaveBeenCalledWith({ queryKey: ['targets'] });
  });

  it('keeps the banner visible for a grace period after a terminal run, then hides it', () => {
    renderBanner();
    act(() => {
      FakeEventSource.last!.emit('progress', {
        run_id: 5, target_id: 2, engine: 'ookla', phase: 'download', progress: 0.4, bps: 1, ping_ms: 1,
      });
    });
    expect(screen.getByText('download')).toBeInTheDocument();

    act(() => {
      FakeEventSource.last!.emit('run', { run_id: 5, status: 'done' });
    });
    // Still visible immediately after the terminal event.
    expect(screen.getByText('download')).toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(1499);
    });
    expect(screen.getByText('download')).toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(screen.queryByText('download')).not.toBeInTheDocument();
  });

  it('exposes an aria-live region and a labeled progressbar', () => {
    renderBanner();
    act(() => {
      FakeEventSource.last!.emit('progress', {
        run_id: 5, target_id: 2, engine: 'ookla', phase: 'download', progress: 0.4, bps: 1, ping_ms: 1,
      });
    });
    const progress = screen.getByRole('progressbar');
    expect(progress).toHaveAttribute('aria-valuenow', '40');
    expect(progress).toHaveAttribute('aria-valuemin', '0');
    expect(progress).toHaveAttribute('aria-valuemax', '100');
    expect(progress.closest('[aria-live="polite"]')).not.toBeNull();
  });

  it('disables Cancel once the run is no longer live, even during the grace period', () => {
    renderBanner();
    act(() => {
      FakeEventSource.last!.emit('progress', {
        run_id: 5, target_id: 2, engine: 'ookla', phase: 'download', progress: 0.4, bps: 1, ping_ms: 1,
      });
    });
    act(() => {
      FakeEventSource.last!.emit('run', { run_id: 5, status: 'done' });
    });
    expect(screen.getByText('Cancel')).toBeDisabled();
  });
});
