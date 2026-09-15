import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { TestButton } from './TestButton';

describe('TestButton', () => {
  it('fires onTest when clicked', async () => {
    const onTest = vi.fn();
    render(<TestButton label="Test channel" onTest={onTest} pending={false} />);
    await userEvent.click(screen.getByRole('button', { name: 'Test channel' }));
    expect(onTest).toHaveBeenCalled();
  });

  it('disables while pending', () => {
    render(<TestButton label="Test channel" onTest={vi.fn()} pending />);
    expect(screen.getByRole('button', { name: 'Test channel' })).toBeDisabled();
  });

  it('shows a success result', () => {
    render(
      <TestButton label="Test channel" onTest={vi.fn()} pending={false} result={{ ok: true, message: 'Sent' }} />,
    );
    expect(screen.getByText('Sent')).toHaveClass('text-ok');
  });

  it('shows a failure result', () => {
    render(
      <TestButton label="Test channel" onTest={vi.fn()} pending={false} result={{ ok: false, message: 'Failed' }} />,
    );
    expect(screen.getByText('Failed')).toHaveClass('text-bad');
  });

  it('shows the disabled reason instead of a result when disabled', () => {
    render(
      <TestButton
        label="Test channel"
        onTest={vi.fn()}
        pending={false}
        disabled
        disabledReason="Save first to test this channel"
      />,
    );
    expect(screen.getByText('Save first to test this channel')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Test channel' })).toBeDisabled();
  });
});
