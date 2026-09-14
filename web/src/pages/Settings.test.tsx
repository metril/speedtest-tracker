import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from '../lib/api';
import { ApiError } from '../lib/api';
import type { NotifyChannel, Settings as SettingsType } from '../lib/api';
import { Settings } from './Settings';

function jsonResponse(body: unknown, status = 200): Response {
  return { ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body) } as Response;
}

type FixtureOverrides = Partial<SettingsType['integrations']> & { channels?: NotifyChannel[] };

function settingsFixture({ channels, ...integrationOverrides }: FixtureOverrides = {}): SettingsType {
  return {
    general: {
      base_url: 'http://localhost:8080',
      timezone: 'UTC',
      units: 'Mbps',
      log_level: 'info',
      retention_days_results: 90,
      retention_days_runs: 30,
      retention_prune_interval_minutes: 60,
    },
    engines: {
      speedtest_bin: 'speedtest',
      iperf3_bin: 'iperf3',
      ookla_accept_license: true,
      ookla_accept_gdpr: true,
      server_list_ttl_seconds: 3600,
      default_ookla_options: {},
      default_cloudflare_options: {},
      default_iperf3_options: {},
    },
    integrations: {
      vm_enabled: true,
      vm_url: 'http://vm:8428',
      vm_auth_header: '***',
      vm_extra_labels: { host: 'pi4' },
      vl_enabled: true,
      vl_url: 'http://vl:9428',
      vl_auth_header: '***',
      vl_stream_fields: {},
      metrics_enabled: true,
      ...integrationOverrides,
    },
    notifications: {
      enabled: false,
      channels: channels ?? [],
      default_thresholds: {},
      cooldown_minutes: 30,
      quiet_hours_start: '',
      quiet_hours_end: '',
      notify_recovery: true,
    },
  };
}

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function renderSettings(opts: {
  put?: ReturnType<typeof vi.fn>;
  test?: ReturnType<typeof vi.fn>;
  testChannel?: ReturnType<typeof vi.fn>;
  settings?: SettingsType;
} = {}) {
  const settings = opts.settings ?? settingsFixture();
  fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.startsWith('/api/v1/settings')) return jsonResponse(settings);
    throw new Error(`unexpected fetch: ${url}`);
  });
  if (opts.put) vi.spyOn(api, 'updateSettings').mockImplementation(opts.put);
  if (opts.test) vi.spyOn(api, 'testIntegration').mockImplementation(opts.test);
  if (opts.testChannel) vi.spyOn(api, 'testNotifyChannel').mockImplementation(opts.testChannel);

  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrap = (node: ReactNode) => <QueryClientProvider client={qc}>{node}</QueryClientProvider>;
  return { ...render(wrap(<Settings />)), qc };
}

describe('Settings page', () => {
  it('saves only the edited section', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({ put });
    await screen.findByLabelText('Timezone');
    await userEvent.clear(screen.getByLabelText('Results retention (days)'));
    await userEvent.type(screen.getByLabelText('Results retention (days)'), '30');
    await userEvent.click(
      within(screen.getByRole('region', { name: 'General' })).getByRole('button', { name: 'Save General' }),
    );
    expect(put).toHaveBeenCalledWith({ general: expect.objectContaining({ retention_days_results: 30 }) });
    expect(put.mock.calls[0][0].integrations).toBeUndefined();
  });

  it('keeps a stored secret when the field is left untouched', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({ put, settings: settingsFixture({ vm_auth_header: '***' }) });
    await userEvent.click(await screen.findByRole('button', { name: 'Save Integrations' }));
    expect(put.mock.calls[0][0].integrations.vm_auth_header).toBe('***');
  });

  it('shows the server validation message inline', async () => {
    const put = vi.fn().mockRejectedValue(new ApiError(400, 'invalid_request', 'vm_url must be http or https'));
    renderSettings({ put });
    await userEvent.click(await screen.findByRole('button', { name: 'Save Integrations' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('vm_url must be http or https');
  });

  it('does not clobber an edited but unsaved field on refetch', async () => {
    const { qc } = renderSettings();
    const tz = await screen.findByLabelText('Timezone');
    await userEvent.clear(tz);
    await userEvent.type(tz, 'Asia/Kolkata');

    // A refetch (e.g. invalidation, background refresh) brings back the
    // original server data; the in-progress, unsaved edit must survive it.
    await qc.refetchQueries({ queryKey: ['settings'] });

    expect(screen.getByLabelText('Timezone')).toHaveValue('Asia/Kolkata');
  });

  it('reports a connection test result inline', async () => {
    const test = vi.fn().mockResolvedValue({ ok: false, error: 'connection refused' });
    renderSettings({ test });
    await userEvent.click(await screen.findByRole('button', { name: 'Test VictoriaMetrics' }));
    expect(await screen.findByText(/connection refused/)).toBeInTheDocument();
  });

  it('saves only the notifications section', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({ put });
    await screen.findByLabelText('Cooldown (minutes)');
    await userEvent.clear(screen.getByLabelText('Cooldown (minutes)'));
    await userEvent.type(screen.getByLabelText('Cooldown (minutes)'), '15');
    await userEvent.click(within(screen.getByRole('region', { name: 'Notifications' }))
      .getByRole('button', { name: 'Save Notifications' }));
    expect(put).toHaveBeenCalledWith({ notifications: expect.objectContaining({ cooldown_minutes: 15 }) });
    expect(put.mock.calls[0][0].general).toBeUndefined();
  });

  it('adds a channel and echoes an untouched token back masked', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({
      put,
      settings: settingsFixture({
        channels: [{
          id: 'c1', type: 'ntfy', name: 'phone', enabled: true, url: 'https://ntfy.sh/x', token: '***',
        }],
      }),
    });
    await userEvent.click(await screen.findByRole('button', { name: 'Add channel' }));
    await userEvent.click(screen.getByRole('button', { name: 'Save Notifications' }));
    const sent = put.mock.calls[0][0].notifications.channels;
    expect(sent).toHaveLength(2);
    expect(sent[0].token).toBe('***');
  });

  it('reports a channel test result inline', async () => {
    const testChannel = vi.fn().mockResolvedValue({ ok: false, error: 'connection refused' });
    renderSettings({
      testChannel,
      settings: settingsFixture({
        channels: [{
          id: 'c1', type: 'ntfy', name: 'phone', enabled: true, url: 'https://ntfy.sh/x',
        }],
      }),
    });
    await userEvent.click(await screen.findByRole('button', { name: 'Test phone' }));
    expect(await screen.findByText(/connection refused/)).toBeInTheDocument();
  });

  it('disables Test with a save-first hint for a dirty/unsaved channel', async () => {
    renderSettings({
      settings: settingsFixture({
        channels: [{
          id: 'c1', type: 'ntfy', name: 'phone', enabled: true, url: 'https://ntfy.sh/x',
        }],
      }),
    });

    // A brand-new, never-saved channel: Test is disabled with the hint.
    await userEvent.click(await screen.findByRole('button', { name: 'Add channel' }));
    expect(screen.getByRole('button', { name: 'Test New channel' })).toBeDisabled();
    expect(screen.getAllByText('Save first to test this channel')).toHaveLength(1);

    // The already-saved channel is untouched: Test stays enabled.
    expect(screen.getByRole('button', { name: 'Test phone' })).toBeEnabled();

    // Editing the saved channel without saving disables its Test button too.
    await userEvent.type(screen.getByDisplayValue('phone'), 'x');
    expect(screen.getByRole('button', { name: 'Test phonex' })).toBeDisabled();
    expect(screen.getAllByText('Save first to test this channel')).toHaveLength(2);
  });

  it('scopes the test-pending state to the channel under test', async () => {
    let resolveTest: (result: { ok: boolean }) => void = () => {};
    const testChannel = vi.fn().mockImplementation(() => new Promise((resolve) => { resolveTest = resolve; }));
    renderSettings({
      testChannel,
      settings: settingsFixture({
        channels: [
          {
            id: 'c1', type: 'ntfy', name: 'phone', enabled: true, url: 'https://ntfy.sh/x',
          },
          {
            id: 'c2', type: 'ntfy', name: 'laptop', enabled: true, url: 'https://ntfy.sh/y',
          },
        ],
      }),
    });

    const phoneButton = await screen.findByRole('button', { name: 'Test phone' });
    const laptopButton = screen.getByRole('button', { name: 'Test laptop' });
    await userEvent.click(phoneButton);

    expect(phoneButton).toBeDisabled();
    expect(laptopButton).toBeEnabled();
    expect(testChannel).toHaveBeenCalledWith('c1');
    expect(testChannel).toHaveBeenCalledTimes(1);

    resolveTest({ ok: true });
    await screen.findByText(/Sent in/);
    expect(phoneButton).toBeEnabled();
  });

  it('shows only the fields the selected channel type uses', async () => {
    renderSettings({
      settings: settingsFixture({
        channels: [{
          id: 'c1', type: 'webhook', name: 'hook', enabled: true, url: 'https://hook',
        }],
      }),
    });
    expect(await screen.findByLabelText('Headers')).toBeInTheDocument();
    expect(screen.queryByLabelText('Priority')).not.toBeInTheDocument();
    await userEvent.selectOptions(screen.getByLabelText('Type'), 'ntfy');
    expect(screen.getByLabelText('Priority')).toBeInTheDocument();
    expect(screen.queryByLabelText('Headers')).not.toBeInTheDocument();
  });
});
