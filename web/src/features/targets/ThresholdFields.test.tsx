import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ThresholdFields } from './ThresholdFields';

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
});
