import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import * as api from './api';
import { useReexecute, useRunTarget } from './queries';

function wrapper(client: QueryClient) {
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}

afterEach(() => {
  vi.restoreAllMocks();
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
