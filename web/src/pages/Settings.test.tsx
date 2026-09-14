import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Navigate, Route, Routes } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from '../lib/api';
import { ApiError } from '../lib/api';
import type { NotifyChannel, Settings as SettingsType } from '../lib/api';
import { Settings } from './Settings';
import { GeneralSection } from '../features/settings/GeneralSection';
import { EnginesSection } from '../features/settings/EnginesSection';
import { IntegrationsSection } from '../features/settings/IntegrationsSection';
import { NotificationsSection } from '../features/settings/NotificationsSection';
import { AuthSettingsSection } from '../features/settings/AuthSettingsSection';

function jsonResponse(body: unknown, status = 200): Response {
  return { ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body) } as Response;
}

type FixtureOverrides = Partial<SettingsType['integrations']> & {
  channels?: NotifyChannel[];
  auth?: Partial<SettingsType['auth']>;
  locked?: string[];
};

function settingsFixture({
  channels, auth: authOverrides, locked, ...integrationOverrides
}: FixtureOverrides = {}): SettingsType {
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
      iperf3_list_url: 'https://export.iperf3serverlist.net/listed_iperf3_servers.json',
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
    auth: {
      mode: 'open',
      user_header: '',
      groups_header: '',
      groups_separator: ',',
      trusted_proxies: [],
      admin_group: '',
      allow_tokens: false,
      ...authOverrides,
    },
    locked: locked ?? [],
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

/** renderSettings mounts the same nested-route tree App.tsx wires up for
 * /settings/*, so each test can deep-link straight to the tab it's
 * exercising (matching how a real reload or bookmark behaves). */
function renderSettings(opts: {
  put?: ReturnType<typeof vi.fn>;
  test?: ReturnType<typeof vi.fn>;
  testChannel?: ReturnType<typeof vi.fn>;
  settings?: SettingsType;
  path?: string;
} = {}) {
  const settings = opts.settings ?? settingsFixture();
  fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.startsWith('/api/v1/settings')) return jsonResponse(settings);
    if (url.startsWith('/api/v1/iperf3/servers')) {
      return jsonResponse({ fetched_at: '2026-09-13T12:00:00.000Z', servers: [], total: 0 });
    }
    throw new Error(`unexpected fetch: ${url}`);
  });
  if (opts.put) vi.spyOn(api, 'updateSettings').mockImplementation(opts.put);
  if (opts.test) vi.spyOn(api, 'testIntegration').mockImplementation(opts.test);
  if (opts.testChannel) vi.spyOn(api, 'testNotifyChannel').mockImplementation(opts.testChannel);

  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return {
    ...render(
      <QueryClientProvider client={qc}>
        <MemoryRouter initialEntries={[opts.path ?? '/settings/general']}>
          <Routes>
            <Route path="/settings" element={<Settings />}>
              <Route index element={<Navigate to="general" replace />} />
              <Route path="general" element={<GeneralSection />} />
              <Route path="engines" element={<EnginesSection />} />
              <Route path="integrations" element={<IntegrationsSection />} />
              <Route path="notifications" element={<NotificationsSection />} />
              <Route path="auth" element={<AuthSettingsSection />} />
            </Route>
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    ),
    qc,
  };
}

describe('Settings page', () => {
  it('redirects /settings to /settings/general', async () => {
    renderSettings({ path: '/settings' });
    expect(await screen.findByLabelText('Timezone')).toBeInTheDocument();
  });

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

  it('saves the SLA plan speeds under "Plan speeds (SLA)"', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({ put });
    await screen.findByLabelText('Timezone');
    await userEvent.type(screen.getByLabelText('Plan download (Mbps)'), '1000');
    await userEvent.type(screen.getByLabelText('Plan upload (Mbps)'), '50');
    await userEvent.click(
      within(screen.getByRole('region', { name: 'General' })).getByRole('button', { name: 'Save General' }),
    );
    expect(put).toHaveBeenCalledWith({
      general: expect.objectContaining({ sla_download_mbps: 1000, sla_upload_mbps: 50 }),
    });
  });

  it('clears the SLA download plan by sending 0', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    const settings = settingsFixture();
    settings.general.sla_download_mbps = 1000;
    renderSettings({ put, settings });
    await screen.findByLabelText('Timezone');
    expect(screen.getByLabelText('Plan download (Mbps)')).toHaveValue(1000);
    // Two "Clear" buttons exist (download, upload); the download field's
    // one is the first in DOM order.
    await userEvent.click(screen.getAllByRole('button', { name: 'Clear' })[0]);
    await userEvent.click(
      within(screen.getByRole('region', { name: 'General' })).getByRole('button', { name: 'Save General' }),
    );
    expect(put).toHaveBeenCalledWith({
      general: expect.objectContaining({ sla_download_mbps: 0 }),
    });
  });

  it('lets the SLA download field be typed empty (not snap back to 0) and sends 0 on save', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    const settings = settingsFixture();
    settings.general.sla_download_mbps = 1000;
    renderSettings({ put, settings });
    const downloadField = await screen.findByLabelText('Plan download (Mbps)');
    expect(downloadField).toHaveValue(1000);

    await userEvent.clear(downloadField);
    expect(downloadField).toHaveValue(null);

    await userEvent.click(
      within(screen.getByRole('region', { name: 'General' })).getByRole('button', { name: 'Save General' }),
    );
    expect(put).toHaveBeenCalledWith({
      general: expect.objectContaining({ sla_download_mbps: 0 }),
    });
  });

  it('saves an edited iperf3 server list URL, including clearing it to disable', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({ put, path: '/settings/engines' });
    const urlField = await screen.findByLabelText('iperf3 server list URL');
    await userEvent.clear(urlField);
    await userEvent.click(
      within(screen.getByRole('region', { name: 'Engines' })).getByRole('button', { name: 'Save Engines' }),
    );
    expect(put).toHaveBeenCalledWith({ engines: expect.objectContaining({ iperf3_list_url: '' }) });
  });

  it('keeps a stored secret when the field is left untouched', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({ put, settings: settingsFixture({ vm_auth_header: '***' }), path: '/settings/integrations' });
    await userEvent.click(await screen.findByRole('button', { name: 'Save Integrations' }));
    expect(put.mock.calls[0][0].integrations.vm_auth_header).toBe('***');
  });

  it('shows the server validation message inline', async () => {
    const put = vi.fn().mockRejectedValue(new ApiError(400, 'invalid_request', 'vm_url must be http or https'));
    renderSettings({ put, path: '/settings/integrations' });
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
    renderSettings({ test, path: '/settings/integrations' });
    await userEvent.click(await screen.findByRole('button', { name: 'Test VictoriaMetrics' }));
    expect(await screen.findByText(/connection refused/)).toBeInTheDocument();
  });

  it('saves only the notifications section', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({ put, path: '/settings/notifications' });
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
      path: '/settings/notifications',
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
      path: '/settings/notifications',
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
      path: '/settings/notifications',
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
      path: '/settings/notifications',
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
      path: '/settings/notifications',
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

  it('saves only the auth section', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({ put, path: '/settings/auth' });
    await userEvent.selectOptions(await screen.findByLabelText('Auth mode'), 'forward_auth');
    await userEvent.type(screen.getByLabelText('Trusted proxy CIDRs'), '10.0.0.0/8');
    await userEvent.click(within(screen.getByRole('region', { name: 'Auth' }))
      .getByRole('button', { name: 'Save Auth' }));
    expect(put).toHaveBeenCalledWith({ auth: expect.objectContaining({
      mode: 'forward_auth', trusted_proxies: ['10.0.0.0/8'],
    }) });
    expect(put.mock.calls[0][0].general).toBeUndefined();
  });

  it('drops a trailing newline and blank lines from trusted proxy CIDRs before saving', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({ put, path: '/settings/auth' });
    await userEvent.selectOptions(await screen.findByLabelText('Auth mode'), 'forward_auth');
    const textarea = screen.getByLabelText('Trusted proxy CIDRs');
    await userEvent.type(textarea, '10.0.0.0/8{enter}{enter}192.168.0.0/16{enter}');
    await userEvent.click(within(screen.getByRole('region', { name: 'Auth' }))
      .getByRole('button', { name: 'Save Auth' }));
    expect(put).toHaveBeenCalledWith({ auth: expect.objectContaining({
      mode: 'forward_auth', trusted_proxies: ['10.0.0.0/8', '192.168.0.0/16'],
    }) });
  });

  it('disables a field that is set by the environment', async () => {
    renderSettings({ settings: settingsFixture({ locked: ['auth.mode'] }), path: '/settings/auth' });
    expect(await screen.findByLabelText('Auth mode')).toBeDisabled();
    expect(within(screen.getByRole('region', { name: 'Auth' }))
      .getByText('set by environment')).toBeInTheDocument();
    expect(screen.getByLabelText('Admin group')).not.toBeDisabled();
  });

  it('blocks a forward_auth switch with no trusted proxies before calling the API', async () => {
    const put = vi.fn();
    renderSettings({ put, path: '/settings/auth' });
    await userEvent.selectOptions(await screen.findByLabelText('Auth mode'), 'forward_auth');
    await userEvent.click(screen.getByRole('button', { name: 'Save Auth' }));
    expect(put).not.toHaveBeenCalled();
    expect(screen.getByRole('alert')).toHaveTextContent(/at least one trusted proxy/i);
  });

  it('shows the server lockout-guard message', async () => {
    const put = vi.fn().mockRejectedValue(
      new ApiError(400, 'invalid_request', 'this request does not carry the Remote-User header from a trusted proxy'),
    );
    renderSettings({
      put,
      path: '/settings/auth',
      settings: settingsFixture({ auth: { mode: 'open', trusted_proxies: ['10.0.0.0/8'] } }),
    });
    await userEvent.selectOptions(await screen.findByLabelText('Auth mode'), 'forward_auth');
    await userEvent.click(screen.getByRole('button', { name: 'Save Auth' }));
    expect(await screen.findByRole('alert')).toHaveTextContent(/Remote-User/);
  });

  it('switching tabs keeps unsaved edits in the tab left behind', async () => {
    renderSettings({ path: '/settings/general' });
    const tz = await screen.findByLabelText('Timezone');
    await userEvent.clear(tz);
    await userEvent.type(tz, 'Asia/Kolkata');

    // Both the md+ column nav and the mobile horizontal-scroll strip render
    // a same-named link (only one is visible at a time via CSS, which
    // jsdom doesn't apply) — either navigates to the same route.
    await userEvent.click(screen.getAllByRole('link', { name: 'Engines' })[0]);
    await screen.findByLabelText('iperf3 server list URL');

    await userEvent.click(screen.getAllByRole('link', { name: 'General' })[0]);
    expect(await screen.findByLabelText('Timezone')).toHaveValue('Asia/Kolkata');
  });
});
