import { describe, expect, it } from 'vitest';
import { formatBps, formatDateTime, formatLoss, formatMs, formatRelative } from './format';

describe('formatBps', () => {
  it('renders Mbps with one decimal', () => {
    expect(formatBps(94_300_000)).toBe('94.3 Mbps');
    expect(formatBps(1_000_000)).toBe('1.0 Mbps');
  });
  it('drops to Kbps below 1 Mbps', () => {
    expect(formatBps(512_000)).toBe('512 Kbps');
  });
  it('climbs to Gbps above 1000 Mbps', () => {
    expect(formatBps(2_400_000_000)).toBe('2.40 Gbps');
  });
  it('renders a dash for missing values', () => {
    expect(formatBps(0)).toBe('—');
    expect(formatBps(Number.NaN)).toBe('—');
  });
});

describe('formatMs', () => {
  it('uses one decimal under 100ms', () => {
    expect(formatMs(12.34)).toBe('12.3 ms');
  });
  it('rounds above 100ms', () => {
    expect(formatMs(143.7)).toBe('144 ms');
  });
  it('renders a dash for missing values', () => {
    expect(formatMs(0)).toBe('—');
  });
});

describe('formatLoss', () => {
  it('renders a percentage with one decimal', () => {
    expect(formatLoss(1.25)).toBe('1.3%');
    expect(formatLoss(0)).toBe('0%');
  });
});

describe('formatRelative', () => {
  const now = new Date('2026-09-13T12:00:00.000Z');
  it('renders seconds, minutes, hours and days', () => {
    expect(formatRelative('2026-09-13T11:59:30.000Z', now)).toBe('30s ago');
    expect(formatRelative('2026-09-13T11:45:00.000Z', now)).toBe('15m ago');
    expect(formatRelative('2026-09-13T09:00:00.000Z', now)).toBe('3h ago');
    expect(formatRelative('2026-09-10T12:00:00.000Z', now)).toBe('3d ago');
  });
  it('renders just now for the last few seconds', () => {
    expect(formatRelative('2026-09-13T11:59:58.000Z', now)).toBe('just now');
  });
  it('survives an unparsable timestamp', () => {
    expect(formatRelative('not-a-date', now)).toBe('—');
  });
});

describe('formatDateTime', () => {
  it('renders a sortable local timestamp', () => {
    expect(formatDateTime('2026-09-13T12:00:00.000Z')).toMatch(/2026/);
  });
  it('survives an unparsable timestamp', () => {
    expect(formatDateTime('nope')).toBe('—');
  });
});

import { formatBytes, formatPercent } from './format';

describe('formatPercent', () => {
  it('renders a 0..1 fraction with one decimal, dropping a trailing .0', () => {
    expect(formatPercent(1)).toBe('100%');
    expect(formatPercent(0.9987)).toBe('99.9%');
    expect(formatPercent(0)).toBe('0%');
  });
  it('renders a dash for a non-finite value', () => {
    expect(formatPercent(Number.NaN)).toBe('—');
  });
});

describe('formatBytes', () => {
  it('scales to KB/MB/GB', () => {
    expect(formatBytes(512)).toBe('512 B');
    expect(formatBytes(2048)).toBe('2.0 KB');
    expect(formatBytes(5 * 1024 * 1024)).toBe('5.0 MB');
    expect(formatBytes(3 * 1024 ** 3)).toBe('3.0 GB');
  });
  it('renders a dash for zero or nonsense', () => {
    expect(formatBytes(0)).toBe('—');
  });
});
