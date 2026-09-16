import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import * as api from './api';
import {
  queryKeys, useCronPreview, useReexecute, useRunTarget, useTargetHistory, useUpdateSettings,
} from './queries';

function wrapper(client: QueryClient) {
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}

/** jsonResponse builds a fetch Response-shaped stub the same way the rest
 * of this codebase's tests do (see Targets.test.tsx); cast to Response
 * since only the fields api.ts's request() reads are provided. */
function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body),
  } as Response;
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('run-triggering mutations invalidate the runs cache', () => {
  it('useRunTarget invalidates ["runs"] on success', async () => {
    vi.spyOn(api, 'runTarget').mockResolvedValue({ run_id: 1 });
    const client = new QueryClient();
    const spy = vi.spyOn(client, 'invalidateQueries');

    const { result } = renderHook(() => useRunTarget(), { wrapper: wrapper(client) });
    result.current.mutate(7);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(spy).toHaveBeenCalledWith({ queryKey: ['runs'] });
  });

  it('useReexecute invalidates ["runs"] on success', async () => {
    vi.spyOn(api, 'reexecuteResult').mockResolvedValue({ run_id: 2 });
    const client = new QueryClient();
    const spy = vi.spyOn(client, 'invalidateQueries');

    const { result } = renderHook(() => useReexecute(), { wrapper: wrapper(client) });
    result.current.mutate(3);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(spy).toHaveBeenCalledWith({ queryKey: ['runs'] });
  });
});

describe('useCronPreview', () => {
  it('stays idle until a cron expression is supplied', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    const { result } = renderHook(() => useCronPreview('', 'UTC'), { wrapper: wrapper(new QueryClient()) });
    expect(result.current.fetchStatus).toBe('idle');
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('fetches the preview once an expression is supplied', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse({ ok: true, next: ['2026-09-13T03:00:00Z'] })));
    const { result } = renderHook(() => useCronPreview('0 3 * * *', 'UTC'), { wrapper: wrapper(new QueryClient()) });
    await waitFor(() => expect(result.current.data).toEqual(['2026-09-13T03:00:00Z']));
  });
});

describe('useTargetHistory', () => {
  it('fetches offset 0 by default, with a query key that includes it', async () => {
    const spy = vi.spyOn(api, 'targetHistory').mockResolvedValue(
      { target_id: 1, from: '', to: '', bucket_seconds: 3600, points: [] },
    );
    const { result } = renderHook(() => useTargetHistory(1, '24h'), { wrapper: wrapper(new QueryClient()) });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(spy).toHaveBeenCalledWith(1, '24h', undefined);
  });

  it('fetches offset 1 when asked, under a distinct query key from offset 0', async () => {
    const spy = vi.spyOn(api, 'targetHistory').mockResolvedValue(
      { target_id: 1, from: '', to: '', bucket_seconds: 3600, points: [] },
    );
    const { result } = renderHook(
      () => useTargetHistory(1, '24h', { offset: 1 }), { wrapper: wrapper(new QueryClient()) },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(spy).toHaveBeenCalledWith(1, '24h', 1);
    expect(queryKeys.history(1, '24h', 1)).not.toEqual(queryKeys.history(1, '24h'));
  });

  it('stays disabled when enabled:false, regardless of offset', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    const { result } = renderHook(
      () => useTargetHistory(1, '24h', { offset: 1, enabled: false }), { wrapper: wrapper(new QueryClient()) },
    );
    expect(result.current.fetchStatus).toBe('idle');
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe('useUpdateSettings', () => {
  it('seeds the settings cache with the response and invalidates "me" after a successful save', async () => {
    const response = {
      general: {} as api.GeneralSettings,
      engines: {} as api.EngineSettings,
      integrations: {} as api.IntegrationSettings,
      notifications: {} as api.NotificationSettings,
      auth: {} as api.AuthSettings,
      locked: [],
    };
    vi.spyOn(api, 'updateSettings').mockResolvedValue(response);
    const client = new QueryClient();
    const setSpy = vi.spyOn(client, 'setQueryData');
    const invalidateSpy = vi.spyOn(client, 'invalidateQueries');

    const { result } = renderHook(() => useUpdateSettings(), { wrapper: wrapper(client) });
    result.current.mutate({ general: { units: 'metric' } });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    // setQueryData (not invalidateQueries) updates `settings` -- so
    // settings.data reflects the save immediately, with no dependency on
    // a refetch that may be slow or never resolve.
    expect(setSpy).toHaveBeenCalledWith(queryKeys.settings, response);
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: queryKeys.me });
    expect(invalidateSpy).not.toHaveBeenCalledWith({ queryKey: queryKeys.settings });
  });
});
