import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Navigate, Route, Routes, useNavigate } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from '../lib/api';
import { ApiError } from '../lib/api';
import type { NotifyChannel, Settings as SettingsType } from '../lib/api';
import { Settings } from './Settings';
import { GeneralSection } from '../features/settings/GeneralSection';
import { EnginesSection } from '../features/settings/EnginesSection';
import { ExportersSection } from '../features/settings/ExportersSection';
import { NotificationsSection } from '../features/settings/NotificationsSection';
import { AccessSection } from '../features/settings/AccessSection';
import { NavigationGuardProvider, useNavigationGuard } from '../features/settings/NavigationGuardContext';

/** DashboardLink stands in for Layout's sidebar NavLink: it routes every
 * click through the registered NavigationGuardContext guard, same as
 * the real sidebar does, without pulling in the whole Layout shell. */
function DashboardLink() {
  const { guard } = useNavigationGuard();
  const navigate = useNavigate();
  return (
    <button type="button" onClick={() => guard(() => navigate('/'))}>
      Dashboard
    </button>
  );
}

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
      vm_auth_type: 'none',
      vm_auth_username: '',
      vm_auth_password: '',
      vm_auth_token: '',
      vm_auth_header_name: '',
      vm_auth_header_value: '',
      vm_extra_labels: { host: 'pi4' },
      vl_enabled: true,
      vl_url: 'http://vl:9428',
      vl_auth_header: '***',
      vl_auth_type: 'none',
      vl_auth_username: '',
      vl_auth_password: '',
      vl_auth_token: '',
      vl_auth_header_name: '',
      vl_auth_header_value: '',
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
      oidc_issuer: '',
      oidc_client_id: '',
      oidc_client_secret: '',
      oidc_redirect_base_url: '',
      oidc_scopes: [],
      oidc_groups_claim: 'groups',
      oidc_allowed_groups: [],
      oidc_allowed_emails: [],
      session_ttl_hours: 24,
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

/** saveBar/saveBarButton/discardButton locate the sticky "Unsaved
 * changes" region and its actions -- absent entirely while a section is
 * clean, per SettingsSaveBar. */
function saveBar() {
  return screen.queryByRole('region', { name: 'Unsaved changes' });
}
function saveButton() {
  return within(saveBar()!).getByRole('button', { name: 'Save changes' });
}
function discardButton() {
  return within(saveBar()!).getByRole('button', { name: 'Discard' });
}

/** renderSettings mounts the same nested-route tree App.tsx wires up for
 * /settings/*, so each test can deep-link straight to the tab it's
 * exercising (matching how a real reload or bookmark behaves). */
function renderSettings(opts: {
  put?: ReturnType<typeof vi.fn>;
  test?: ReturnType<typeof vi.fn>;
  testChannel?: ReturnType<typeof vi.fn>;
  settings?: SettingsType;
  path?: string;
  me?: { mode: string; user: string; groups: string[]; is_admin: boolean };
} = {}) {
  const settings = opts.settings ?? settingsFixture();
  fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.startsWith('/api/v1/settings')) return jsonResponse(settings);
    if (url.startsWith('/api/v1/iperf3/servers')) {
      return jsonResponse({ fetched_at: '2026-09-13T12:00:00.000Z', servers: [], total: 0 });
    }
    if (url.startsWith('/api/v1/me')) {
      if (opts.me) return jsonResponse(opts.me);
      throw new Error(`unexpected fetch: ${url}`);
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
              <Route path="exporters" element={<ExportersSection />} />
              <Route path="integrations" element={<Navigate to="/settings/exporters" replace />} />
              <Route path="notifications" element={<NotificationsSection />} />
              <Route path="access" element={<AccessSection />} />
              <Route path="auth" element={<Navigate to="/settings/access" replace />} />
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

  it('redirects the old /settings/integrations path to /settings/exporters', async () => {
    renderSettings({ path: '/settings/integrations' });
    expect(await screen.findByLabelText('VictoriaMetrics URL')).toBeInTheDocument();
  });

  it('redirects the old /settings/auth path to /settings/access', async () => {
    renderSettings({ path: '/settings/auth' });
    expect(await screen.findByLabelText('Auth mode')).toBeInTheDocument();
  });

  it('renders the section tabs and marks the active one', async () => {
    renderSettings({ path: '/settings/engines' });
    await screen.findByLabelText('iperf3 server list URL');
    const tabs = screen.getAllByRole('tab');
    expect(tabs.map((t) => t.textContent)).toEqual(['General', 'Engines', 'Exporters', 'Notifications', 'Access']);
    expect(screen.getByRole('tab', { name: 'Engines' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('tab', { name: 'General' })).toHaveAttribute('aria-selected', 'false');
  });

  it('shows no save bar while the section is clean', async () => {
    renderSettings();
    await screen.findByLabelText('Timezone');
    expect(saveBar()).not.toBeInTheDocument();
  });

  it('shows the save bar after an edit and hides it again after saving', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({ put });
    await screen.findByLabelText('Timezone');
    await userEvent.clear(screen.getByLabelText('Results retention (days)'));
    await userEvent.type(screen.getByLabelText('Results retention (days)'), '30');
    expect(saveBar()).toBeInTheDocument();
    await userEvent.click(saveButton());
    expect(put).toHaveBeenCalledWith({ general: expect.objectContaining({ retention_days_results: 30 }) });
    expect(await screen.findByText('Saved')).toBeInTheDocument();
    expect(saveBar()).not.toBeInTheDocument();
  });

  it('hides the save bar right after saving even if the settings refetch never resolves', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({ put });
    await screen.findByLabelText('Timezone');

    // From here on, a GET to /settings (the invalidateQueries refetch
    // triggered by the save) hangs forever -- only the mutation response
    // itself should be able to clear the save bar.
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.startsWith('/api/v1/settings')) return new Promise(() => {});
      throw new Error(`unexpected fetch: ${url}`);
    });

    await userEvent.clear(screen.getByLabelText('Results retention (days)'));
    await userEvent.type(screen.getByLabelText('Results retention (days)'), '30');
    expect(saveBar()).toBeInTheDocument();
    await userEvent.click(saveButton());

    expect(await screen.findByText('Saved')).toBeInTheDocument();
    expect(saveBar()).not.toBeInTheDocument();
  });

  it('hides the save bar after Discard, restoring the server value', async () => {
    renderSettings();
    const tz = await screen.findByLabelText('Timezone');
    await userEvent.selectOptions(tz, 'Asia/Kolkata');
    expect(saveBar()).toBeInTheDocument();
    await userEvent.click(discardButton());
    expect(saveBar()).not.toBeInTheDocument();
    expect(screen.getByLabelText('Timezone')).toHaveValue('UTC');
  });

  it('saves only the edited section', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({ put });
    await screen.findByLabelText('Timezone');
    await userEvent.clear(screen.getByLabelText('Results retention (days)'));
    await userEvent.type(screen.getByLabelText('Results retention (days)'), '30');
    await userEvent.click(saveButton());
    expect(put).toHaveBeenCalledWith({ general: expect.objectContaining({ retention_days_results: 30 }) });
    expect(put.mock.calls[0][0].integrations).toBeUndefined();
  });

  it('saves the SLA plan speeds under "Plan speeds (SLA)"', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({ put });
    await screen.findByLabelText('Timezone');
    await userEvent.type(screen.getByLabelText('Plan download (Mbps)'), '1000');
    await userEvent.type(screen.getByLabelText('Plan upload (Mbps)'), '50');
    await userEvent.click(saveButton());
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
    await userEvent.click(saveButton());
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

    await userEvent.click(saveButton());
    expect(put).toHaveBeenCalledWith({
      general: expect.objectContaining({ sla_download_mbps: 0 }),
    });
  });

  it('saves an edited iperf3 server list URL, including clearing it to disable', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({ put, path: '/settings/engines' });
    const urlField = await screen.findByLabelText('iperf3 server list URL');
    await userEvent.clear(urlField);
    await userEvent.click(saveButton());
    expect(put).toHaveBeenCalledWith({ engines: expect.objectContaining({ iperf3_list_url: '' }) });
  });

  it('keeps a stored secret when the field is left untouched', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({ put, settings: settingsFixture({ vm_auth_header: '***' }), path: '/settings/exporters' });
    await screen.findByLabelText('VictoriaMetrics URL');
    // Untouched, masked secrets never register as a change: no save bar.
    expect(saveBar()).not.toBeInTheDocument();
    await userEvent.type(screen.getByLabelText('VictoriaMetrics URL'), '/extra');
    await userEvent.click(saveButton());
    expect(put.mock.calls[0][0].integrations.vm_auth_header).toBe('***');
  });

  it('shows the server validation message inline', async () => {
    const put = vi.fn().mockRejectedValue(new ApiError(400, 'invalid_request', 'vm_url must be http or https'));
    renderSettings({ put, path: '/settings/exporters' });
    const urlField = await screen.findByLabelText('VictoriaMetrics URL');
    await userEvent.type(urlField, '/x');
    await userEvent.click(saveButton());
    expect(await screen.findByRole('alert')).toHaveTextContent('vm_url must be http or https');
  });

  it('does not clobber an edited but unsaved field on refetch', async () => {
    const { qc } = renderSettings();
    const tz = await screen.findByLabelText('Timezone');
    await userEvent.selectOptions(tz, 'Asia/Kolkata');

    // A refetch (e.g. invalidation, background refresh) brings back the
    // original server data; the in-progress, unsaved edit must survive it.
    await qc.refetchQueries({ queryKey: ['settings'] });

    expect(screen.getByLabelText('Timezone')).toHaveValue('Asia/Kolkata');
  });

  it('reports a connection test result inline', async () => {
    const test = vi.fn().mockResolvedValue({ ok: false, error: 'connection refused' });
    renderSettings({ test, path: '/settings/exporters' });
    await userEvent.click(await screen.findByRole('button', { name: 'Test VictoriaMetrics' }));
    expect(await screen.findByText(/connection refused/)).toBeInTheDocument();
  });

  it('sends a structured auth payload with the connection test', async () => {
    const test = vi.fn().mockResolvedValue({ ok: true, latency_ms: 5 });
    renderSettings({ test, path: '/settings/exporters' });
    await userEvent.selectOptions(await screen.findByLabelText('VictoriaMetrics auth'), 'bearer');
    await userEvent.type(screen.getByLabelText('VictoriaMetrics auth token'), 'tok123');
    await userEvent.click(screen.getByRole('button', { name: 'Test VictoriaMetrics' }));
    expect(test).toHaveBeenCalledWith('vm', expect.objectContaining({
      url: 'http://vm:8428',
      auth: expect.objectContaining({ type: 'bearer', token: 'tok123' }),
    }));
  });

  it('saves only the notifications section', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({ put, path: '/settings/notifications' });
    await screen.findByLabelText('Cooldown (minutes)');
    await userEvent.clear(screen.getByLabelText('Cooldown (minutes)'));
    await userEvent.type(screen.getByLabelText('Cooldown (minutes)'), '15');
    await userEvent.click(saveButton());
    expect(put).toHaveBeenCalledWith({ notifications: expect.objectContaining({ cooldown_minutes: 15 }) });
    expect(put.mock.calls[0][0].general).toBeUndefined();
  });

  it('adds a channel and echoes untouched apprise urls back masked', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({
      put,
      path: '/settings/notifications',
      settings: settingsFixture({
        channels: [{
          id: 'c1', type: 'apprise', name: 'phone', enabled: true, url: '', urls: ['***'],
        }],
      }),
    });
    await userEvent.click(await screen.findByRole('button', { name: 'Add channel' }));
    await userEvent.click(saveButton());
    const sent = put.mock.calls[0][0].notifications.channels;
    expect(sent).toHaveLength(2);
    expect(sent[0].urls).toEqual(['***']);
  });

  it('reports a channel test result inline', async () => {
    const testChannel = vi.fn().mockResolvedValue({ ok: false, error: 'connection refused' });
    renderSettings({
      testChannel,
      path: '/settings/notifications',
      settings: settingsFixture({
        channels: [{
          id: 'c1', type: 'apprise', name: 'phone', enabled: true, url: '', urls: ['ntfy://host/topic'],
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
          id: 'c1', type: 'apprise', name: 'phone', enabled: true, url: '', urls: ['ntfy://host/topic'],
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
            id: 'c1', type: 'apprise', name: 'phone', enabled: true, url: '', urls: ['ntfy://host/x'],
          },
          {
            id: 'c2', type: 'apprise', name: 'laptop', enabled: true, url: '', urls: ['ntfy://host/y'],
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
    expect(screen.queryByLabelText('Apprise URLs')).not.toBeInTheDocument();
    await userEvent.selectOptions(screen.getByLabelText('Type'), 'apprise');
    expect(screen.getByLabelText('Apprise URLs')).toBeInTheDocument();
    expect(screen.queryByLabelText('Headers')).not.toBeInTheDocument();
  });

  it('saves only the access section', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({ put, path: '/settings/access' });
    await userEvent.selectOptions(await screen.findByLabelText('Auth mode'), 'forward_auth');
    await userEvent.type(screen.getByLabelText('Trusted proxy CIDRs'), '10.0.0.0/8');
    await userEvent.click(saveButton());
    expect(put).toHaveBeenCalledWith({ auth: expect.objectContaining({
      mode: 'forward_auth', trusted_proxies: ['10.0.0.0/8'],
    }) });
    expect(put.mock.calls[0][0].general).toBeUndefined();
  });

  it('drops a trailing newline and blank lines from trusted proxy CIDRs before saving', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({ put, path: '/settings/access' });
    await userEvent.selectOptions(await screen.findByLabelText('Auth mode'), 'forward_auth');
    const textarea = screen.getByLabelText('Trusted proxy CIDRs');
    await userEvent.type(textarea, '10.0.0.0/8{enter}{enter}192.168.0.0/16{enter}');
    await userEvent.click(saveButton());
    expect(put).toHaveBeenCalledWith({ auth: expect.objectContaining({
      mode: 'forward_auth', trusted_proxies: ['10.0.0.0/8', '192.168.0.0/16'],
    }) });
  });

  it('disables a field that is set by the environment', async () => {
    renderSettings({
      settings: settingsFixture({
        locked: ['auth.mode'], auth: { mode: 'forward_auth', trusted_proxies: ['10.0.0.0/8'] },
      }),
      path: '/settings/access',
    });
    expect(await screen.findByLabelText('Auth mode', { exact: false })).toBeDisabled();
    expect(screen.getByText('set by environment')).toBeInTheDocument();
    expect(screen.getByLabelText('Admin group')).not.toBeDisabled();
  });

  it('blocks a forward_auth switch with no trusted proxies before calling the API', async () => {
    const put = vi.fn();
    renderSettings({ put, path: '/settings/access' });
    await userEvent.selectOptions(await screen.findByLabelText('Auth mode'), 'forward_auth');
    await userEvent.click(saveButton());
    expect(put).not.toHaveBeenCalled();
    expect(screen.getByRole('alert')).toHaveTextContent(/at least one trusted proxy/i);
  });

  it('shows the forward_auth validation error on the trusted-proxies field too', async () => {
    renderSettings({ path: '/settings/access' });
    await userEvent.selectOptions(await screen.findByLabelText('Auth mode', { exact: false }), 'forward_auth');
    await userEvent.click(saveButton());
    // Once in the save bar, once inline under the offending row.
    expect(screen.getAllByText(/at least one trusted proxy/i)).toHaveLength(2);
  });

  it('saves the oidc mode settings', async () => {
    const put = vi.fn().mockResolvedValue(settingsFixture());
    renderSettings({ put, path: '/settings/access' });
    await userEvent.selectOptions(await screen.findByLabelText('Auth mode'), 'oidc');
    await userEvent.type(screen.getByLabelText('Issuer'), 'https://idp.example.com');
    await userEvent.type(screen.getByLabelText('Client ID'), 'client-1');
    await userEvent.click(saveButton());
    expect(put).toHaveBeenCalledWith({ auth: expect.objectContaining({
      mode: 'oidc', oidc_issuer: 'https://idp.example.com', oidc_client_id: 'client-1',
    }) });
  });

  it('blocks saving oidc mode with no issuer or client id', async () => {
    const put = vi.fn();
    renderSettings({ put, path: '/settings/access' });
    await userEvent.selectOptions(await screen.findByLabelText('Auth mode'), 'oidc');
    await userEvent.click(saveButton());
    expect(put).not.toHaveBeenCalled();
    expect(screen.getByRole('alert')).toHaveTextContent(/issuer/i);
  });

  it('tests OIDC discovery with the in-progress form values', async () => {
    const testOIDC = vi.fn().mockResolvedValue({ ok: true, latency_ms: 42 });
    vi.spyOn(api, 'testOIDC').mockImplementation(testOIDC);
    renderSettings({ path: '/settings/access' });
    await userEvent.selectOptions(await screen.findByLabelText('Auth mode'), 'oidc');
    await userEvent.type(screen.getByLabelText('Issuer'), 'https://idp.example.com');
    await userEvent.type(screen.getByLabelText('Client ID'), 'client-1');
    await userEvent.click(screen.getByRole('button', { name: 'Test OIDC discovery' }));
    expect(testOIDC).toHaveBeenCalledWith({
      issuer: 'https://idp.example.com', client_id: 'client-1', client_secret: '',
    });
    expect(await screen.findByText(/Discovery OK/)).toBeInTheDocument();
  });

  it('shows the server lockout-guard message', async () => {
    const put = vi.fn().mockRejectedValue(
      new ApiError(400, 'invalid_request', 'this request does not carry the Remote-User header from a trusted proxy'),
    );
    renderSettings({
      put,
      path: '/settings/access',
      settings: settingsFixture({ auth: { mode: 'open', trusted_proxies: ['10.0.0.0/8'] } }),
    });
    await userEvent.selectOptions(await screen.findByLabelText('Auth mode'), 'forward_auth');
    await userEvent.click(saveButton());
    expect(await screen.findByRole('alert')).toHaveTextContent(/Remote-User/);
  });

  it('disables every field and Test with a read-only hint for a non-admin viewer', async () => {
    renderSettings({
      path: '/settings/exporters',
      me: { mode: 'oidc', user: 'bob', groups: [], is_admin: false },
    });
    expect(await screen.findByLabelText('VictoriaMetrics URL')).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Test VictoriaMetrics' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Test VictoriaLogs' })).toBeDisabled();
    expect(screen.getAllByText('Read-only: admin group required').length).toBeGreaterThan(0);
    // A read-only viewer can't dirty any field, so no save bar ever appears.
    expect(saveBar()).not.toBeInTheDocument();
  });

  it('leaves every field and Test enabled for an admin viewer', async () => {
    renderSettings({
      path: '/settings/exporters',
      me: { mode: 'oidc', user: 'alice', groups: ['admin'], is_admin: true },
    });
    expect(await screen.findByLabelText('VictoriaMetrics URL')).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Test VictoriaMetrics' })).toBeEnabled();
  });

  it('switching tabs keeps unsaved edits in the tab left behind', async () => {
    renderSettings({ path: '/settings/general' });
    const tz = await screen.findByLabelText('Timezone');
    await userEvent.selectOptions(tz, 'Asia/Kolkata');

    // Leaving a dirty tab opens the confirm dialog; Keep editing stays put.
    await userEvent.click(screen.getByRole('tab', { name: 'Engines' }));
    const dialog = await screen.findByRole('dialog', { name: 'Discard unsaved changes?' });
    expect(dialog).toHaveTextContent('Your edits to General have not been saved.');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Keep editing' }));
    expect(screen.getByLabelText('Timezone')).toHaveValue('Asia/Kolkata');

    // Discard clears the edit and completes the navigation.
    await userEvent.click(screen.getByRole('tab', { name: 'Engines' }));
    const dialog2 = await screen.findByRole('dialog', { name: 'Discard unsaved changes?' });
    await userEvent.click(within(dialog2).getByRole('button', { name: 'Discard' }));
    await screen.findByLabelText('iperf3 server list URL');

    await userEvent.click(screen.getByRole('tab', { name: 'General' }));
    expect(await screen.findByLabelText('Timezone')).toHaveValue('UTC');
  });

  it('guards sidebar navigation by any dirty section, with "leave Settings" copy, and completes the navigation on discard', async () => {
    const settings = settingsFixture();
    fetchMock = vi.fn().mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.startsWith('/api/v1/settings')) return jsonResponse(settings);
      if (url.startsWith('/api/v1/iperf3/servers')) {
        return jsonResponse({ fetched_at: '2026-09-13T12:00:00.000Z', servers: [], total: 0 });
      }
      throw new Error(`unexpected fetch: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={qc}>
        <MemoryRouter initialEntries={['/settings/general']}>
          <NavigationGuardProvider>
            <DashboardLink />
            <Routes>
              <Route path="/" element={<p>Dashboard page</p>} />
              <Route path="/settings" element={<Settings />}>
                <Route index element={<Navigate to="general" replace />} />
                <Route path="general" element={<GeneralSection />} />
                <Route path="engines" element={<EnginesSection />} />
              </Route>
            </Routes>
          </NavigationGuardProvider>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    const tz = await screen.findByLabelText('Timezone');
    await userEvent.selectOptions(tz, 'Asia/Kolkata');

    // Switching tabs while General is dirty opens the tab-switch dialog;
    // Keep editing leaves the draft (and the dirty state) in place.
    await userEvent.click(screen.getByRole('tab', { name: 'Engines' }));
    const tabDialog = await screen.findByRole('dialog', { name: 'Discard unsaved changes?' });
    await userEvent.click(within(tabDialog).getByRole('button', { name: 'Keep editing' }));
    expect(screen.getByLabelText('Timezone')).toHaveValue('Asia/Kolkata');

    // The sidebar link is guarded by *any* dirty section (General, even
    // though it's not the active tab check the tab dialog uses), with
    // its own "leave Settings" copy.
    await userEvent.click(screen.getByRole('button', { name: 'Dashboard' }));
    const leaveDialog = await screen.findByRole('dialog', { name: 'Leave Settings?' });
    expect(leaveDialog).toHaveTextContent('Your unsaved settings changes will be lost.');

    await userEvent.click(within(leaveDialog).getByRole('button', { name: 'Discard' }));
    expect(await screen.findByText('Dashboard page')).toBeInTheDocument();
  });
});
