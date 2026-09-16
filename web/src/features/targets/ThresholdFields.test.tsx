import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ThresholdFields, validateThresholds } from './ThresholdFields';

describe('ThresholdFields', () => {
  it('shows the Custom notification toggle by default, off with empty value', () => {
    render(<ThresholdFields value={{}} onChange={vi.fn()} />);
    expect(screen.getByLabelText('Custom notification')).toBeInTheDocument();
    expect(screen.queryByText('Min download (Mbps) mode')).not.toBeInTheDocument();
  });

  it('shows fields when the value already has overrides', () => {
    render(<ThresholdFields value={{ download_mbps_min: 10 }} onChange={vi.fn()} />);
    expect(screen.getByLabelText('Min download (Mbps) mode')).toBeInTheDocument();
  });

  it('hides the toggle and behaves as always-custom when showCustomToggle is false', () => {
    render(<ThresholdFields value={{}} onChange={vi.fn()} showCustomToggle={false} />);
    expect(screen.queryByLabelText('Custom notification')).not.toBeInTheDocument();
    expect(screen.getByLabelText('Min download (Mbps) mode')).toBeInTheDocument();
  });

  it('shows the SLA tolerance override field with overrides present', () => {
    render(<ThresholdFields value={{ sla_tolerance_pct: 0 }} onChange={vi.fn()} />);
    expect(screen.getByLabelText('SLA tolerance override (%)')).toHaveValue(0);
  });
});

describe('validateThresholds SLA tolerance', () => {
  it('accepts 0 and 99', () => {
    expect(validateThresholds({ sla_tolerance_pct: 0 })).toBeUndefined();
    expect(validateThresholds({ sla_tolerance_pct: 99 })).toBeUndefined();
  });

  it('rejects out-of-range values', () => {
    expect(validateThresholds({ sla_tolerance_pct: -1 })).toBe('SLA tolerance must be 0-99');
    expect(validateThresholds({ sla_tolerance_pct: 100 })).toBe('SLA tolerance must be 0-99');
  });
});
