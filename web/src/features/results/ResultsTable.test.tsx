import {
  fireEvent, render, screen, waitFor, within,
} from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Result } from '../../lib/api';
import { ResultsTable } from './ResultsTable';

function makeResult(overrides: Partial<Result> = {}): Result {
  return {
    id: 1,
    run_id: 1,
    target_id: 1,
    target_name: 'home-wan',
    engine: 'ookla',
    options_snapshot: {},
    status: 'ok',
    started_at: new Date().toISOString(),
    duration_ms: 1000,
    download_bps: 100_000_000,
    upload_bps: 20_000_000,
    ping_ms: 10,
    jitter_ms: 1,
    packet_loss_pct: 0,
    server_name: 'srv',
    server_host: 'host',
    isp: 'isp',
    result_url: '',
    tags: [],
    ...overrides,
  };
}

beforeEach(() => {
  // jsdom reports zero size for every element, so @tanstack/react-virtual
  // would otherwise compute an empty visible range. Give the scroll
  // container a real height so rows actually render.
  vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(600);
  vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(600);
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('ResultsTable delete confirmation', () => {
  it('does not call onDelete when the confirmation dialog is canceled', async () => {
    const onDelete = vi.fn();
    render(
      <ResultsTable rows={[makeResult()]} onDelete={onDelete} onReexecute={vi.fn()} onTag={vi.fn()} />,
    );

    fireEvent.click(screen.getByRole('button', { name: /Delete result for/ }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(onDelete).not.toHaveBeenCalled();
  });

  it('calls onDelete when the confirmation dialog is accepted', async () => {
    const onDelete = vi.fn();
    render(
      <ResultsTable rows={[makeResult({ id: 42 })]} onDelete={onDelete} onReexecute={vi.fn()} onTag={vi.fn()} />,
    );

    fireEvent.click(screen.getByRole('button', { name: /Delete result for/ }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));

    expect(onDelete).toHaveBeenCalledWith(42);
  });
});

describe('ResultsTable errors', () => {
  it('shows a View error button for a failed row and opens the error in a dialog', async () => {
    render(
      <ResultsTable
        rows={[makeResult({ id: 3, target_name: 'wan-2', status: 'failed', error: 'dial tcp: i/o timeout' })]}
        onDelete={vi.fn()}
        onReexecute={vi.fn()}
        onTag={vi.fn()}
      />,
    );

    expect(screen.queryByText('dial tcp: i/o timeout')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /view error/i }));
    expect(await screen.findByText('dial tcp: i/o timeout')).toBeInTheDocument();
  });

  it('does not show a View error button for a successful row', () => {
    render(
      <ResultsTable
        rows={[makeResult({ id: 4, target_name: 'wan-3', status: 'ok' })]}
        onDelete={vi.fn()}
        onReexecute={vi.fn()}
        onTag={vi.fn()}
      />,
    );
    expect(screen.queryByRole('button', { name: /view error/i })).not.toBeInTheDocument();
  });
});

describe('ResultsTable accessibility', () => {
  it('exposes a table/row/cell role structure and labeled action buttons', () => {
    render(
      <ResultsTable
        rows={[makeResult({ id: 7, target_name: 'wan-1' })]}
        onDelete={vi.fn()}
        onReexecute={vi.fn()}
        onTag={vi.fn()}
      />,
    );

    expect(screen.getByRole('table', { name: 'Results' })).toBeInTheDocument();
    expect(screen.getAllByRole('row').length).toBeGreaterThan(1); // header row + data row
    expect(screen.getAllByRole('cell').length).toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: 'Replay result for wan-1' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete result for wan-1' })).toBeInTheDocument();
  });
});
