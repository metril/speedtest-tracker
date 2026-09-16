import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Me } from '../lib/api';
import { OpenModeBanner } from './OpenModeBanner';

function jsonResponse(body: unknown): Response {
  return { ok: true, status: 200, statusText: 'ok', text: async () => JSON.stringify(body) } as Response;
}

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function renderWithMe(me: Me) {
  fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.startsWith('/api/v1/me')) return jsonResponse(me);
    throw new Error(`unexpected fetch: ${url}`);
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <OpenModeBanner />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('OpenModeBanner', () => {
  it('warns while the instance is open and stays quiet otherwise', async () => {
    renderWithMe({
      mode: 'open', user: '', display_name: '', groups: [], is_admin: true,
    });
    expect(await screen.findByRole('status')).toHaveTextContent(/no authentication/i);
    cleanup();

    renderWithMe({
      mode: 'token', user: 'token', display_name: 'token', groups: [], is_admin: true,
    });
    expect(screen.queryByRole('status')).toBeNull();
  });
});
