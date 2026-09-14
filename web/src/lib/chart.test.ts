import { describe, expect, it } from 'vitest';
import { formatAxisTick } from './chart';

describe('formatAxisTick', () => {
  it('renders a clock time when the plotted span is a day or less (24h chart)', () => {
    const got = formatAxisTick('2026-09-13T14:05:00.000Z', 24 * 60 * 60 * 1000);
    expect(got).toMatch(/^\d{2}:\d{2}$/);
    expect(got).not.toMatch(/[A-Za-z]/);
  });

  it('renders a calendar date when the plotted span exceeds a day (7d/30d chart)', () => {
    const got = formatAxisTick('2026-09-13T14:05:00.000Z', 7 * 24 * 60 * 60 * 1000);
    expect(got).toMatch(/[A-Za-z]{3} \d{1,2}/);
    expect(got).not.toMatch(/:/);
  });

  it('returns the raw input for an unparseable timestamp', () => {
    expect(formatAxisTick('not-a-date', 1000)).toBe('not-a-date');
  });
});
