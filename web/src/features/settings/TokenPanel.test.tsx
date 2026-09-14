import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from '../../lib/api';
import { ApiError } from '../../lib/api';
import type { ApiTokenInfo } from '../../lib/api';
import { TokenPanel } from './TokenPanel';

function jsonResponse(body: unknown, status = 200): Response {
  return { ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body) } as Response;
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

function renderPanel(opts: {
  create?: ReturnType<typeof vi.fn>;
  del?: ReturnType<typeof vi.fn>;
  tokens?: ApiTokenInfo[];
} = {}) {
  const tokens = opts.tokens ?? [];
  fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.startsWith('/api/v1/tokens')) return jsonResponse({ tokens });
    throw new Error(`unexpected fetch: ${url}`);
  });
  if (opts.create) vi.spyOn(api, 'createToken').mockImplementation(opts.create);
  if (opts.del) vi.spyOn(api, 'deleteToken').mockImplementation(opts.del);

  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrap = (node: ReactNode) => <QueryClientProvider client={qc}>{node}</QueryClientProvider>;
  return render(wrap(<TokenPanel />));
}

describe('TokenPanel', () => {
  it('creates a token and shows the plaintext exactly once', async () => {
    const create = vi.fn().mockResolvedValue({
      id: 1, name: 'ha', prefix: 'stt_abc', created_at: 'x', token: 'stt_abcdef123',
    });
    renderPanel({ create, tokens: [] });
    await userEvent.type(screen.getByLabelText('Token name'), 'ha');
    await userEvent.click(screen.getByRole('button', { name: 'Create token' }));
    expect(create).toHaveBeenCalledWith('ha');
    expect(await screen.findByText('stt_abcdef123')).toBeInTheDocument();
    expect(screen.getByText(/shown only once/i)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Done' }));
    expect(screen.queryByText('stt_abcdef123')).toBeNull();
  });

  it('copies the new token to the clipboard', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    renderPanel({
      create: vi.fn().mockResolvedValue({
        id: 1, name: 'ha', prefix: 'stt_abc', created_at: 'x', token: 'stt_xyz',
      }),
      tokens: [],
    });
    await userEvent.type(screen.getByLabelText('Token name'), 'ha');
    await userEvent.click(screen.getByRole('button', { name: 'Create token' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Copy' }));
    expect(writeText).toHaveBeenCalledWith('stt_xyz');
    expect(await screen.findByText('Copied')).toBeInTheDocument();
  });

  it('lists tokens with their prefix and last use, never a full token', async () => {
    renderPanel({
      tokens: [{
        id: 1, name: 'ha', prefix: 'stt_abc', created_at: '2026-09-14T10:00:00.000Z', last_used_at: '2026-09-14T11:00:00.000Z',
      }],
    });
    expect(await screen.findByText('ha')).toBeInTheDocument();
    expect(screen.getByText(/stt_abc…/)).toBeInTheDocument();
  });

  it('confirms before revoking', async () => {
    const del = vi.fn().mockResolvedValue(undefined);
    renderPanel({ del, tokens: [{ id: 1, name: 'ha', prefix: 'stt_abc', created_at: 'x' }] });
    await userEvent.click(await screen.findByRole('button', { name: 'Revoke ha' }));
    expect(del).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', { name: 'Confirm revoke' }));
    expect(del).toHaveBeenCalledWith(1);
  });

  it('shows the server error when revoking the last token is refused', async () => {
    const del = vi.fn().mockRejectedValue(
      new ApiError(400, 'invalid_request', 'cannot revoke the last API token while auth mode is token'),
    );
    renderPanel({ del, tokens: [{ id: 1, name: 'ha', prefix: 'stt_abc', created_at: 'x' }] });
    await userEvent.click(await screen.findByRole('button', { name: 'Revoke ha' }));
    await userEvent.click(screen.getByRole('button', { name: 'Confirm revoke' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(/last API token/);
  });

  it('requires a name', async () => {
    const create = vi.fn();
    renderPanel({ create, tokens: [] });
    await userEvent.click(screen.getByRole('button', { name: 'Create token' }));
    expect(create).not.toHaveBeenCalled();
  });
});
