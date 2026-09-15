import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import { SettingsFormProvider } from '@/components/settings';
import type { GeneralSettings } from '../../lib/api';
import { GeneralSection } from './GeneralSection';
import { useSettingsSection } from './useSettingsSection';

vi.mock('./useSettingsSection', () => ({ useSettingsSection: vi.fn() }));

function mockSection(general: GeneralSettings, setGeneral = vi.fn()) {
  (useSettingsSection as unknown as ReturnType<typeof vi.fn>).mockReturnValue({ general, setGeneral });
  return setGeneral;
}

const baseGeneral: GeneralSettings = {
  base_url: 'http://localhost',
  timezone: 'UTC',
  units: 'Mbps',
  log_level: 'info',
  retention_days_results: 30,
  retention_days_runs: 30,
  retention_prune_interval_minutes: 60,
  sla_download_mbps: 0,
  sla_upload_mbps: 0,
};

function renderSection(locked: string[] = []) {
  render(
    <MemoryRouter>
      <SettingsFormProvider locked={locked}>
        <GeneralSection />
      </SettingsFormProvider>
    </MemoryRouter>,
  );
}

describe('GeneralSection', () => {
  it('renders the four cards', () => {
    mockSection(baseGeneral);
    renderSection();
    expect(screen.getByRole('region', { name: 'Site' })).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Data retention' })).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Plan speeds (SLA)' })).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Diagnostics' })).toBeInTheDocument();
  });

  it('preserves existing field labels', () => {
    mockSection(baseGeneral);
    renderSection();
    expect(screen.getByLabelText('Base URL')).toBeInTheDocument();
    expect(screen.getByLabelText('Timezone')).toBeInTheDocument();
    expect(screen.getByLabelText('Units')).toBeInTheDocument();
    expect(screen.getByLabelText('Results retention (days)')).toBeInTheDocument();
    expect(screen.getByLabelText('Runs retention (days)')).toBeInTheDocument();
    expect(screen.getByLabelText('Prune interval (minutes)')).toBeInTheDocument();
    expect(screen.getByLabelText('Plan download (Mbps)')).toBeInTheDocument();
    expect(screen.getByLabelText('Plan upload (Mbps)')).toBeInTheDocument();
    expect(screen.getByLabelText('Log level')).toBeInTheDocument();
  });

  it('clears the SLA download field via the Clear button', async () => {
    const setGeneral = mockSection({ ...baseGeneral, sla_download_mbps: 1000 });
    renderSection();
    const input = screen.getByLabelText('Plan download (Mbps)');
    expect(input).toHaveValue(1000);
    await userEvent.click(screen.getAllByRole('button', { name: 'Clear' })[0]);
    expect(input).toHaveValue(null);
    expect(setGeneral).toHaveBeenCalledWith(expect.objectContaining({ sla_download_mbps: 0 }));
  });

  it('disables fields for a locked key', () => {
    mockSection(baseGeneral);
    renderSection(['general.base_url']);
    expect(screen.getByRole('textbox', { name: /Base URL/ })).toBeDisabled();
    expect(screen.getByLabelText('Timezone')).not.toBeDisabled();
  });
});
