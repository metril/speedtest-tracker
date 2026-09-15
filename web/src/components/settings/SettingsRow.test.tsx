import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Input } from '@/components/ui/input';
import { SettingsFormProvider } from './settingsFormContext';
import { SettingsRow, useSettingsRowField } from './SettingsRow';

function Field() {
  const props = useSettingsRowField();
  return <Input {...props} />;
}

describe('SettingsRow', () => {
  it('renders label, description and error', () => {
    render(
      <SettingsRow label="Name" description="A hint" htmlFor="name" error="Required">
        <Field />
      </SettingsRow>,
    );
    expect(screen.getByText('Name')).toBeInTheDocument();
    expect(screen.getByText('A hint')).toBeInTheDocument();
    const input = screen.getByRole('textbox');
    expect(input).toHaveAttribute('id', 'name');
    expect(input).toHaveAttribute('aria-invalid', 'true');
    expect(input).toHaveAttribute('aria-describedby', 'name-error');
    expect(screen.getByText('Required')).toHaveAttribute('id', 'name-error');
  });

  it('shows a LockedBadge and disables the field when lockKey is locked', () => {
    render(
      <SettingsFormProvider locked={['secret']}>
        <SettingsRow label="Secret" htmlFor="secret" lockKey="secret">
          <Field />
        </SettingsRow>
      </SettingsFormProvider>,
    );
    expect(screen.getByText('set by environment')).toBeInTheDocument();
    expect(screen.getByRole('textbox')).toBeDisabled();
  });

  it('disables the field when the form is read-only', () => {
    render(
      <SettingsFormProvider readOnly>
        <SettingsRow label="Name" htmlFor="name">
          <Field />
        </SettingsRow>
      </SettingsFormProvider>,
    );
    expect(screen.getByRole('textbox')).toBeDisabled();
  });

  it('throws when useSettingsRowField is used outside a SettingsRow', () => {
    function Bare() {
      useSettingsRowField();
      return null;
    }
    expect(() => render(<Bare />)).toThrow();
  });
});
