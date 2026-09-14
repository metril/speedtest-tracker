import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import {
  afterEach, beforeEach, describe, expect, it, vi,
} from 'vitest';
import type { Target } from '../lib/api';
import { Targets } from './Targets';

function wrap(node: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{node}</QueryClientProvider>);
}

function target(overrides: Partial<Target> = {}): Target {
  return {
    id: 1, name: 'home', engine: 'ookla', enabled: true, lane: 'wan',
    options: {}, thresholds: {}, created_at: '', updated_at: '',
    ...overrides,
  };
}

/** jsonResponse builds a fetch Response-shaped stub the same way the rest
 * of this codebase's tests do (see TargetForm.test.tsx); cast to Response
 * since only the fields api.ts's request() reads are provided. */
function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body),
  } as Response;
}

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => {
  vi.unstubAllGlobals();
});

describe('Targets page delete confirmation', () => {
  it('does not delete when the confirmation is declined', async () => {
    const targets = [target()];
    fetchMock.mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith('/targets') ) return jsonResponse(targets);
      throw new Error(`unexpected fetch: ${url}`);
    });
    vi.spyOn(window, 'confirm').mockReturnValue(false);

    wrap(<Targets />);
    await screen.findByText('home');

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    expect(window.confirm).toHaveBeenCalled();
    // Only the initial GET /targets happened — no DELETE was issued.
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('deletes when the confirmation is accepted', async () => {
    const targets = [target()];
    fetchMock.mockImplementation(async (input, init) => {
      const url = String(input);
      if (url.endsWith('/targets') && (!init || init.method === undefined)) return jsonResponse(targets);
      if (url.endsWith('/targets/1') && init?.method === 'DELETE') return { ok: true, status: 204, statusText: 'no content', text: async () => '' } as Response;
      return jsonResponse(targets);
    });
    vi.spyOn(window, 'confirm').mockReturnValue(true);

    wrap(<Targets />);
    await screen.findByText('home');

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringContaining('/targets/1'),
        expect.objectContaining({ method: 'DELETE' }),
      );
    });
  });
});

describe('Targets page per-row run pending state', () => {
  it('shows pending state only for the row being run', async () => {
    const targets = [target({ id: 1, name: 'home' }), target({ id: 2, name: 'office' })];
    let resolveRun: (() => void) | undefined;
    fetchMock.mockImplementation(async (input, init) => {
      const url = String(input);
      if (url.endsWith('/targets') && !init?.method) return jsonResponse(targets);
      if (url.endsWith('/targets/1/run') && init?.method === 'POST') {
        await new Promise<void>((resolve) => { resolveRun = resolve; });
        return jsonResponse({ run_id: 9 });
      }
      return jsonResponse(targets);
    });

    wrap(<Targets />);
    await screen.findByText('home');
    await screen.findByText('office');

    const runButtons = screen.getAllByRole('button', { name: /Run now/ });
    fireEvent.click(runButtons[0]);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Running…' })).toBeInTheDocument();
    });
    // The other row's button must still read "Run now", not be disabled.
    const remaining = screen.getAllByRole('button', { name: 'Run now' });
    expect(remaining).toHaveLength(1);
    expect(remaining[0]).not.toBeDisabled();

    resolveRun?.();
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Running…' })).not.toBeInTheDocument();
    });
  });
});
