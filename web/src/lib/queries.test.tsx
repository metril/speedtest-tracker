import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import * as api from './api';
import { useCronPreview, useReexecute, useRunTarget } from './queries';

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
