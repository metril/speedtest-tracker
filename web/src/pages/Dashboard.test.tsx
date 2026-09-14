import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { Dashboard } from './Dashboard';

/** jsonResponse builds a fetch Response-shaped stub the same way the rest
 * of this codebase's tests do (see Targets.test.tsx). */
function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body),
  } as Response;
}

const settings = {
  general: {
    base_url: '', timezone: 'UTC', units: 'Mbps', log_level: 'info',
    retention_days_results: 90, retention_days_runs: 30, retention_prune_interval_minutes: 60,
    sla_download_mbps: 1000, sla_upload_mbps: 50,
  },
  engines: {}, integrations: {}, notifications: {}, auth: {}, locked: [],
};

const target = {
  id: 1, name: 'home', engine: 'fake', enabled: true, lane: 'wan',
  options: {}, thresholds: {}, created_at: '', updated_at: '',
};

const summary = {
  from: '2026-09-13T00:00:00.000Z', to: '2026-09-14T00:00:00.000Z',
  targets: [{
    target_id: 1, target_name: 'home', engine: 'fake', latest: null,
    count: 5, fail_count: 0, success_rate: 1,
    avg_download_bps: 100e6, min_download_bps: 90e6, max_download_bps: 110e6,
    avg_upload_bps: 20e6, avg_ping_ms: 10, max_ping_ms: 15, sla_compliance: null,
  }],
  total_results: 5, total_failures: 0, success_rate: 1, sla_compliance: null,
};

const history = {
  target_id: 1, from: '', to: '', bucket_seconds: 3600,
  points: [{
    bucket_start: '2026-09-13T10:00:00.000Z', count: 1, fail_count: 0,
    avg_download_bps: 100e6, min_download_bps: 100e6, max_download_bps: 100e6,
    avg_upload_bps: 20e6, min_upload_bps: 20e6, max_upload_bps: 20e6,
    avg_ping_ms: 10, min_ping_ms: 10, max_ping_ms: 10, avg_jitter_ms: 1,
  }],
};

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.includes('/stats/summary')) return jsonResponse(summary);
    if (url.includes('/targets/1/history')) return jsonResponse(history);
    if (url.includes('/targets')) return jsonResponse([target]);
    if (url.includes('/outages')) return jsonResponse({ from: '', to: '', incidents: [] });
    if (url.includes('/settings')) return jsonResponse(settings);
    throw new Error(`unexpected fetch: ${url}`);
  });
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => {
  vi.unstubAllGlobals();
});

function renderDashboard() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <Dashboard />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

it('does not fetch a previous-period history until "Compare with previous period" is toggled on', async () => {
  renderDashboard();
  await screen.findByText('home');
  fetchMock.mockClear();

  await userEvent.click(screen.getByLabelText('Compare with previous period'));

  await waitFor(() => expect(fetchMock).toHaveBeenCalled());
  const urls = fetchMock.mock.calls.map(([input]) => String(input));
  expect(urls.some((u) => u.includes('/targets/1/history') && u.includes('offset=1'))).toBe(true);
});
