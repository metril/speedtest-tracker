import { describe, expect, it } from 'vitest';
import { formatBytes, splitBytes, toBytes } from './bytes';

describe('toBytes', () => {
  it('converts each unit to bytes', () => {
    expect(toBytes(1, 'B')).toBe(1);
    expect(toBytes(1, 'KB')).toBe(1000);
    expect(toBytes(1, 'MB')).toBe(1_000_000);
    expect(toBytes(1, 'GB')).toBe(1_000_000_000);
    expect(toBytes(1.5, 'MB')).toBe(1_500_000);
  });
});

describe('splitBytes', () => {
  it('round-trips through toBytes for each unit', () => {
    expect(splitBytes(100_000)).toEqual({ value: 100, unit: 'KB' });
    expect(splitBytes(1_000_000)).toEqual({ value: 1, unit: 'MB' });
    expect(splitBytes(1_000_000_000)).toEqual({ value: 1, unit: 'GB' });
    expect(splitBytes(1_500_000)).toEqual({ value: 1500, unit: 'KB' });
  });

  it('falls back to B when nothing divides evenly', () => {
    expect(splitBytes(1_234_567)).toEqual({ value: 1_234_567, unit: 'B' });
  });

  it('falls back to B for 0 and non-finite values', () => {
    expect(splitBytes(0)).toEqual({ value: 0, unit: 'B' });
    expect(splitBytes(NaN)).toEqual({ value: NaN, unit: 'B' });
    expect(splitBytes(Infinity)).toEqual({ value: Infinity, unit: 'B' });
  });
});

describe('formatBytes', () => {
  it('formats known sizes', () => {
    expect(formatBytes(1e5)).toBe('100 KB');
    expect(formatBytes(1e6)).toBe('1 MB');
    expect(formatBytes(2.5e7)).toBe('25 MB');
    expect(formatBytes(1e9)).toBe('1 GB');
    expect(formatBytes(1_500_000)).toBe('1500 KB');
  });
});
