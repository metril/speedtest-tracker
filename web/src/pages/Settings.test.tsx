import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as api from '../lib/api';
import { ApiError } from '../lib/api';
import type { Settings as SettingsType } from '../lib/api';
import { Settings } from './Settings';

function jsonResponse(body: unknown, status = 200): Response {
  return { ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body) } as Response;
}

function settingsFixture(integrationOverrides: Partial<SettingsType['integrations']> = {}): SettingsType {
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

  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrap = (node: ReactNode) => <QueryClientProvider client={qc}>{node}</QueryClientProvider>;
  return render(wrap(<Settings />));
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

  it('reports a connection test result inline', async () => {
    const test = vi.fn().mockResolvedValue({ ok: false, error: 'connection refused' });
    renderSettings({ test });
    await userEvent.click(await screen.findByRole('button', { name: 'Test VictoriaMetrics' }));
    expect(await screen.findByText(/connection refused/)).toBeInTheDocument();
  });
});
