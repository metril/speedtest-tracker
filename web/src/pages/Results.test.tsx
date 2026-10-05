import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { LiveControlsContext, LiveRunProvider } from '../features/live/LiveRunProvider';
import { Results } from './Results';

function jsonResponse(body: unknown, status = 200): Response {
  return { ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body) } as Response;
}

const target1 = {
  id: 1, name: 'home', engine: 'fake', enabled: true, queue_id: 1, queue_name: 'wan', options: {}, thresholds: {}, created_at: '', updated_at: '',
};

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.startsWith('/api/v1/targets')) return jsonResponse([target1]);
    if (url.startsWith('/api/v1/tags')) return jsonResponse([]);
    if (url.startsWith('/api/v1/results')) return jsonResponse({ results: [], next_cursor: '' });
    throw new Error(`unexpected fetch: ${url}`);
  });
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => vi.unstubAllGlobals());

function Location() {
  return <span data-testid="search">{useLocation().search}</span>;
}

function wrap(initial = '/') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <MemoryRouter initialEntries={[initial]}>
      <QueryClientProvider client={qc}>
        <LiveRunProvider>
          <Results />
          <Location />
        </LiveRunProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  );
}

it('offers a CSV export link carrying the active filters', async () => {
  wrap();
  await screen.findByText('home');
  await userEvent.selectOptions(screen.getByLabelText('Target'), '1');
  await userEvent.selectOptions(screen.getByLabelText('Status'), 'ok');

  const link = screen.getByRole('link', { name: /export csv/i });
  expect(link).toHaveAttribute('href', expect.stringContaining('/api/v1/results.csv?'));
  expect(link.getAttribute('href')).toContain('status=ok');
});

it('opens the drawer for the run the re-execute created, not an id-less open', async () => {
  vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(600);
  vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(600);
  const result = {
    id: 1, run_id: 5, target_id: 1, target_name: 'home', engine: 'ookla',
    options_snapshot: {}, status: 'ok', started_at: new Date().toISOString(),
    duration_ms: 1000, download_bps: 1e8, upload_bps: 2e7, ping_ms: 10, jitter_ms: 1,
    packet_loss_pct: 0, server_name: 'srv', server_host: 'host', isp: 'isp', result_url: '', tags: [],
  };
  fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url.startsWith('/api/v1/targets')) return jsonResponse([target1]);
    if (url.startsWith('/api/v1/tags')) return jsonResponse([]);
    if (url.includes('/reexecute') && init?.method === 'POST') return jsonResponse({ run_id: 7 });
    if (url.startsWith('/api/v1/results')) return jsonResponse({ results: [result], next_cursor: '' });
    throw new Error(`unexpected fetch: ${url}`);
  });

  const open = vi.fn();
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <MemoryRouter>
      <QueryClientProvider client={qc}>
        <LiveControlsContext.Provider value={{ expanded: false, hidden: false, open, close: vi.fn() }}>
          <Results />
        </LiveControlsContext.Provider>
      </QueryClientProvider>
    </MemoryRouter>,
  );

  await userEvent.click(await screen.findByRole('button', { name: /replay result/i }));
  await waitFor(() => expect(open).toHaveBeenCalledWith(7));
});

const okResult = {
  id: 1, run_id: 5, target_id: 1, target_name: 'home', engine: 'ookla',
  options_snapshot: {}, status: 'ok', started_at: new Date().toISOString(),
  duration_ms: 1000, download_bps: 1e8, upload_bps: 2e7, ping_ms: 10, jitter_ms: 1,
  loss_pct: 0, server_name: 's', isp: 'i', tags: [], error: '',
};

function withResults(extra?: (url: string, init?: RequestInit) => Response | undefined) {
  vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(600);
  vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(600);
  fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const custom = extra?.(url, init);
    if (custom) return custom;
    if (url.startsWith('/api/v1/targets')) return jsonResponse([target1]);
    if (url.startsWith('/api/v1/tags')) return jsonResponse([]);
    if (url.startsWith('/api/v1/results')) return jsonResponse({ results: [okResult], next_cursor: '' });
    throw new Error(`unexpected fetch: ${url}`);
  });
}

it('highlights the row named by result_id and clears the param', async () => {
  withResults();
  wrap('/?result_id=1');
  const row = (await screen.findByText('home', { selector: 'span.truncate' })).closest('[role="row"]')!;
  await waitFor(() => expect(row).toHaveAttribute('data-highlighted', 'true'));
  expect(screen.getByTestId('search').textContent).toBe('');
});

it('shows an error when deleting a result fails', async () => {
  withResults((url, init) => (url.startsWith('/api/v1/results/1') && init?.method === 'DELETE'
    ? jsonResponse({ error: { code: 'internal', message: 'disk on fire' } }, 500) : undefined));
  wrap();
  await userEvent.click(await screen.findByRole('button', { name: /delete result/i }));
  await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: 'Delete' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('disk on fire');
});

it('names the target on the tag button and cancels tag editing with Escape', async () => {
  withResults();
  wrap();
  await userEvent.click(await screen.findByRole('button', { name: 'Edit tags for home' }));
  const input = screen.getByRole('textbox', { name: 'Tags' });
  await userEvent.type(input, 'x{Escape}');
  expect(screen.queryByRole('textbox', { name: 'Tags' })).toBeNull();
  expect(fetchMock.mock.calls.some(([, init]) => (init as RequestInit | undefined)?.method === 'PUT')).toBe(false);
});
