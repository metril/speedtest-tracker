import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Gauge, arcPath, gaugeFraction } from './Gauge';

describe('gaugeFraction', () => {
  it('is 0 at zero and 1 at the maximum', () => {
    expect(gaugeFraction(0)).toBe(0);
    expect(gaugeFraction(1000 * 1_000_000)).toBeCloseTo(1, 5);
  });

  it('clamps out-of-range and non-finite input', () => {
    expect(gaugeFraction(-5)).toBe(0);
    expect(gaugeFraction(Number.NaN)).toBe(0);
    expect(gaugeFraction(50_000 * 1_000_000)).toBe(1);
  });

  it('is log-scaled so the low end is legible', () => {
    const tenth = gaugeFraction(100 * 1_000_000);
    expect(tenth).toBeGreaterThan(0.55);
    expect(tenth).toBeLessThan(0.75);
  });

  it('is monotonic', () => {
    expect(gaugeFraction(20e6)).toBeLessThan(gaugeFraction(200e6));
  });
});

describe('arcPath', () => {
  it('emits a single arc command anchored at the start point', () => {
    const d = arcPath(100, 100, 80, -210, 30);
    expect(d.startsWith('M ')).toBe(true);
    expect(d).toContain('A 80 80');
    expect(d.split('A').length).toBe(2);
  });
});

describe('Gauge', () => {
  it('shows the formatted throughput and the phase', () => {
    render(<Gauge bps={123_400_000} phase="download" />);
    expect(screen.getByText('123.4')).toBeInTheDocument();
    expect(screen.getByText('Mbps')).toBeInTheDocument();
    expect(screen.getByText('download')).toBeInTheDocument();
  });

  it('exposes the reading to assistive tech', () => {
    render(<Gauge bps={50_000_000} phase="upload" />);
    const meter = screen.getByRole('meter');
    expect(meter).toHaveAttribute('aria-valuenow', '50');
    expect(meter).toHaveAttribute('aria-valuetext', '50.0 Mbps');
  });
});
