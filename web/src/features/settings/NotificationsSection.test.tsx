import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import { SettingsFormProvider } from '@/components/settings';
import type { NotificationSettings, NotifyChannel } from '../../lib/api';
import { NotificationsSection } from './NotificationsSection';
import { useSettingsSection } from './useSettingsSection';

vi.mock('./useSettingsSection', () => ({ useSettingsSection: vi.fn() }));

const baseNotifications: NotificationSettings = {
  enabled: true,
  channels: [],
  default_thresholds: {},
  cooldown_minutes: 30,
  quiet_hours_start: '',
  quiet_hours_end: '',
  notify_recovery: true,
};

function mockSection(overrides: Partial<ReturnType<typeof useSettingsSection>> = {}) {
  const setNotifications = vi.fn();
  const addChannel = vi.fn();
  (useSettingsSection as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
    notifications: baseNotifications,
    setNotifications,
    readOnly: false,
    error: undefined,
    addChannel,
    updateChannel: vi.fn(),
    removeChannel: vi.fn(),
    runChannelTest: vi.fn(),
    channelResults: {},
    testingChannelId: null,
    savedChannels: [],
    ...overrides,
  });
  return { setNotifications, addChannel };
}

function renderSection(locked: string[] = []) {
  render(
    <MemoryRouter>
      <SettingsFormProvider locked={locked}>
        <NotificationsSection />
      </SettingsFormProvider>
    </MemoryRouter>,
  );
}

describe('NotificationsSection', () => {
  it('renders the three cards', () => {
    mockSection();
    renderSection();
    expect(screen.getByRole('region', { name: 'Delivery' })).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Channels' })).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Default thresholds' })).toBeInTheDocument();
  });

  it('toggles Enabled from the Delivery header switch', async () => {
    const { setNotifications } = mockSection();
    renderSection();
    await userEvent.click(screen.getByRole('switch', { name: 'Enabled' }));
    expect(setNotifications).toHaveBeenCalledWith(expect.objectContaining({ enabled: false }));
  });

  it('renders quiet hours as two time inputs', async () => {
    const { setNotifications } = mockSection();
    renderSection();
    const start = screen.getByLabelText('Quiet hours start');
    const end = screen.getByLabelText('Quiet hours end');
    expect(start).toHaveAttribute('type', 'time');
    expect(end).toHaveAttribute('type', 'time');
    await userEvent.type(start, '08:00');
    expect(setNotifications).toHaveBeenCalledWith(expect.objectContaining({ quiet_hours_start: '08:00' }));
  });

  it('shows the empty-channels state', () => {
    mockSection();
    renderSection();
    expect(screen.getByText('No channels yet — nothing will be delivered.')).toBeInTheDocument();
  });

  it('renders a ChannelEditor per channel and calls addChannel from the footer button', async () => {
    const channel: NotifyChannel = { id: 'c1', type: 'webhook', name: 'hook', enabled: true, url: 'https://x' };
    const { addChannel } = mockSection({ notifications: { ...baseNotifications, channels: [channel] } });
    renderSection();
    expect(screen.queryByText('No channels yet — nothing will be delivered.')).not.toBeInTheDocument();
    expect(screen.getByText('hook')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Add channel' }));
    expect(addChannel).toHaveBeenCalled();
  });

  it('highlights the channel card a save error names', () => {
    const channel: NotifyChannel = { id: 'c1', type: 'webhook', name: 'Discord', enabled: true, url: '' };
    mockSection({
      notifications: { ...baseNotifications, channels: [channel] },
      error: 'channel "Discord": url must be an absolute http(s) URL',
    });
    renderSection();
    expect(screen.getByText('channel "Discord": url must be an absolute http(s) URL')).toBeInTheDocument();
  });

  it('disables the Delivery switch and Add channel button when read-only', () => {
    mockSection({ readOnly: true });
    renderSection();
    expect(screen.getByRole('switch', { name: 'Enabled' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Add channel' })).toBeDisabled();
  });
});
