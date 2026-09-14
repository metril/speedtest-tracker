import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { CURATED_TIMEZONES, TimezoneSelect } from './TimezoneSelect';

describe('TimezoneSelect', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('shows curated zones first, then all zones, when supportedValuesOf exists', () => {
    (Intl as unknown as { supportedValuesOf: (k: string) => string[] }).supportedValuesOf =
      () => ['UTC', 'America/New_York', 'Pacific/Auckland', 'Africa/Cairo'];
    render(<TimezoneSelect id="tz" value="UTC" onChange={() => {}} />);
    const select = screen.getByRole('combobox');
    const groups = select.querySelectorAll('optgroup');
    expect(groups).toHaveLength(2);
    expect(groups[0].getAttribute('label')).toBe('Common');
    expect(groups[1].getAttribute('label')).toBe('All zones');
    // Curated zones aren't duplicated in "All zones".
    expect(groups[1].querySelector('option[value="UTC"]')).toBeNull();
    expect(groups[1].querySelector('option[value="Africa/Cairo"]')).not.toBeNull();
    delete (Intl as unknown as { supportedValuesOf?: unknown }).supportedValuesOf;
  });

  it('falls back to curated list plus free-text input when supportedValuesOf is absent', () => {
    delete (Intl as unknown as { supportedValuesOf?: unknown }).supportedValuesOf;
    render(<TimezoneSelect id="tz" value="UTC" onChange={() => {}} />);
    expect(screen.queryByRole('combobox')).toBeInTheDocument();
    expect(screen.getByLabelText('Custom timezone')).toBeInTheDocument();
    expect(screen.queryAllByRole('option')).toHaveLength(CURATED_TIMEZONES.length);
  });

  it('keeps an unknown value selected as a disabled Custom… option in fallback mode', () => {
    delete (Intl as unknown as { supportedValuesOf?: unknown }).supportedValuesOf;
    render(<TimezoneSelect id="tz" value="Antarctica/Vostok" onChange={() => {}} />);
    const select = screen.getByRole('combobox') as HTMLSelectElement;
    expect(select.value).toBe('__custom__');
    expect(screen.getByLabelText('Custom timezone')).toHaveValue('Antarctica/Vostok');
  });

  it('keeps an unknown value selected when the full zone list is available', () => {
    (Intl as unknown as { supportedValuesOf: (k: string) => string[] }).supportedValuesOf =
      () => ['UTC', 'America/New_York'];
    render(<TimezoneSelect id="tz" value="Antarctica/Vostok" onChange={() => {}} />);
    const select = screen.getByRole('combobox') as HTMLSelectElement;
    expect(select.value).toBe('Antarctica/Vostok');
    delete (Intl as unknown as { supportedValuesOf?: unknown }).supportedValuesOf;
  });

  it('calls onChange when a new value is picked', async () => {
    delete (Intl as unknown as { supportedValuesOf?: unknown }).supportedValuesOf;
    const onChange = vi.fn();
    render(<TimezoneSelect id="tz" value="UTC" onChange={onChange} />);
    await userEvent.selectOptions(screen.getByRole('combobox'), 'Europe/London');
    expect(onChange).toHaveBeenCalledWith('Europe/London');
  });
});
