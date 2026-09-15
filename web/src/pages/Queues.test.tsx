import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  fireEvent, render, screen, waitFor,
} from '@testing-library/react';
import type { ReactNode } from 'react';
import {
  afterEach, beforeEach, describe, expect, it, vi,
} from 'vitest';
import type { Queue } from '../lib/api';
import { Queues } from './Queues';

function wrap(node: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{node}</QueryClientProvider>);
}

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body),
  } as Response;
}

const wan: Queue = { id: 1, name: 'wan', created_at: '' };
const lan: Queue = { id: 2, name: 'lan', created_at: '' };

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => vi.unstubAllGlobals());

describe('Queues page', () => {
  it('lists queues and explains the run-in-parallel semantics', async () => {
    fetchMock.mockImplementation(async () => jsonResponse([wan, lan]));
    wrap(<Queues />);
    expect(screen.getByText(/run one at a time; different queues run in parallel/)).toBeInTheDocument();
    expect(await screen.findByText('wan')).toBeInTheDocument();
    expect(screen.getByText('lan')).toBeInTheDocument();
  });

  it('creates a queue from the inline form', async () => {
    fetchMock.mockImplementation(async (input, init) => {
      const url = String(input);
      if (init?.method === 'POST' && url.endsWith('/queues')) {
        return jsonResponse({ id: 3, name: 'office', created_at: '' }, 201);
      }
      return jsonResponse([wan, lan]);
    });
    wrap(<Queues />);
    await screen.findByText('wan');

    fireEvent.change(screen.getByLabelText('New queue name'), { target: { value: 'office' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add queue' }));

    await waitFor(() => {
      const call = fetchMock.mock.calls.find(([, init]) => init?.method === 'POST');
      expect(call).toBeDefined();
      expect(JSON.parse(call![1].body as string)).toEqual({ name: 'office' });
    });
  });

  it('renames a queue inline', async () => {
    fetchMock.mockImplementation(async (input, init) => {
      const url = String(input);
      if (init?.method === 'PUT' && url.endsWith('/queues/1')) {
        return jsonResponse({ id: 1, name: 'branch', created_at: '' });
      }
      return jsonResponse([wan, lan]);
    });
    wrap(<Queues />);
    await screen.findByText('wan');

    fireEvent.click(screen.getAllByRole('button', { name: 'Rename' })[0]);
    const input = screen.getByLabelText('Rename queue wan');
    fireEvent.change(input, { target: { value: 'branch' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      const call = fetchMock.mock.calls.find(([, init]) => init?.method === 'PUT');
      expect(call).toBeDefined();
      expect(JSON.parse(call![1].body as string)).toEqual({ name: 'branch' });
    });
  });

  it('shows the in-use error and reopens the confirm dialog on a failed delete', async () => {
    fetchMock.mockImplementation(async (input, init) => {
      const url = String(input);
      if (init?.method === 'DELETE' && url.endsWith('/queues/1')) {
        return jsonResponse({ error: { code: 'queue_in_use', message: 'targets still reference this queue' } }, 409);
      }
      return jsonResponse([wan, lan]);
    });
    wrap(<Queues />);
    await screen.findByText('wan');

    fireEvent.click(screen.getAllByRole('button', { name: 'Delete' })[0]);
    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }));

    expect(await screen.findByText('targets still reference this queue')).toBeInTheDocument();
  });
});
