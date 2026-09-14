import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { LiveRunProvider, useLivePanel } from './LiveRunProvider';

// FakeEventSource lets the test push SSE events, mirroring useLiveRun.test.ts.
class FakeEventSource {
  static last: FakeEventSource | null = null;
  listeners = new Map<string, ((e: MessageEvent) => void)[]>();
  constructor(readonly url: string) {
    FakeEventSource.last = this;
  }
  addEventListener(type: string, fn: (e: MessageEvent) => void) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), fn]);
  }
  removeEventListener() {}
  close() {}
  emit(type: string, data: unknown) {
    for (const fn of this.listeners.get(type) ?? []) fn({ data: JSON.stringify(data) } as MessageEvent);
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

function Probe() {
  const { live, expanded, open, close } = useLivePanel();
  return (
    <div>
      <span data-testid="expanded">{String(expanded)}</span>
      <span data-testid="status">{live?.status ?? 'none'}</span>
      <button onClick={open}>open</button>
      <button onClick={close}>close</button>
    </div>
  );
}

function wrap() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <LiveRunProvider><Probe /></LiveRunProvider>
    </QueryClientProvider>,
  );
}

describe('LiveRunProvider', () => {
  it('does not auto-expand for a run the user did not start', () => {
    wrap();
    act(() => emit('run', { run_id: 1, status: 'running', targets_total: 1, targets_done: 0 }));
    expect(screen.getByTestId('status').textContent).toBe('running');
    expect(screen.getByTestId('expanded').textContent).toBe('false');
  });

  it('expands only via open()', async () => {
    wrap();
    await userEvent.click(screen.getByText('open'));
    expect(screen.getByTestId('expanded').textContent).toBe('true');
  });

  it('resets expanded to false when the run finishes, after the hide delay', async () => {
    vi.useFakeTimers();
    try {
      wrap();
      act(() => { fireEvent.click(screen.getByText('open')); });
      expect(screen.getByTestId('expanded').textContent).toBe('true');

      act(() => emit('run', { run_id: 1, status: 'done', targets_total: 1, targets_done: 1 }));
      expect(screen.getByTestId('expanded').textContent).toBe('true');

      act(() => { vi.advanceTimersByTime(4000); });
      expect(screen.getByTestId('expanded').textContent).toBe('false');
    } finally {
      vi.useRealTimers();
    }
  });
});
