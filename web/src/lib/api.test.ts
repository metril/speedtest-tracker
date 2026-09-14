import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, deleteTarget, listTargets } from './api';

function mockFetch(status: number, body: unknown) {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    statusText: 'error',
    text: async () => (body === undefined ? '' : JSON.stringify(body)),
  });
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
