import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { SettingsSaveBar } from './SettingsSaveBar';

describe('SettingsSaveBar', () => {
  it('renders nothing when there are no changes and no error', () => {
    const { container } = render(
      <SettingsSaveBar dirtyCount={0} saving={false} onSave={vi.fn()} onDiscard={vi.fn()} />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it('renders the singular change count', () => {
    render(<SettingsSaveBar dirtyCount={1} saving={false} onSave={vi.fn()} onDiscard={vi.fn()} />);
    expect(screen.getByText('1 unsaved change')).toBeInTheDocument();
  });

  it('renders the plural change count', () => {
    render(<SettingsSaveBar dirtyCount={3} saving={false} onSave={vi.fn()} onDiscard={vi.fn()} />);
    expect(screen.getByText('3 unsaved changes')).toBeInTheDocument();
  });

  it('shows an error even with no dirty changes', () => {
    render(<SettingsSaveBar dirtyCount={0} saving={false} onSave={vi.fn()} onDiscard={vi.fn()} error="Boom" />);
    expect(screen.getByRole('alert')).toHaveTextContent('Boom');
  });

  it('calls onSave and onDiscard', async () => {
    const onSave = vi.fn();
    const onDiscard = vi.fn();
    render(<SettingsSaveBar dirtyCount={2} saving={false} onSave={onSave} onDiscard={onDiscard} />);
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    await userEvent.click(screen.getByRole('button', { name: 'Discard' }));
    expect(onSave).toHaveBeenCalled();
    expect(onDiscard).toHaveBeenCalled();
  });

  it('disables Save while saving', () => {
    render(<SettingsSaveBar dirtyCount={2} saving onSave={vi.fn()} onDiscard={vi.fn()} />);
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeDisabled();
  });

  it('shows a read-only message instead of buttons', () => {
    render(<SettingsSaveBar dirtyCount={2} saving={false} onSave={vi.fn()} onDiscard={vi.fn()} readOnly />);
    expect(screen.getByText('Read-only: admin group required')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Save changes' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Discard' })).not.toBeInTheDocument();
  });

  it('shows a brief Saved status instead of the bar when clean and flashing', () => {
    render(
      <SettingsSaveBar dirtyCount={0} saving={false} onSave={vi.fn()} onDiscard={vi.fn()} savedFlash />,
    );
    expect(screen.getByRole('status')).toHaveTextContent('Saved');
    expect(screen.queryByRole('region', { name: 'Unsaved changes' })).not.toBeInTheDocument();
  });

  it('renders nothing when clean and not flashing, even if savedFlash was passed false', () => {
    const { container } = render(
      <SettingsSaveBar dirtyCount={0} saving={false} onSave={vi.fn()} onDiscard={vi.fn()} savedFlash={false} />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
