import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { History, HistoryPoint } from '../lib/api';
import { Dashboard, mergePrevByOffset } from './Dashboard';

/** point builds a minimal HistoryPoint for mergePrevByOffset tests -- only
 * bucket_start and the one metric under test matter. */
function point(bucketStart: string, download: number): HistoryPoint {
  return {
    bucket_start: bucketStart, count: 1, fail_count: 0,
    avg_download_bps: download, min_download_bps: download, max_download_bps: download,
    avg_upload_bps: 0, min_upload_bps: 0, max_upload_bps: 0,
    avg_ping_ms: 0, min_ping_ms: 0, max_ping_ms: 0, avg_jitter_ms: 0,
  };
}

function historyOf(from: string, bucketSeconds: number, points: HistoryPoint[]): History {
  return { target_id: 1, from, to: '', bucket_seconds: bucketSeconds, points };
}

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
  id: 1, name: 'home', engine: 'fake', enabled: true, queue_id: 1, queue_name: 'wan',
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

  const urlsBeforeToggle = fetchMock.mock.calls.map(([input]) => String(input));
  expect(urlsBeforeToggle.some((u) => u.includes('/targets/1/history') && u.includes('offset=1'))).toBe(false);

  fetchMock.mockClear();
  await userEvent.click(screen.getByLabelText('Compare with previous period'));

  await waitFor(() => expect(fetchMock).toHaveBeenCalled());
  const urlsAfterToggle = fetchMock.mock.calls.map(([input]) => String(input));
  expect(urlsAfterToggle.some((u) => u.includes('/targets/1/history') && u.includes('offset=1'))).toBe(true);
});

describe('mergePrevByOffset', () => {
  const bucketSeconds = 3600;
  const currentFrom = '2026-09-13T00:00:00.000Z';

  it('a gap in the previous window does not shift later points', () => {
    // Current has slots 0, 1, 2. Previous is missing slot 1 (a gap) but
    // has slots 0 and 2 -- the same offset (one hour before current, per
    // useAllTargetHistories' offset: 1) shifted back by the window span.
    const rows = [
      { bucket_start: '2026-09-13T00:00:00.000Z', avg_download_bps_1: 10 },
      { bucket_start: '2026-09-13T01:00:00.000Z', avg_download_bps_1: 20 },
      { bucket_start: '2026-09-13T02:00:00.000Z', avg_download_bps_1: 30 },
    ];
    const currentHistories = new Map([[1, historyOf(currentFrom, bucketSeconds, [])]]);
    const prevFrom = '2026-09-12T00:00:00.000Z';
    const prevHistories = new Map([[1, historyOf(prevFrom, bucketSeconds, [
      point('2026-09-12T00:00:00.000Z', 100), // slot 0
      // slot 1 missing: a gap
      point('2026-09-12T02:00:00.000Z', 300), // slot 2
    ])]]);

    const merged = mergePrevByOffset(rows, currentHistories, prevHistories, [1], ['avg_download_bps']);

    expect(merged[0].avg_download_bps_1_prev).toBe(100);
    expect(merged[1].avg_download_bps_1_prev).toBeUndefined();
    // The gap must not shift slot 2's previous point onto slot 1's row.
    expect(merged[2].avg_download_bps_1_prev).toBe(300);
  });

  it('targets with differing point counts each align by their own slot', () => {
    // Target 1 has 3 current slots; target 2 only has slot 2 (e.g. it
    // joined later). Each target's previous overlay must use its own
    // slot, not the other target's count or the merged row's position.
    const rows: Record<string, number | string>[] = [
      { bucket_start: '2026-09-13T00:00:00.000Z', avg_download_bps_1: 10 },
      { bucket_start: '2026-09-13T01:00:00.000Z', avg_download_bps_1: 20 },
      { bucket_start: '2026-09-13T02:00:00.000Z', avg_download_bps_1: 30, avg_download_bps_2: 5 },
    ];
    const currentHistories = new Map([
      [1, historyOf(currentFrom, bucketSeconds, [])],
      [2, historyOf(currentFrom, bucketSeconds, [])],
    ]);
    const prevFrom = '2026-09-12T00:00:00.000Z';
    const prevHistories = new Map([
      [1, historyOf(prevFrom, bucketSeconds, [
        point('2026-09-12T00:00:00.000Z', 100),
        point('2026-09-12T01:00:00.000Z', 200),
        point('2026-09-12T02:00:00.000Z', 300),
      ])],
      [2, historyOf(prevFrom, bucketSeconds, [
        point('2026-09-12T02:00:00.000Z', 50), // only slot 2 has data
      ])],
    ]);

    const merged = mergePrevByOffset(rows, currentHistories, prevHistories, [1, 2], ['avg_download_bps']);

    expect(merged[2].avg_download_bps_1_prev).toBe(300);
    expect(merged[2].avg_download_bps_2_prev).toBe(50);
  });
});
