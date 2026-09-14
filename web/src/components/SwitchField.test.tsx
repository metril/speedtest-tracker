import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { SwitchField } from './SwitchField';

describe('SwitchField', () => {
  it('renders a labelled switch, unchecked by default', () => {
    render(<SwitchField id="f" label="Enable thing" checked={false} onCheckedChange={vi.fn()} />);
    const el = screen.getByRole('switch', { name: 'Enable thing' });
    expect(el).toHaveAttribute('aria-checked', 'false');
  });

  it('toggles when the switch itself is clicked', async () => {
    const onCheckedChange = vi.fn();
    render(<SwitchField id="f" label="Enable thing" checked={false} onCheckedChange={onCheckedChange} />);
    await userEvent.click(screen.getByRole('switch', { name: 'Enable thing' }));
    expect(onCheckedChange).toHaveBeenCalledWith(true);
  });

  it('toggles when the label text is clicked', async () => {
    const onCheckedChange = vi.fn();
    render(<SwitchField id="f" label="Enable thing" checked={false} onCheckedChange={onCheckedChange} />);
    await userEvent.click(screen.getByText('Enable thing'));
    expect(onCheckedChange).toHaveBeenCalledWith(true);
  });

  it('is keyboard accessible: focusable and toggles with Space', async () => {
    const onCheckedChange = vi.fn();
    render(<SwitchField id="f" label="Enable thing" checked={false} onCheckedChange={onCheckedChange} />);
    const el = screen.getByRole('switch', { name: 'Enable thing' });
    await userEvent.tab();
    expect(el).toHaveFocus();
    await userEvent.keyboard(' ');
    expect(onCheckedChange).toHaveBeenCalledWith(true);
  });

  it('shows an optional hint and respects disabled', () => {
    render(
      <SwitchField id="f" label="Enable thing" hint="Extra detail" checked disabled onCheckedChange={vi.fn()} />,
    );
    expect(screen.getByText('Extra detail')).toBeInTheDocument();
    expect(screen.getByRole('switch', { name: 'Enable thing' })).toBeDisabled();
  });
});
