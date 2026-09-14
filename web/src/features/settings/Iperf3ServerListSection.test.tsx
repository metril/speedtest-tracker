import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import {
  afterEach, beforeEach, describe, expect, it, vi,
} from 'vitest';
import * as api from '../../lib/api';
import { ApiError } from '../../lib/api';
import { Iperf3ServerListSection } from './Iperf3ServerListSection';

function jsonResponse(body: unknown, status = 200): Response {
  return { ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body) } as Response;
}

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function renderSection(opts: {
  fetchedAt?: string;
  total?: number;
  refresh?: ReturnType<typeof vi.fn>;
} = {}) {
  fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.startsWith('/api/v1/iperf3/servers')) {
      return jsonResponse({ fetched_at: opts.fetchedAt ?? '2026-09-13T12:00:00.000Z', servers: [], total: opts.total ?? 42 });
    }
    throw new Error(`unexpected fetch: ${url}`);
  });
  if (opts.refresh) vi.spyOn(api, 'refreshIperf3Servers').mockImplementation(opts.refresh);

  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrap = (node: ReactNode) => <QueryClientProvider client={qc}>{node}</QueryClientProvider>;
  return render(wrap(<Iperf3ServerListSection />));
}

describe('Iperf3ServerListSection', () => {
  it('shows the last refreshed time and server count', async () => {
    renderSection({ fetchedAt: '2026-09-13T12:00:00.000Z', total: 123 });
    expect(await screen.findByText(/123 servers/)).toBeInTheDocument();
  });

  it('shows "never" when the list has not been fetched yet', async () => {
    renderSection({ fetchedAt: '', total: 0 });
    expect(await screen.findByText(/never/)).toBeInTheDocument();
    expect(screen.getByText(/0 servers/)).toBeInTheDocument();
  });

  it('refreshes on button click and shows the pending state', async () => {
    let resolveRefresh: (v: { fetched_at: string; count: number }) => void = () => {};
    const refresh = vi.fn().mockReturnValue(new Promise((resolve) => { resolveRefresh = resolve; }));
    renderSection({ refresh });

    await userEvent.click(await screen.findByRole('button', { name: 'Refresh' }));
    expect(refresh).toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Refreshing…' })).toBeDisabled();

    resolveRefresh({ fetched_at: '2026-09-14T00:00:00.000Z', count: 55 });
    await waitFor(() => expect(screen.getByRole('button', { name: 'Refresh' })).not.toBeDisabled());
    expect(await screen.findByText(/55 servers/)).toBeInTheDocument();
  });

  it('shows an error message when the refresh fails', async () => {
    const refresh = vi.fn().mockRejectedValue(new ApiError(502, 'refresh_failed', 'upstream unreachable'));
    renderSection({ refresh });

    await userEvent.click(await screen.findByRole('button', { name: 'Refresh' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('upstream unreachable');
  });
});
