import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { SettingsFormProvider } from '@/components/settings';
import type { IntegrationSettings } from '../../lib/api';
import { ExportersSection } from './ExportersSection';
import { useSettingsSection } from './useSettingsSection';

vi.mock('./useSettingsSection', () => ({ useSettingsSection: vi.fn() }));

const baseIntegrations: IntegrationSettings = {
  vm_enabled: false,
  vm_url: '',
  vm_auth_header: '',
  vm_auth_type: 'none',
  vm_auth_username: '',
  vm_auth_password: '',
  vm_auth_token: '',
  vm_auth_header_name: '',
  vm_auth_header_value: '',
  vm_extra_labels: {},
  vl_enabled: false,
  vl_url: '',
  vl_auth_header: '',
  vl_auth_type: 'none',
  vl_auth_username: '',
  vl_auth_password: '',
  vl_auth_token: '',
  vl_auth_header_name: '',
  vl_auth_header_value: '',
  vl_stream_fields: {},
  metrics_enabled: true,
};

function mockSection(overrides: Partial<{
  integrations: IntegrationSettings;
  setIntegrations: ReturnType<typeof vi.fn>;
  runTest: ReturnType<typeof vi.fn>;
  vmResult: { ok: boolean; message: string } | null;
  vlResult: { ok: boolean; message: string } | null;
  testPending: boolean;
  readOnly: boolean;
  locked: string[];
}> = {}) {
  const setIntegrations = overrides.setIntegrations ?? vi.fn();
  const runTest = overrides.runTest ?? vi.fn();
  (useSettingsSection as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
    integrations: overrides.integrations ?? baseIntegrations,
    setIntegrations,
    runTest,
    vmResult: overrides.vmResult ?? null,
    vlResult: overrides.vlResult ?? null,
    testPending: overrides.testPending ?? false,
    readOnly: overrides.readOnly ?? false,
    locked: overrides.locked ?? [],
  });
  return { setIntegrations, runTest };
}

function renderSection() {
  return render(
    <SettingsFormProvider>
      <ExportersSection />
    </SettingsFormProvider>,
  );
}

describe('ExportersSection', () => {
  it('renders the three cards', () => {
    mockSection();
    renderSection();
    expect(screen.getByRole('region', { name: 'VictoriaMetrics' })).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'VictoriaLogs' })).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Prometheus endpoint' })).toBeInTheDocument();
  });

  it('toggles the VictoriaMetrics enabled switch', async () => {
    const { setIntegrations } = mockSection();
    renderSection();
    await userEvent.click(screen.getByRole('switch', { name: 'Enable VictoriaMetrics' }));
    expect(setIntegrations).toHaveBeenCalledWith({ ...baseIntegrations, vm_enabled: true });
  });

  it('toggles the VictoriaLogs enabled switch', async () => {
    const { setIntegrations } = mockSection();
    renderSection();
    await userEvent.click(screen.getByRole('switch', { name: 'Enable VictoriaLogs' }));
    expect(setIntegrations).toHaveBeenCalledWith({ ...baseIntegrations, vl_enabled: true });
  });

  it('toggles the /metrics enabled switch', async () => {
    const { setIntegrations } = mockSection({ integrations: { ...baseIntegrations, metrics_enabled: false } });
    renderSection();
    await userEvent.click(screen.getByRole('switch', { name: 'Enable /metrics endpoint' }));
    expect(setIntegrations).toHaveBeenCalledWith({ ...baseIntegrations, metrics_enabled: true });
  });

  it('swaps auth rows when the auth type changes', async () => {
    function Stateful() {
      const [integrations, setIntegrations] = useState(baseIntegrations);
      (useSettingsSection as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
        integrations, setIntegrations, runTest: vi.fn(), vmResult: null, vlResult: null,
        testPending: false, readOnly: false, locked: [],
      });
      return <ExportersSection />;
    }
    render(<SettingsFormProvider><Stateful /></SettingsFormProvider>);
    expect(screen.queryByLabelText('VictoriaMetrics auth username')).not.toBeInTheDocument();
    await userEvent.selectOptions(screen.getByLabelText('VictoriaMetrics auth'), 'basic');
    expect(screen.getByLabelText('VictoriaMetrics auth username')).toBeInTheDocument();
    expect(screen.getByLabelText('VictoriaMetrics auth password')).toBeInTheDocument();
  });

  it('runs the VictoriaMetrics test with the current url and auth', async () => {
    const { runTest } = mockSection({
      integrations: { ...baseIntegrations, vm_url: 'http://vm.example.com', vm_auth_type: 'bearer', vm_auth_token: 'tok' },
    });
    renderSection();
    await userEvent.click(screen.getByRole('button', { name: 'Test VictoriaMetrics' }));
    expect(runTest).toHaveBeenCalledWith('vm', 'http://vm.example.com', {
      type: 'bearer',
      username: '',
      password: '',
      token: 'tok',
      header_name: '',
      header_value: '',
    });
  });

  it('shows the VictoriaMetrics test result', () => {
    mockSection({ vmResult: { ok: true, message: 'ok!' } });
    renderSection();
    expect(screen.getByText('ok!')).toBeInTheDocument();
  });

  it('disables the test buttons and shows a reason when read-only', () => {
    mockSection({ readOnly: true });
    renderSection();
    expect(screen.getByRole('button', { name: 'Test VictoriaMetrics' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Test VictoriaLogs' })).toBeDisabled();
    expect(screen.getAllByText('Read-only: admin group required')).toHaveLength(2);
  });

  it('disables a locked header switch', () => {
    mockSection({ locked: ['integrations.vm_enabled'] });
    renderSection();
    expect(screen.getByRole('switch', { name: 'Enable VictoriaMetrics' })).toBeDisabled();
    expect(screen.getByRole('switch', { name: 'Enable VictoriaLogs' })).not.toBeDisabled();
  });
});
