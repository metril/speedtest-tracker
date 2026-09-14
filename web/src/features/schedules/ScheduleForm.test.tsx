import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Target } from '../../lib/api';
import { ScheduleForm } from './ScheduleForm';

function jsonResponse(body: unknown, status = 200): Response {
  return { ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body) } as Response;
}

function target(id: number, name: string): Target {
  return { id, name, engine: 'fake', enabled: true, lane: 'wan', options: {}, thresholds: {}, created_at: '', updated_at: '' };
}

function wrap(node: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{node}</QueryClientProvider>);
}

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn(async () => jsonResponse({ ok: true, next: ['2026-09-13T03:00:00Z', '2026-09-14T03:00:00Z'] })));
});
afterEach(() => vi.unstubAllGlobals());

const targets = [target(1, 'home'), target(2, 'nas')];

describe('ScheduleForm', () => {
  it('applies a preset to the cron field and previews the next runs', async () => {
    wrap(<ScheduleForm targets={targets} onSubmit={vi.fn()} onCancel={vi.fn()} submitting={false} />);
    await userEvent.click(screen.getByRole('button', { name: 'Daily 03:00' }));
    expect(screen.getByLabelText('Cron expression')).toHaveValue('0 3 * * *');
    await waitFor(() => expect(screen.getByTestId('cron-preview').textContent).toMatch(/2026/));
  });

  it('submits name, cron, timezone and the ordered target ids', async () => {
    const onSubmit = vi.fn();
    wrap(<ScheduleForm targets={targets} onSubmit={onSubmit} onCancel={vi.fn()} submitting={false} />);
    await userEvent.type(screen.getByLabelText('Name'), 'nightly');
    await userEvent.click(screen.getByRole('button', { name: 'Daily 03:00' }));
    await userEvent.click(screen.getByRole('checkbox', { name: /home/ }));
    await userEvent.click(screen.getByRole('checkbox', { name: /nas/ }));
    await userEvent.click(screen.getByRole('button', { name: 'Save schedule' }));
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({
      name: 'nightly', cron: '0 3 * * *', enabled: true, target_ids: [1, 2],
    }));
  });

  it('reorders selected targets', async () => {
    const onSubmit = vi.fn();
    wrap(<ScheduleForm targets={targets} onSubmit={onSubmit} onCancel={vi.fn()} submitting={false} />);
    await userEvent.type(screen.getByLabelText('Name'), 's');
    await userEvent.click(screen.getByRole('checkbox', { name: /home/ }));
    await userEvent.click(screen.getByRole('checkbox', { name: /nas/ }));
    await userEvent.click(screen.getByRole('button', { name: 'Move nas up' }));
    await userEvent.click(screen.getByRole('button', { name: 'Save schedule' }));
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ target_ids: [2, 1] }));
  });

  it('refuses to submit without a target', async () => {
    const onSubmit = vi.fn();
    wrap(<ScheduleForm targets={targets} onSubmit={onSubmit} onCancel={vi.fn()} submitting={false} />);
    await userEvent.type(screen.getByLabelText('Name'), 's');
    await userEvent.click(screen.getByRole('button', { name: 'Save schedule' }));
    expect(onSubmit).not.toHaveBeenCalled();
    expect(screen.getByText(/at least one target/i)).toBeInTheDocument();
  });

  it('debounces cron and timezone edits before querying the preview', async () => {
    vi.useFakeTimers();
    const fetchMock = vi.fn(async () => jsonResponse({ ok: true, next: ['2026-09-13T03:00:00Z'] }));
    vi.stubGlobal('fetch', fetchMock);
    try {
      wrap(<ScheduleForm targets={targets} onSubmit={vi.fn()} onCancel={vi.fn()} submitting={false} />);
      const initialCalls = fetchMock.mock.calls.length;

      fireEvent.change(screen.getByLabelText('Cron expression'), { target: { value: '0 5 * * *' } });
      fireEvent.change(screen.getByLabelText('Custom timezone'), { target: { value: 'Pacific/Fiji' } });
      // Not yet debounced: no new preview request.
      expect(fetchMock.mock.calls.length).toBe(initialCalls);

      await act(async () => { await vi.advanceTimersByTimeAsync(300); });
      expect(fetchMock.mock.calls.length).toBeGreaterThan(initialCalls);
    } finally {
      vi.useRealTimers();
    }
  });

  it('shows a Custom… option instead of injecting the free-text zone into the select', async () => {
    wrap(<ScheduleForm targets={targets} onSubmit={vi.fn()} onCancel={vi.fn()} submitting={false} />);
    await userEvent.type(screen.getByLabelText('Custom timezone'), 'Pacific/Fiji');
    const select = screen.getByLabelText('Timezone') as HTMLSelectElement;
    expect(select.value).toBe('__custom__');
    expect(screen.getByRole('option', { name: 'Custom…' })).toBeInTheDocument();
    expect(screen.queryByRole('option', { name: 'Pacific/Fiji' })).not.toBeInTheDocument();
  });
});
