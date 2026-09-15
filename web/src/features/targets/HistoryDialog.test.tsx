import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import {
  afterEach, beforeEach, describe, expect, it, vi,
} from 'vitest';
import type { Target, TargetRevision } from '../../lib/api';
import { HistoryDialog } from './HistoryDialog';

function wrap(node: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{node}</QueryClientProvider>);
}

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body),
  } as Response;
}

const target: Target = {
  id: 1, name: 'home', engine: 'ookla', enabled: true, queue_id: 1, queue_name: 'wan',
  options: {}, thresholds: {}, created_at: '', updated_at: '',
};

const revisions: TargetRevision[] = [
  {
    version: 2, action: 'update', created_at: '2026-01-02T00:00:00Z',
    snapshot: target, changed: ['name'],
  },
  {
    version: 1, action: 'create', created_at: '2026-01-01T00:00:00Z',
    snapshot: target, changed: [],
  },
];

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => {
  vi.unstubAllGlobals();
});

describe('HistoryDialog', () => {
  it('lists revisions with action, changed fields, and a Current badge on the newest', async () => {
    fetchMock.mockImplementation(async (input: unknown) => {
      const url = String(input);
      if (url.endsWith('/targets/1/revisions')) return jsonResponse(revisions);
      throw new Error(`unexpected fetch: ${url}`);
    });
    const user = userEvent.setup();
    wrap(<HistoryDialog target={target} />);

    await user.click(screen.getByRole('button', { name: 'History' }));

    expect(await screen.findByText('v2')).toBeInTheDocument();
    expect(screen.getByText('v1')).toBeInTheDocument();
    expect(screen.getByText('Updated')).toBeInTheDocument();
    expect(screen.getByText('Created')).toBeInTheDocument();
    expect(screen.getByText('Current')).toBeInTheDocument();
    expect(screen.getByText(/changed: name/)).toBeInTheDocument();
    // The current (newest) revision has no Revert button; the older one does.
    expect(screen.getAllByRole('button', { name: 'Revert' })).toHaveLength(1);
  });

  it('does not fetch revisions until the dialog is opened', () => {
    fetchMock.mockImplementation(async () => jsonResponse(revisions));
    wrap(<HistoryDialog target={target} />);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('reverts only after the confirm step, then clears the confirm state', async () => {
    fetchMock.mockImplementation(async (input: unknown, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/targets/1/revisions') && !init?.method) return jsonResponse(revisions);
      if (url.endsWith('/targets/1/revisions/1/revert') && init?.method === 'POST') {
        return jsonResponse({ ...target, name: 'home' });
      }
      throw new Error(`unexpected fetch: ${url} ${init?.method}`);
    });
    const user = userEvent.setup();
    wrap(<HistoryDialog target={target} />);

    await user.click(screen.getByRole('button', { name: 'History' }));
    await screen.findByText('v1');

    await user.click(screen.getByRole('button', { name: 'Revert' }));
    expect(screen.queryByRole('button', { name: 'Revert' })).not.toBeInTheDocument();
    const confirmBtn = await screen.findByRole('button', { name: 'Confirm revert' });

    await user.click(confirmBtn);

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringContaining('/targets/1/revisions/1/revert'),
        expect.objectContaining({ method: 'POST' }),
      );
    });
  });
});
