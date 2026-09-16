import { render, screen } from '@testing-library/react';
import { expect, it } from 'vitest';
import type { GeneralSettings, SummaryStats } from '../../lib/api';
import { SummaryTiles } from './SummaryTiles';

const general: GeneralSettings = {
  base_url: '', timezone: 'UTC', units: 'Mbps', log_level: 'info',
  retention_days_results: 90, retention_days_runs: 30, retention_prune_interval_minutes: 60,
  sla_download_mbps: 1000, sla_upload_mbps: 50,
};

const stats: SummaryStats = {
  from: '2026-09-12T00:00:00.000Z', to: '2026-09-13T00:00:00.000Z',
  targets: [{
    target_id: 1, target_name: 'home', engine: 'fake', latest: null,
    count: 10, fail_count: 1, success_rate: 0.9,
    avg_download_bps: 100e6, min_download_bps: 50e6, max_download_bps: 150e6,
    avg_upload_bps: 20e6, avg_ping_ms: 12, max_ping_ms: 30, sla_compliance: null,
  }],
  total_results: 10, total_failures: 1, success_rate: 0.9, sla_compliance: null,
};

it('shows success rate, average download, average upload and average ping', () => {
  render(<SummaryTiles stats={stats} />);
  expect(screen.getByText('90%')).toBeInTheDocument();
  expect(screen.getByText('100.0 Mbps')).toBeInTheDocument();
  expect(screen.getByText('20.0 Mbps')).toBeInTheDocument();
  expect(screen.getByText('12.0 ms')).toBeInTheDocument();
});

it('shows the total test count as subtext on the success rate tile', () => {
  render(<SummaryTiles stats={stats} />);
  expect(screen.getByText('10 tests')).toBeInTheDocument();
});

it('shows a dash for average ping when no target has an ok reading in range', () => {
  const noOkReadings: SummaryStats = {
    ...stats,
    targets: [{ ...stats.targets[0], avg_ping_ms: 0 }],
  };
  render(<SummaryTiles stats={noOkReadings} />);
  expect(screen.getByText('—')).toBeInTheDocument();
});

it('excludes targets with no ok ping reading from the average ping instead of dragging it to 0', () => {
  const mixed: SummaryStats = {
    ...stats,
    targets: [
      stats.targets[0], // avg_ping_ms: 12, count: 10
      { ...stats.targets[0], target_id: 2, target_name: 'office', avg_ping_ms: 0, count: 5 },
    ],
    total_results: 15,
  };
  render(<SummaryTiles stats={mixed} />);
  // Only the first target's ping (12ms) counts; the second (no ok
  // reading) must be excluded from both numerator and denominator rather
  // than pulling the average toward 0.
  expect(screen.getByText('12.0 ms')).toBeInTheDocument();
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

it('shows a "Meets plan" tile when sla_compliance is non-null', () => {
  render(<SummaryTiles stats={{ ...stats, sla_compliance: 0.974 }} />);
  expect(screen.getByText('Meets plan')).toBeInTheDocument();
  expect(screen.getByText('97.4%')).toBeInTheDocument();
});

it('hides the "Meets plan" tile when sla_compliance is null', () => {
  render(<SummaryTiles stats={stats} />);
  expect(screen.queryByText('Meets plan')).not.toBeInTheDocument();
});

it('shows the plan speeds as subtext on the "Meets plan" tile', () => {
  render(<SummaryTiles stats={{ ...stats, sla_compliance: 0.9 }} general={general} />);
  expect(screen.getByText('1000/50 Mbps plan')).toBeInTheDocument();
});

it('shows the tolerance-adjusted subtext when a tolerance is set', () => {
  render(
    <SummaryTiles
      stats={{ ...stats, sla_compliance: 0.9 }}
      general={{ ...general, sla_download_mbps: 100, sla_upload_mbps: 20, sla_tolerance_pct: 10 }}
    />,
  );
  expect(screen.getByText('≥ 90/18 Mbps (100/20 plan, 10% tolerance)')).toBeInTheDocument();
});

it('adds a tooltip title to the "Meets plan" tile', () => {
  render(<SummaryTiles stats={{ ...stats, sla_compliance: 0.9 }} general={general} />);
  expect(screen.getByText('Meets plan').closest('[title]')).toHaveAttribute(
    'title',
    'Share of tests in this period, including failed ones, whose download and upload both reached the plan speed minus tolerance.',
  );
});

it('shows a favorable delta on "Meets plan" vs. the previous period', () => {
  const previous: SummaryStats = { ...stats, sla_compliance: 0.8 };
  render(<SummaryTiles stats={{ ...stats, sla_compliance: 0.9 }} previousStats={previous} />);
  expect(screen.getByLabelText(/up 12% from previous period/i)).toHaveClass('text-ok');
});
