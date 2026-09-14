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

it('shows tests, success rate, average download and worst ping', () => {
  render(<SummaryTiles stats={stats} />);
  expect(screen.getByText('10')).toBeInTheDocument();
  expect(screen.getByText('90%')).toBeInTheDocument();
  expect(screen.getByText('100.0 Mbps')).toBeInTheDocument();
  expect(screen.getByText('30 ms')).toBeInTheDocument();
});

it('renders an empty state when nothing ran in the window', () => {
  render(<SummaryTiles stats={{ ...stats, targets: [], total_results: 0, total_failures: 0, success_rate: 0 }} />);
  expect(screen.getByText(/no tests in this range/i)).toBeInTheDocument();
});
