import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from '../lib/api';
import { Schedules } from './Schedules';

function jsonResponse(body: unknown, status = 200): Response {
  return { ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body) } as Response;
}

const schedule = {
  id: 1, name: 'nightly', cron: '0 3 * * *', enabled: true, timezone: 'UTC',
  target_ids: [1], next_run: '2026-09-14T03:00:00Z',
  last_run: { status: 'done', started_at: '2026-09-13T03:00:00Z' },
  created_at: '', updated_at: '',
};
const schedule2 = {
  id: 2, name: 'weekly', cron: '0 4 * * 0', enabled: true, timezone: 'UTC',
  target_ids: [2], next_run: '2026-09-14T04:00:00Z', last_run: null,
  created_at: '', updated_at: '',
};
const target1 = { id: 1, name: 't1', engine: 'fake', enabled: true, lane: 'wan', options: {}, thresholds: {}, created_at: '', updated_at: '' };
const target2 = { id: 2, name: 't2', engine: 'fake', enabled: true, lane: 'wan', options: {}, thresholds: {}, created_at: '', updated_at: '' };

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.startsWith('/api/v1/schedules/validate')) return jsonResponse({ ok: true, next: [] });
    if (url.startsWith('/api/v1/schedules/1/run')) return jsonResponse({ run_id: 9 }, 202);
    if (url.startsWith('/api/v1/schedules')) return jsonResponse({ schedules: [schedule, schedule2] });
    if (url.startsWith('/api/v1/targets')) return jsonResponse([target1, target2]);
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

  it('renders the batched last run without fetching runs per schedule', async () => {
    const listRuns = vi.spyOn(api, 'listRuns');
    wrap();
    expect(await screen.findByText('done')).toBeInTheDocument();
    expect(listRuns).not.toHaveBeenCalled();
  });

  it('runs a schedule now', async () => {
    wrap();
    await screen.findByText('nightly');
    await userEvent.click(screen.getAllByRole('button', { name: 'Run now' })[0]);
    await waitFor(() => expect(
      fetchMock.mock.calls.some(([u, i]) => String(u) === '/api/v1/schedules/1/run' && (i as RequestInit)?.method === 'POST'),
    ).toBe(true));
  });

  it('remounts the form with the right values when switching between edits', async () => {
    wrap();
    await screen.findByText('nightly');
    const editButtons = screen.getAllByRole('button', { name: 'Edit' });

    await userEvent.click(editButtons[0]);
    expect(await screen.findByLabelText('Name')).toHaveValue('nightly');

    await userEvent.click(editButtons[1]);
    await waitFor(() => expect(screen.getByLabelText('Name')).toHaveValue('weekly'));
  });

  it('closes the form and shows warnings once after a create with warnings', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.startsWith('/api/v1/schedules/validate')) return jsonResponse({ ok: true, next: [] });
      if (init?.method === 'POST' && url === '/api/v1/schedules') {
        return jsonResponse({
          schedule: { ...schedule, id: 3, name: 'new-one' },
          warnings: ['overlaps with schedule "nightly" on lane "wan"'],
        }, 201);
      }
      if (url.startsWith('/api/v1/schedules')) return jsonResponse({ schedules: [schedule, schedule2] });
      if (url.startsWith('/api/v1/targets')) return jsonResponse([target1, target2]);
      if (url.startsWith('/api/v1/runs')) return jsonResponse({ runs: [], next_cursor: '' });
      throw new Error(`unexpected fetch: ${url}`);
    });

    wrap();
    await screen.findByText('nightly');
    await userEvent.click(screen.getByRole('button', { name: 'New schedule' }));
    await userEvent.type(screen.getByLabelText('Name'), 'new-one');
    await userEvent.click(await screen.findByRole('button', { name: 'Add t1' }));
    await userEvent.click(screen.getByRole('button', { name: 'Save schedule' }));

    await waitFor(() => expect(screen.queryByRole('button', { name: 'Save schedule' })).not.toBeInTheDocument());
    expect(screen.getAllByText(/overlaps with schedule "nightly"/).length).toBe(1);
  });
});
