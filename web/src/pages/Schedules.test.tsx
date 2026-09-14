import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Schedules } from './Schedules';

function jsonResponse(body: unknown, status = 200): Response {
  return { ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body) } as Response;
}

const schedule = {
  id: 1, name: 'nightly', cron: '0 3 * * *', enabled: true, timezone: 'UTC',
  target_ids: [1], next_run: '2026-09-14T03:00:00Z', created_at: '', updated_at: '',
};

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.startsWith('/api/v1/schedules/1/run')) return jsonResponse({ run_id: 9 }, 202);
    if (url.startsWith('/api/v1/schedules')) return jsonResponse({ schedules: [schedule] });
    if (url.startsWith('/api/v1/targets')) return jsonResponse([]);
    if (url.startsWith('/api/v1/runs')) {
      return jsonResponse({ runs: [{ id: 4, schedule_id: 1, trigger: 'cron', status: 'done', started_at: '2026-09-13T03:00:00Z', finished_at: '2026-09-13T03:01:00Z', error: null }], next_cursor: '' });
    }
    throw new Error(`unexpected fetch: ${url}`);
  });
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => vi.unstubAllGlobals());

function wrap() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Schedules /></QueryClientProvider>);
}

describe('Schedules page', () => {
  it('lists schedules with their next and last run', async () => {
    wrap();
    expect(await screen.findByText('nightly')).toBeInTheDocument();
    expect(screen.getByText('0 3 * * *')).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText('done')).toBeInTheDocument());
  });

  it('runs a schedule now', async () => {
    wrap();
    await userEvent.click(await screen.findByRole('button', { name: 'Run now' }));
    await waitFor(() => expect(
      fetchMock.mock.calls.some(([u, i]) => String(u) === '/api/v1/schedules/1/run' && (i as RequestInit)?.method === 'POST'),
    ).toBe(true));
  });
});
