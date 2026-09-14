import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { LiveRun } from '../../lib/useLiveRun';
import { LivePanel } from './LivePanel';
import { LivePanelContext } from './LiveRunProvider';

function live(overrides: Partial<LiveRun> = {}): LiveRun {
  return {
    runId: 1, targetId: 2, resultId: 0, engine: 'ookla', phase: 'download',
    progress: 0.4, bps: 240_000_000, pingMs: 8.2, jitterMs: 1.1, lossPct: 0,
    serverName: 'Init7 Zurich', isp: 'Init7', status: 'running',
    targetsTotal: 2, targetsDone: 1, samples: [1e6, 2e6, 3e6], finished: false,
    ...overrides,
  };
}

function renderPanel(value: Parameters<typeof LivePanelContext.Provider>[0]['value']) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <LivePanelContext.Provider value={value}>
        <LivePanel />
      </LivePanelContext.Provider>
    </QueryClientProvider>,
  );
}

describe('LivePanel', () => {
  it('renders nothing when no run is live', () => {
    const { container } = renderPanel({ live: null, expanded: false, open: vi.fn(), close: vi.fn() });
    expect(container).toBeEmptyDOMElement();
  });

  it('shows the gauge, server line and stepper when expanded', () => {
    renderPanel({ live: live(), expanded: true, open: vi.fn(), close: vi.fn() });
    expect(screen.getByRole('dialog', { name: /live test/i })).toBeInTheDocument();
    expect(screen.getByRole('meter')).toHaveAttribute('aria-valuenow', '240');
    expect(screen.getByText(/Init7 Zurich/)).toBeInTheDocument();
    expect(screen.getByText('Target 2 of 2')).toBeInTheDocument();
  });

  it('shows a compact bar instead of a dialog when collapsed', () => {
    renderPanel({ live: live(), expanded: false, open: vi.fn(), close: vi.fn() });
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(screen.getByRole('button', { name: /expand live test/i })).toBeInTheDocument();
  });

  it('scopes the live region to the phase text, not the throughput readout', () => {
    const { container } = renderPanel({ live: live(), expanded: false, open: vi.fn(), close: vi.fn() });
    const liveRegion = container.querySelector('[aria-live="polite"]');
    expect(liveRegion).not.toBeNull();
    expect(liveRegion).toHaveTextContent('download');
    expect(liveRegion).not.toHaveTextContent('Mbps');
    expect(screen.getByText(/Mbps/)).not.toHaveAttribute('aria-live');
    expect(screen.getByText(/Mbps/).closest('[aria-live]')).toBeNull();
  });

  it('cancels the run', async () => {
    const fetchMock = vi.fn(async (..._args: unknown[]) => ({ ok: true, status: 202, statusText: 'ok', text: async () => '{}' }) as Response);
    vi.stubGlobal('fetch', fetchMock);
    renderPanel({ live: live(), expanded: true, open: vi.fn(), close: vi.fn() });
    await userEvent.click(screen.getByRole('button', { name: 'Cancel run' }));
    expect(String(fetchMock.mock.calls[0][0])).toBe('/api/v1/runs/1');
    vi.unstubAllGlobals();
  });

  it('links to the result once the run has finished', () => {
    renderPanel({
      live: live({ finished: true, status: 'done', phase: 'done', resultId: 42 }),
      expanded: true, open: vi.fn(), close: vi.fn(),
    });
    expect(screen.getByRole('link', { name: /view result/i })).toHaveAttribute('href', '/results?result_id=42');
  });
});
