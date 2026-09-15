import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { SettingsFormProvider, isLocked, useSettingsForm } from './settingsFormContext';

function Probe() {
  const { readOnly, locked } = useSettingsForm();
  return <span>{`readOnly=${readOnly} locked=${locked.join(',')}`}</span>;
}

describe('settingsFormContext', () => {
  it('defaults to not read-only with no locked keys', () => {
    render(<Probe />);
    expect(screen.getByText('readOnly=false locked=')).toBeInTheDocument();
  });

  it('provides overridden values to descendants', () => {
    render(
      <SettingsFormProvider readOnly locked={['a', 'b']}>
        <Probe />
      </SettingsFormProvider>,
    );
    expect(screen.getByText('readOnly=true locked=a,b')).toBeInTheDocument();
  });

  it('isLocked checks membership', () => {
    expect(isLocked(['a', 'b'], 'a')).toBe(true);
    expect(isLocked(['a', 'b'], 'c')).toBe(false);
    expect(isLocked(['a', 'b'], undefined)).toBe(false);
  });
});
