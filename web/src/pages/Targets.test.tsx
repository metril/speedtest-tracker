import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  fireEvent, render, screen, waitFor, within,
} from '@testing-library/react';
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
    id: 1, name: 'home', engine: 'ookla', enabled: true, queue_id: 1, queue_name: 'wan',
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
  it('does not delete when the confirmation dialog is canceled', async () => {
    const targets = [target()];
    fetchMock.mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith('/targets')) return jsonResponse(targets);
      if (url.endsWith('/targets/deleted')) return jsonResponse([]);
      if (url.endsWith('/schedules')) return jsonResponse({ schedules: [] });
      throw new Error(`unexpected fetch: ${url}`);
    });

    wrap(<Targets />);
    await screen.findByText('home');

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    // Only the GETs (targets list + recently-deleted list) happened — no
    // DELETE was issued.
    expect(fetchMock).not.toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({ method: 'DELETE' }),
    );
  });

  it('deletes when the confirmation dialog is accepted', async () => {
    const targets = [target()];
    fetchMock.mockImplementation(async (input, init) => {
      const url = String(input);
      if (url.endsWith('/targets') && (!init || init.method === undefined)) return jsonResponse(targets);
      if (url.endsWith('/targets/1') && init?.method === 'DELETE') return { ok: true, status: 204, statusText: 'no content', text: async () => '' } as Response;
      if (url.endsWith('/targets/deleted')) return jsonResponse([]);
      if (url.endsWith('/schedules')) return jsonResponse({ schedules: [] });
      return jsonResponse(targets);
    });

    wrap(<Targets />);
    await screen.findByText('home');

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringContaining('/targets/1'),
        expect.objectContaining({ method: 'DELETE' }),
      );
    });
  });

  it('shows the deleted target in Recently deleted right after deleting it', async () => {
    const targets = [target()];
    let isDeleted = false;
    fetchMock.mockImplementation(async (input, init) => {
      const url = String(input);
      if (url.endsWith('/targets') && (!init || init.method === undefined)) {
        return jsonResponse(isDeleted ? [] : targets);
      }
      if (url.endsWith('/targets/1') && init?.method === 'DELETE') {
        isDeleted = true;
        return { ok: true, status: 204, statusText: 'no content', text: async () => '' } as Response;
      }
      if (url.endsWith('/targets/deleted')) {
        return jsonResponse(isDeleted
          ? [{
            id: 1, name: 'home', engine: 'ookla', queue_id: 1, queue_name: 'wan',
            deleted_at: '2026-01-01T00:00:00Z', version: 2,
          }]
          : []);
      }
      if (url.endsWith('/schedules')) return jsonResponse({ schedules: [] });
      throw new Error(`unexpected fetch: ${url}`);
    });

    wrap(<Targets />);
    await screen.findByText('home');

    // Collapsed with no expand affordance while nothing is deleted yet.
    expect(screen.getByRole('button', { name: /Recently deleted/ })).toBeDisabled();

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));

    const toggle = await screen.findByRole('button', { name: /Recently deleted \(1\)/ });
    expect(toggle).not.toBeDisabled();

    fireEvent.click(toggle);
    // The row now appears in the Recently deleted table (and no longer in
    // the live targets table, which is now empty).
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Restore' })).toBeInTheDocument();
    });
    expect(screen.getByText('home')).toBeInTheDocument();
  });
});

describe('Targets page recently deleted section', () => {
  it('shows the restore hint and only disables the row being restored', async () => {
    let isDeleted1 = false;
    let isDeleted2 = false;
    let resolveRestore: (() => void) | undefined;
    const targets = [target({ id: 1, name: 'home' }), target({ id: 2, name: 'office' })];
    const deletedRow = (id: number, name: string) => ({
      id, name, engine: 'ookla', queue_id: 1, queue_name: 'wan', deleted_at: '2026-01-01T00:00:00Z', version: 1,
    });

    fetchMock.mockImplementation(async (input, init) => {
      const url = String(input);
      if (url.endsWith('/targets') && !init?.method) {
        return jsonResponse(targets.filter((t) => (t.id === 1 ? !isDeleted1 : !isDeleted2)));
      }
      if (url.endsWith('/targets/deleted')) {
        const rows = [];
        if (isDeleted1) rows.push(deletedRow(1, 'home'));
        if (isDeleted2) rows.push(deletedRow(2, 'office'));
        return jsonResponse(rows);
      }
      if (url.endsWith('/targets/1') && init?.method === 'DELETE') {
        isDeleted1 = true;
        return { ok: true, status: 204, statusText: 'no content', text: async () => '' } as Response;
      }
      if (url.endsWith('/targets/2') && init?.method === 'DELETE') {
        isDeleted2 = true;
        return { ok: true, status: 204, statusText: 'no content', text: async () => '' } as Response;
      }
      if (url.endsWith('/targets/deleted/1/restore') && init?.method === 'POST') {
        await new Promise<void>((resolve) => { resolveRestore = resolve; });
        return jsonResponse(deletedRow(1, 'home'));
      }
      if (url.endsWith('/schedules')) return jsonResponse({ schedules: [] });
      throw new Error(`unexpected fetch: ${url}`);
    });

    wrap(<Targets />);
    await screen.findByText('home');
    await screen.findByText('office');

    // Delete both targets so Recently deleted has two rows.
    const deleteButtons = screen.getAllByRole('button', { name: 'Delete' });
    fireEvent.click(deleteButtons[0]);
    let dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));
    await screen.findByRole('button', { name: /Recently deleted \(1\)/ });

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));
    const toggle = await screen.findByRole('button', { name: /Recently deleted \(2\)/ });

    fireEvent.click(toggle);
    expect(screen.getByText(
      'Restored targets keep their history but must be re-added to any schedules.',
    )).toBeInTheDocument();

    const restoreButtons = await screen.findAllByRole('button', { name: 'Restore' });
    expect(restoreButtons).toHaveLength(2);
    fireEvent.click(restoreButtons[0]);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Restoring…' })).toBeInTheDocument();
    });
    // The other row's restore button must still read "Restore" and not
    // be disabled.
    const remaining = screen.getAllByRole('button', { name: 'Restore' });
    expect(remaining).toHaveLength(1);
    expect(remaining[0]).not.toBeDisabled();

    resolveRestore?.();
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Restoring…' })).not.toBeInTheDocument();
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
      if (url.endsWith('/schedules')) return jsonResponse({ schedules: [] });
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
    expect(screen.queryByText(/Queued run/)).not.toBeInTheDocument();
  });
});

describe('Targets page schedules column', () => {
  it('lists the schedules that include each target, and a dash for targets in none', async () => {
    const targets = [target({ id: 1, name: 'home' }), target({ id: 2, name: 'office' })];
    fetchMock.mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith('/targets')) return jsonResponse(targets);
      if (url.endsWith('/targets/deleted')) return jsonResponse([]);
      if (url.endsWith('/schedules')) {
        return jsonResponse({
          schedules: [
            {
              id: 1, name: 'nightly', cron: '0 3 * * *', enabled: true, timezone: 'UTC',
              target_ids: [1], next_run: '', last_run: null, created_at: '', updated_at: '',
            },
          ],
        });
      }
      throw new Error(`unexpected fetch: ${url}`);
    });

    wrap(<Targets />);
    await screen.findByText('home');

    expect(screen.getByText('nightly')).toBeInTheDocument();
    const rows = screen.getAllByRole('row');
    const officeRow = rows.find((r) => r.textContent?.includes('office'));
    expect(officeRow?.textContent).toContain('—');
  });
});

describe('Targets page hosts column', () => {
  it('shows a dash for a target with zero or one configured host', async () => {
    const targets = [
      target({ id: 1, name: 'home', engine: 'ookla', options: {} }),
      target({ id: 2, name: 'single', engine: 'ookla', options: { server_id: 111 } }),
    ];
    fetchMock.mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith('/targets')) return jsonResponse(targets);
      if (url.endsWith('/targets/deleted')) return jsonResponse([]);
      if (url.endsWith('/schedules')) return jsonResponse({ schedules: [] });
      throw new Error(`unexpected fetch: ${url}`);
    });

    wrap(<Targets />);
    await screen.findByText('home');

    const rows = screen.getAllByRole('row');
    for (const name of ['home', 'single']) {
      const row = rows.find((r) => r.textContent?.includes(name));
      expect(row?.textContent).toContain('—');
    }
  });

  it('shows the list size and the entry rotation_index points to next', async () => {
    const targets = [
      target({
        id: 1, name: 'rot-ookla', engine: 'ookla', rotation_index: 1,
        options: { server_ids: [111, 222, 333] },
      }),
      target({
        id: 2, name: 'rot-iperf3', engine: 'iperf3', rotation_index: 2,
        options: { hosts: ['a.lan', 'b.lan'] },
      }),
    ];
    fetchMock.mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith('/targets')) return jsonResponse(targets);
      if (url.endsWith('/targets/deleted')) return jsonResponse([]);
      if (url.endsWith('/schedules')) return jsonResponse({ schedules: [] });
      throw new Error(`unexpected fetch: ${url}`);
    });

    wrap(<Targets />);
    await screen.findByText('rot-ookla');

    expect(screen.getByText('3 hosts, next: 222')).toBeInTheDocument();
    // rotation_index 2 % 2 hosts = 0 -> "a.lan"
    expect(screen.getByText('2 hosts, next: a.lan')).toBeInTheDocument();
  });
});
