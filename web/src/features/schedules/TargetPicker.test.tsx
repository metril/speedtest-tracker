import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { Target } from '../../lib/api';
import { TargetPicker } from './TargetPicker';

function target(id: number, name: string, engine: string, queueName: string): Target {
  return {
    id, name, engine, enabled: true, queue_id: queueName === 'wan' ? 1 : 2, queue_name: queueName,
    options: {}, thresholds: {}, created_at: '', updated_at: '',
  };
}

const targets = [
  target(1, 'home-wan', 'ookla', 'wan'),
  target(2, 'home-lan', 'iperf3', 'lan'),
  target(3, 'office-wan', 'cloudflare', 'wan'),
  target(4, 'lab', 'fake', 'lan'),
];

describe('TargetPicker', () => {
  it('filters the list by name, case-insensitively', async () => {
    const onChange = vi.fn();
    render(<TargetPicker targets={targets} selected={[]} onChange={onChange} />);
    await userEvent.type(screen.getByLabelText('Search targets'), 'HOME');
    expect(screen.getByText('home-wan')).toBeInTheDocument();
    expect(screen.getByText('home-lan')).toBeInTheDocument();
    expect(screen.queryByText('office-wan')).not.toBeInTheDocument();
    expect(screen.queryByText('lab')).not.toBeInTheDocument();
  });

  it('filters by engine and lane chips', async () => {
    const onChange = vi.fn();
    render(<TargetPicker targets={targets} selected={[]} onChange={onChange} />);
    await userEvent.click(screen.getByRole('button', { name: 'wan' }));
    expect(screen.getByText('home-wan')).toBeInTheDocument();
    expect(screen.getByText('office-wan')).toBeInTheDocument();
    expect(screen.queryByText('home-lan')).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'ookla' }));
    expect(screen.getByText('home-wan')).toBeInTheDocument();
    expect(screen.queryByText('office-wan')).not.toBeInTheDocument();
  });

  it('checking a box appends to selection; unchecking removes it', async () => {
    const onChange = vi.fn();
    render(<TargetPicker targets={targets} selected={[1]} onChange={onChange} />);

    await userEvent.click(screen.getByRole('checkbox', { name: /home-lan/ }));
    expect(onChange).toHaveBeenCalledWith([1, 2]);

    await userEvent.click(screen.getByRole('checkbox', { name: /home-wan/ }));
    expect(onChange).toHaveBeenCalledWith([]);
  });

  it('selects and clears all matching targets in bulk', async () => {
    const onChange = vi.fn();
    const { rerender } = render(<TargetPicker targets={targets} selected={[]} onChange={onChange} />);
    await userEvent.click(screen.getByRole('button', { name: 'wan' }));
    await userEvent.click(screen.getByRole('button', { name: 'Select all matching' }));
    expect(onChange).toHaveBeenCalledWith([1, 3]);

    rerender(<TargetPicker targets={targets} selected={[1, 3, 4]} onChange={onChange} />);
    await userEvent.click(screen.getByRole('button', { name: 'Clear matching' }));
    expect(onChange).toHaveBeenCalledWith([4]);
  });
});
