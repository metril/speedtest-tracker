import { afterEach, describe, expect, it, vi } from 'vitest';
import * as api from './api';
import { ApiError, deleteTarget, listTargets } from './api';

function mockFetch(status: number, body: unknown) {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    statusText: 'error',
    text: async () => (body === undefined ? '' : JSON.stringify(body)),
  });
}

/** jsonResponse builds a fetch Response-shaped stub the same way the rest
 * of this codebase's tests do (see Targets.test.tsx); cast to Response
 * since only the fields api.ts's request() reads are provided. */
function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body),
  } as Response;
}

describe('api error handling', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('resolves with the parsed JSON body on success', async () => {
    vi.stubGlobal('fetch', mockFetch(200, [{ id: 1 }]));
    await expect(listTargets()).resolves.toEqual([{ id: 1 }]);
  });

  it('throws an ApiError with the code/message from the error envelope', async () => {
    vi.stubGlobal(
      'fetch',
      mockFetch(404, { error: { code: 'not_found', message: 'target missing' } }),
    );
    await expect(listTargets()).rejects.toMatchObject({
      name: 'ApiError',
      status: 404,
      code: 'not_found',
      message: 'target missing',
    });
  });

  it('falls back to a generic code/message when the body has no envelope', async () => {
    vi.stubGlobal('fetch', mockFetch(500, undefined));
    const err = await listTargets().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).code).toBe('unknown');
    expect((err as ApiError).message).toBe('error');
  });

  it('resolves with undefined on 204 No Content', async () => {
    vi.stubGlobal('fetch', mockFetch(204, undefined));
    await expect(deleteTarget(1)).resolves.toBeUndefined();
  });
});

describe('schedules client', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('unwraps the schedules envelope', async () => {
    const fetchMock = vi.fn(async (..._args: Parameters<typeof fetch>) => jsonResponse({ schedules: [{ id: 1, name: 's' }] }));
    vi.stubGlobal('fetch', fetchMock);
    await expect(api.listSchedules()).resolves.toEqual([{ id: 1, name: 's' }]);
    expect(String(fetchMock.mock.calls[0][0])).toBe('/api/v1/schedules');
  });

  it('returns next fire times from the validate endpoint', async () => {
    const fetchMock = vi.fn(async (..._args: Parameters<typeof fetch>) => jsonResponse({ ok: true, next: ['2026-09-13T03:00:00Z'] }));
    vi.stubGlobal('fetch', fetchMock);
    await expect(api.validateCron('0 3 * * *', 'UTC')).resolves.toEqual(['2026-09-13T03:00:00Z']);
    const [url, init] = fetchMock.mock.calls[0];
    expect(String(url)).toBe('/api/v1/schedules/validate');
    expect(JSON.parse(String((init as RequestInit).body))).toEqual({ cron: '0 3 * * *', timezone: 'UTC' });
  });

  it('sends the schedule_id filter when listing runs', async () => {
    const fetchMock = vi.fn(async (..._args: Parameters<typeof fetch>) => jsonResponse({ runs: [], next_cursor: '' }));
    vi.stubGlobal('fetch', fetchMock);
    await api.listRuns(7);
    expect(String(fetchMock.mock.calls[0][0])).toBe('/api/v1/runs?schedule_id=7&limit=20');
  });

  it('turns a 404 from targetLatest into null', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse({ error: { code: 'not_found', message: 'no' } }, 404)));
    await expect(api.targetLatest(3)).resolves.toBeNull();
  });
});

describe('settings client', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('fetches settings from the settings endpoint', async () => {
    const fetchMock = vi.fn(async (..._args: Parameters<typeof fetch>) => jsonResponse({ general: {}, engines: {}, integrations: {} }));
    vi.stubGlobal('fetch', fetchMock);
    await api.getSettings();
    expect(String(fetchMock.mock.calls[0][0])).toBe('/api/v1/settings');
  });

  it('sends only the provided sections on update', async () => {
    const fetchMock = vi.fn(async (..._args: Parameters<typeof fetch>) => jsonResponse({ general: {}, engines: {}, integrations: {} }));
    vi.stubGlobal('fetch', fetchMock);
    await api.updateSettings({ integrations: { vm_enabled: true, vm_url: 'http://vm:8428' } });
    const [url, init] = fetchMock.mock.calls[0];
    expect(String(url)).toBe('/api/v1/settings');
    expect((init as RequestInit).method).toBe('PUT');
    expect(JSON.parse(String((init as RequestInit).body))).toEqual({
      integrations: { vm_enabled: true, vm_url: 'http://vm:8428' },
    });
  });

  it('posts a connection test to the target endpoint', async () => {
    const fetchMock = vi.fn(async (..._args: Parameters<typeof fetch>) => jsonResponse({ ok: true, status: 200, latency_ms: 3 }));
    vi.stubGlobal('fetch', fetchMock);
    const res = await api.testIntegration('vl', { url: 'http://vl:9428' });
    const [url, init] = fetchMock.mock.calls[0];
    expect(String(url)).toBe('/api/v1/settings/test/vl');
    expect((init as RequestInit).method).toBe('POST');
    expect(JSON.parse(String((init as RequestInit).body))).toEqual({ url: 'http://vl:9428' });
    expect(res.ok).toBe(true);
  });
});
