import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { SettingsFormProvider } from '../../components/settings';
import { EnginesSection } from './EnginesSection';

const setEngines = vi.fn();

const engines = {
  speedtest_bin: 'speedtest',
  iperf3_bin: 'iperf3',
  ookla_accept_license: false,
  ookla_accept_gdpr: false,
  server_list_ttl_seconds: 86400,
  iperf3_list_url: 'https://example.com/servers.json',
};

vi.mock('./useSettingsSection', () => ({
  useSettingsSection: () => ({ engines, setEngines, saving: false, error: undefined, saved: false, readOnly: false }),
}));

vi.mock('./Iperf3ServerListSection', () => ({
  Iperf3ServerListSection: () => <span>iperf3-list-footer</span>,
}));

function renderSection() {
  return render(
    <SettingsFormProvider>
      <EnginesSection />
    </SettingsFormProvider>,
  );
}

describe('EnginesSection', () => {
  it('renders the Binaries, Ookla terms and iperf3 public server list cards', () => {
    renderSection();
    expect(screen.getByRole('region', { name: 'Binaries' })).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Ookla terms' })).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'iperf3 public server list' })).toBeInTheDocument();
    expect(screen.getByText('iperf3-list-footer')).toBeInTheDocument();
  });

  it('renders the binary path fields with their current values', () => {
    renderSection();
    expect(screen.getByLabelText('Speedtest binary path')).toHaveValue('speedtest');
    expect(screen.getByLabelText('iperf3 binary path')).toHaveValue('iperf3');
  });

  it('toggles the Ookla license switch', async () => {
    renderSection();
    await userEvent.click(screen.getByRole('switch', { name: 'Accept Ookla license' }));
    expect(setEngines).toHaveBeenCalledWith({ ...engines, ookla_accept_license: true });
  });

  it('toggles the Ookla GDPR switch', async () => {
    renderSection();
    await userEvent.click(screen.getByRole('switch', { name: 'Accept Ookla GDPR terms' }));
    expect(setEngines).toHaveBeenCalledWith({ ...engines, ookla_accept_gdpr: true });
  });

  it('edits the server list TTL', () => {
    renderSection();
    fireEvent.change(screen.getByLabelText('Server list TTL (seconds)'), { target: { value: '60' } });
    expect(setEngines).toHaveBeenLastCalledWith({ ...engines, server_list_ttl_seconds: 60 });
  });

  it('edits the iperf3 server list URL', () => {
    renderSection();
    fireEvent.change(screen.getByLabelText('iperf3 server list URL'), { target: { value: '' } });
    expect(setEngines).toHaveBeenLastCalledWith({ ...engines, iperf3_list_url: '' });
  });
});
