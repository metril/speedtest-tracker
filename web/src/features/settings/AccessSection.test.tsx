import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactNode } from 'react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { SettingsFormProvider } from '@/components/settings';
import * as api from '../../lib/api';
import type { AuthSettings } from '../../lib/api';
import { AccessSection, validateAuthSettings } from './AccessSection';
import { useSettingsSection } from './useSettingsSection';

vi.mock('./useSettingsSection', () => ({ useSettingsSection: vi.fn() }));

const baseAuth: AuthSettings = {
  mode: 'open',
  user_header: 'X-User',
  groups_header: 'X-Groups',
  groups_separator: ',',
  trusted_proxies: [],
  admin_group: '',
  allow_tokens: false,
  oidc_issuer: '',
  oidc_client_id: '',
  oidc_client_secret: '',
  oidc_redirect_base_url: '',
  oidc_scopes: [],
  oidc_groups_claim: '',
  oidc_allowed_groups: [],
  oidc_allowed_emails: [],
  session_ttl_hours: 24,
};

function mockSection(auth: AuthSettings, extra: Partial<Record<string, unknown>> = {}) {
  const setAuth = vi.fn();
  (useSettingsSection as unknown as ReturnType<typeof vi.fn>).mockReturnValue({
    auth, setAuth, readOnly: false, ...extra,
  });
  return setAuth;
}

function jsonResponse(body: unknown, status = 200): Response {
  return { ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body) } as Response;
}

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn().mockImplementation(async () => jsonResponse({ tokens: [] }));
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function renderSection(locked: string[] = []) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrap = (node: ReactNode) => (
    <MemoryRouter>
      <QueryClientProvider client={qc}>
        <SettingsFormProvider locked={locked}>{node}</SettingsFormProvider>
      </QueryClientProvider>
    </MemoryRouter>
  );
  return render(wrap(<AccessSection />));
}

describe('AccessSection', () => {
  it('shows only the Authentication card in open mode', () => {
    mockSection(baseAuth);
    renderSection();
    expect(screen.getByRole('region', { name: 'Authentication' })).toBeInTheDocument();
    expect(screen.queryByRole('region', { name: 'Proxy headers' })).toBeNull();
    expect(screen.queryByRole('region', { name: 'Provider' })).toBeNull();
    expect(screen.queryByRole('region', { name: 'Authorization' })).toBeNull();
    expect(screen.queryByRole('region', { name: 'API tokens' })).toBeNull();
    expect(screen.getByLabelText('Auth mode')).toBeInTheDocument();
  });

  it('shows proxy headers and authorization cards in forward_auth mode', () => {
    mockSection({ ...baseAuth, mode: 'forward_auth' });
    renderSection();
    expect(screen.getByRole('region', { name: 'Proxy headers' })).toBeInTheDocument();
    expect(screen.getByLabelText('User header')).toBeInTheDocument();
    expect(screen.getByLabelText('Groups header')).toBeInTheDocument();
    expect(screen.getByLabelText('Groups separator')).toBeInTheDocument();
    expect(screen.getByText('Trusted proxy CIDRs')).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Authorization' })).toBeInTheDocument();
    expect(screen.getByText('Accept API tokens as well')).toBeInTheDocument();
    expect(screen.queryByRole('region', { name: 'Provider' })).toBeNull();
    expect(screen.getByRole('region', { name: 'API tokens' })).toBeInTheDocument();
  });

  it('flags an invalid CIDR chip', () => {
    mockSection({ ...baseAuth, mode: 'forward_auth', trusted_proxies: ['not-a-cidr'] });
    renderSection();
    expect(screen.getByText('not-a-cidr')).toHaveAttribute('title', expect.stringContaining('does not look like a CIDR'));
  });

  it('shows the Provider card with a working Test OIDC discovery button', async () => {
    mockSection({
      ...baseAuth, mode: 'oidc', oidc_issuer: 'https://issuer.example', oidc_client_id: 'client-1',
    });
    vi.spyOn(api, 'testOIDC').mockResolvedValue({ ok: true, latency_ms: 12 });
    renderSection();
    expect(screen.getByRole('region', { name: 'Provider' })).toBeInTheDocument();
    expect(screen.getByLabelText('Issuer')).toHaveValue('https://issuer.example');
    expect(screen.getByLabelText('Client ID')).toHaveValue('client-1');
    expect(screen.getByLabelText('Client secret')).toHaveAttribute('placeholder', 'leave unchanged');

    await userEvent.click(screen.getByRole('button', { name: 'Test OIDC discovery' }));
    expect(await screen.findByText(/Discovery OK/)).toBeInTheDocument();
    expect(api.testOIDC).toHaveBeenCalledWith({
      issuer: 'https://issuer.example', client_id: 'client-1', client_secret: '',
    });
  });

  it('shows oidc-only authorization fields and hides API tokens in open mode but not oidc', () => {
    mockSection({ ...baseAuth, mode: 'oidc' });
    renderSection();
    expect(screen.getByText('Allowed groups')).toBeInTheDocument();
    expect(screen.getByText('Allowed emails')).toBeInTheDocument();
    expect(screen.getByLabelText('Session TTL (hours)')).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'API tokens' })).toBeInTheDocument();
  });

  it('locks the auth mode field when locked', () => {
    mockSection(baseAuth);
    renderSection(['auth.mode']);
    expect(screen.getByRole('combobox')).toBeDisabled();
  });
});

describe('validateAuthSettings', () => {
  it('requires a trusted proxy CIDR in forward_auth mode', () => {
    expect(validateAuthSettings({ ...baseAuth, mode: 'forward_auth', trusted_proxies: [] })).toEqual({
      field: 'auth-trusted-proxies',
      message: 'forward_auth requires at least one trusted proxy CIDR.',
    });
  });

  it('requires an issuer and client id in oidc mode', () => {
    expect(validateAuthSettings({ ...baseAuth, mode: 'oidc' })).toEqual({
      field: 'auth-oidc-issuer',
      message: 'oidc requires an issuer URL.',
    });
    expect(validateAuthSettings({ ...baseAuth, mode: 'oidc', oidc_issuer: 'https://x' })).toEqual({
      field: 'auth-oidc-client-id',
      message: 'oidc requires a client ID.',
    });
  });

  it('passes for open and token modes', () => {
    expect(validateAuthSettings({ ...baseAuth, mode: 'open' })).toBeUndefined();
    expect(validateAuthSettings({ ...baseAuth, mode: 'token' })).toBeUndefined();
  });
});
