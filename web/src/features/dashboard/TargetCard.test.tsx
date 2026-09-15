import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import type { TargetSummary } from '../../lib/api';
import { TargetCard } from './TargetCard';

const summary: TargetSummary = {
  target_id: 1, target_name: 'home', engine: 'ookla',
  latest: {
    id: 9, run_id: 1, target_id: 1, target_name: 'home', engine: 'ookla',
    options_snapshot: {}, status: 'ok', started_at: new Date().toISOString(),
    duration_ms: 1000, download_bps: 120e6, upload_bps: 30e6, ping_ms: 11.2,
    jitter_ms: 1.1, packet_loss_pct: 0, server_name: 'Init7', server_host: 'h',
    isp: 'Init7', result_url: '', tags: [],
  },
  count: 5, fail_count: 0, success_rate: 1,
  avg_download_bps: 110e6, min_download_bps: 90e6, max_download_bps: 130e6,
  avg_upload_bps: 28e6, avg_ping_ms: 12, max_ping_ms: 20,
  sla_compliance: null,
};

it('shows the latest download, upload, ping and a relative time', () => {
  render(<TargetCard summary={summary} spark={[1, 2, 3]} onRun={() => {}} running={false} />);
  expect(screen.getByText('120.0 Mbps')).toBeInTheDocument();
  expect(screen.getByText('30.0 Mbps')).toBeInTheDocument();
  expect(screen.getByText('11.2 ms')).toBeInTheDocument();
  expect(screen.getByText(/just now|s ago/)).toBeInTheDocument();
});

it('marks a failing target and still offers Run now', async () => {
  const onRun = vi.fn();
  render(<TargetCard
    summary={{ ...summary, latest: { ...summary.latest!, status: 'failed' } }}
    spark={[]} onRun={onRun} running={false} />);
  expect(screen.getByLabelText(/status: failed/i)).toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: /run now/i }));
  expect(onRun).toHaveBeenCalledWith(1);
});

it('opens a dialog with the full error text when View error is clicked', async () => {
  render(<TargetCard
    summary={{ ...summary, latest: { ...summary.latest!, status: 'failed', error: 'dial tcp: connection refused' } }}
    spark={[]} onRun={() => {}} running={false} />);
  expect(screen.queryByText('dial tcp: connection refused')).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: /view error/i }));
  expect(await screen.findByText('dial tcp: connection refused')).toBeInTheDocument();
});

it('does not show a View error button for a successful result', () => {
  render(<TargetCard summary={summary} spark={[]} onRun={() => {}} running={false} />);
  expect(screen.queryByRole('button', { name: /view error/i })).not.toBeInTheDocument();
});

it('renders a never-run card without a latest result', () => {
  render(<TargetCard summary={{ ...summary, latest: null }} spark={[]} onRun={() => {}} running={false} />);
  expect(screen.getByText(/never run/i)).toBeInTheDocument();
});

it('shows a neutral badge when no thresholds are configured', () => {
  render(<TargetCard summary={summary} spark={[]} onRun={() => {}} running={false} />);
  expect(screen.getByText('Within thresholds')).toBeInTheDocument();
});

it('shows "No data" for a target with no latest result', () => {
  render(<TargetCard summary={{ ...summary, latest: null }} spark={[]} onRun={() => {}} running={false}
    thresholds={{ ping_ms_max: 20 }} />);
  expect(screen.getByText('No data')).toBeInTheDocument();
});

it('flags a breached threshold', () => {
  render(<TargetCard summary={summary} spark={[]} onRun={() => {}} running={false}
    thresholds={{ ping_ms_max: 5 }} />);
  expect(screen.getByText('Threshold breached')).toBeInTheDocument();
});

it('shows an SLA badge when sla_compliance is non-null', () => {
  render(<TargetCard summary={{ ...summary, sla_compliance: 0.974 }} spark={[]} onRun={() => {}} running={false} />);
  expect(screen.getByText('SLA 97%')).toBeInTheDocument();
});

it('omits the SLA badge when sla_compliance is null', () => {
  render(<TargetCard summary={summary} spark={[]} onRun={() => {}} running={false} />);
  expect(screen.queryByText(/^SLA /)).not.toBeInTheDocument();
});
