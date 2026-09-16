import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Sparkline, sparkPath } from './Sparkline';

describe('sparkPath', () => {
  it('is empty for fewer than two samples', () => {
    expect(sparkPath([], 120, 32)).toBe('');
    expect(sparkPath([5], 120, 32)).toBe('');
  });

  it('spans the full width and puts the peak at the top', () => {
    const d = sparkPath([0, 10], 100, 20);
    expect(d).toBe('M 0 20 L 100 0');
  });

  it('keeps a flat series on the baseline instead of dividing by zero', () => {
    const d = sparkPath([7, 7, 7], 100, 20);
    expect(d).toBe('M 0 20 L 50 20 L 100 20');
  });
});

describe('Sparkline', () => {
  it('renders nothing until there are two samples', () => {
    const { container } = render(<Sparkline samples={[1]} />);
    expect(container.querySelector('path')).toBeNull();
  });

  it('renders a path once samples arrive', () => {
    const { container } = render(<Sparkline samples={[1, 5, 3]} />);
    expect(container.querySelector('path')).not.toBeNull();
  });
});

describe('Sparkline phase colour', () => {
  it('matches the gauge accent for the current phase', () => {
    const { container } = render(<Sparkline samples={[1, 2, 3]} phase="upload" />);
    expect(container.querySelector('path')?.getAttribute('stroke')).toBe('var(--color-series-upload)');
  });
});
