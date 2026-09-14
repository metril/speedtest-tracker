import { render, screen } from '@testing-library/react';
import { expect, it } from 'vitest';
import type { SummaryStats } from '../../lib/api';
import { SummaryTiles } from './SummaryTiles';

const stats: SummaryStats = {
  from: '2026-09-12T00:00:00.000Z', to: '2026-09-13T00:00:00.000Z',
  targets: [{
    target_id: 1, target_name: 'home', engine: 'fake', latest: null,
    count: 10, fail_count: 1, success_rate: 0.9,
    avg_download_bps: 100e6, min_download_bps: 50e6, max_download_bps: 150e6,
    avg_upload_bps: 20e6, avg_ping_ms: 12, max_ping_ms: 30,
  }],
  total_results: 10, total_failures: 1, success_rate: 0.9,
};

it('shows success rate, average download, average upload and average ping', () => {
  render(<SummaryTiles stats={stats} />);
  expect(screen.getByText('90%')).toBeInTheDocument();
  expect(screen.getByText('100.0 Mbps')).toBeInTheDocument();
  expect(screen.getByText('20.0 Mbps')).toBeInTheDocument();
  expect(screen.getByText('12.0 ms')).toBeInTheDocument();
});

it('shows a dash for average ping when no target has an ok reading in range', () => {
  const noOkReadings: SummaryStats = {
    ...stats,
    targets: [{ ...stats.targets[0], avg_ping_ms: 0 }],
  };
  render(<SummaryTiles stats={noOkReadings} />);
  expect(screen.getByText('—')).toBeInTheDocument();
});

it('renders an empty state when nothing ran in the window', () => {
  render(<SummaryTiles stats={{ ...stats, targets: [], total_results: 0, total_failures: 0, success_rate: 0 }} />);
  expect(screen.getByText(/no tests in this range/i)).toBeInTheDocument();
});

it('shows a favorable delta for a download rise vs. the previous period', () => {
  const previous: SummaryStats = {
    ...stats,
    targets: [{ ...stats.targets[0], avg_download_bps: 80e6 }],
  };
  render(<SummaryTiles stats={stats} previousStats={previous} />);
  expect(screen.getByLabelText(/up 25% from previous period/i)).toHaveClass('text-ok');
});

it('hides the delta when the previous window has no data', () => {
  const empty: SummaryStats = { ...stats, targets: [], total_results: 0, total_failures: 0, success_rate: 0 };
  render(<SummaryTiles stats={stats} previousStats={empty} />);
  expect(screen.queryByLabelText(/from previous period/i)).not.toBeInTheDocument();
});
