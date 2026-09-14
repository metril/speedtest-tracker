import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { LiveRunProvider } from '../features/live/LiveRunProvider';
import { Results } from './Results';

function jsonResponse(body: unknown, status = 200): Response {
  return { ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body) } as Response;
}

const target1 = {
  id: 1, name: 'home', engine: 'fake', enabled: true, lane: 'wan', options: {}, thresholds: {}, created_at: '', updated_at: '',
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

function wrap() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <LiveRunProvider>
        <Results />
      </LiveRunProvider>
    </QueryClientProvider>,
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
