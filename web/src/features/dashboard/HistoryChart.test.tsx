import { render, screen } from '@testing-library/react';
import { expect, it } from 'vitest';
import { formatBps } from '../../lib/format';
import { SERIES } from '../../lib/chart';
import { HistoryChart } from './HistoryChart';

const points = [
  { bucket_start: '2026-09-13T10:00:00.000Z', count: 2, fail_count: 0,
    avg_download_bps: 100e6, min_download_bps: 90e6, max_download_bps: 110e6,
    avg_upload_bps: 20e6, min_upload_bps: 18e6, max_upload_bps: 22e6,
    avg_ping_ms: 12, min_ping_ms: 10, max_ping_ms: 14, avg_jitter_ms: 1 },
  { bucket_start: '2026-09-13T11:00:00.000Z', count: 2, fail_count: 1,
    avg_download_bps: 50e6, min_download_bps: 40e6, max_download_bps: 60e6,
    avg_upload_bps: 10e6, min_upload_bps: 9e6, max_upload_bps: 11e6,
    avg_ping_ms: 30, min_ping_ms: 25, max_ping_ms: 40, avg_jitter_ms: 4 },
];

const throughput = [
  { key: 'avg_download_bps' as const, label: 'Download', color: SERIES.download, unit: formatBps },
  { key: 'avg_upload_bps' as const, label: 'Upload', color: SERIES.upload, unit: formatBps },
];

it('exposes an accessible summary with min, max and latest per series', () => {
  render(<HistoryChart title="Throughput" points={points} series={throughput} />);
  const fig = screen.getByRole('img', { name: /throughput/i });
  expect(fig.getAttribute('aria-label')).toMatch(/Download.*min 50\.0 Mbps.*max 100\.0 Mbps.*latest 50\.0 Mbps/);
  expect(fig.getAttribute('aria-label')).toMatch(/Upload/);
});

it('renders an empty state instead of an axis-only chart', () => {
  render(<HistoryChart title="Throughput" points={[]} series={throughput} />);
  expect(screen.getByText(/no data in this range/i)).toBeInTheDocument();
});
