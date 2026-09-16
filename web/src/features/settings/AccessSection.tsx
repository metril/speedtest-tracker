import { useState } from 'react';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { ListInput, SettingsCard, SettingsRow, TestButton, useSettingsRowField } from '@/components/settings';
import { testOIDC } from '../../lib/api';
import type { AuthMode, AuthSettings, OidcDisplayClaim } from '../../lib/api';
import { inputClass } from './styles';
import { TokenPanel } from './TokenPanel';
import { useSettingsSection } from './useSettingsSection';

const MODE_HELP: Record<AuthMode, string> = {
  open: 'No authentication. Anyone who can reach this instance can change settings and run tests.',
  forward_auth: 'Trusts identity headers set by a reverse proxy. Requires the trusted proxy CIDRs below.',
  token: 'Requires an API token in the Authorization header for every request.',
  oidc: 'Users sign in through your OpenID Connect provider; admin decided by the admin group.',
};

/** A CIDR line must look like "<ip>/<bits>" -- no spaces, one slash. This
 * is a client-side sanity check; the server remains the authority. */
const CIDR_RE = /^\S+\/\d{1,3}$/;

function cidrError(s: string): string | undefined {
  return CIDR_RE.test(s) ? undefined : `"${s}" does not look like a CIDR (e.g. 10.0.0.0/8).`;
}

/** validateAuthSettings runs before save('auth', ...): forward_auth is
 * useless (and dangerous -- it would trust nobody, or everybody) without
 * at least one well-formed trusted proxy CIDR. */
export function validateAuthSettings(auth: AuthSettings): { field: string; message: string } | undefined {
  if (auth.mode === 'oidc') {
    if (!auth.oidc_issuer.trim()) return { field: 'auth-oidc-issuer', message: 'oidc requires an issuer URL.' };
    if (!auth.oidc_client_id.trim()) return { field: 'auth-oidc-client-id', message: 'oidc requires a client ID.' };
    return undefined;
  }
  if (auth.mode !== 'forward_auth') return undefined;
  const lines = auth.trusted_proxies.map((l) => l.trim()).filter(Boolean);
  if (lines.length === 0) {
    return { field: 'auth-trusted-proxies', message: 'forward_auth requires at least one trusted proxy CIDR.' };
  }
  const bad = lines.find((l) => !CIDR_RE.test(l));
  if (bad) return { field: 'auth-trusted-proxies', message: `"${bad}" does not look like a CIDR (e.g. 10.0.0.0/8).` };
  return undefined;
}

function ModeField({ value, onChange }: { value: AuthMode; onChange: (v: AuthMode) => void }) {
  const props = useSettingsRowField();
  return (
    <select {...props} className={inputClass} value={value} onChange={(e) => onChange(e.target.value as AuthMode)}>
      <option value="open">open</option>
      <option value="forward_auth">forward_auth</option>
      <option value="token">token</option>
      <option value="oidc">oidc</option>
    </select>
  );
}

function DisplayClaimField({ value, onChange }: { value: OidcDisplayClaim; onChange: (v: OidcDisplayClaim) => void }) {
  const props = useSettingsRowField();
  return (
    <select
      {...props} className={inputClass} value={value}
      onChange={(e) => onChange(e.target.value as OidcDisplayClaim)}
    >
      <option value="name">name</option>
      <option value="preferred_username">preferred_username</option>
      <option value="email">email</option>
    </select>
  );
}

function TextField({
  value, onChange, type, placeholder,
}: { value: string; onChange: (v: string) => void; type?: string; placeholder?: string }) {
  const props = useSettingsRowField();
  return (
    <Input {...props} type={type} placeholder={placeholder} value={value} onChange={(e) => onChange(e.target.value)} />
  );
}

function NumberField({ value, onChange }: { value: number; onChange: (v: number) => void }) {
  const props = useSettingsRowField();
  return <Input {...props} type="number" value={value} onChange={(e) => onChange(Number(e.target.value))} />;
}

function ListField({
  value, onChange, label, placeholder, validate,
}: {
  value: string[]; onChange: (v: string[]) => void; label: string; placeholder?: string;
  validate?: (s: string) => string | undefined;
}) {
  const props = useSettingsRowField();
  return (
    <ListInput
      id={props.id} label={label} value={value} onChange={onChange}
      placeholder={placeholder} validate={validate} disabled={props.disabled}
      aria-invalid={props['aria-invalid']} aria-describedby={props['aria-describedby']}
    />
  );
}

function SwitchRowField({ checked, onChange }: { checked: boolean; onChange: (v: boolean) => void }) {
  const props = useSettingsRowField();
  return <Switch id={props.id} checked={checked} onCheckedChange={onChange} disabled={props.disabled} />;
}

/** AccessSection is the redesigned Auth tab: authentication mode, proxy
 * headers, OIDC provider config, authorization rules and API tokens as
 * SettingsCards instead of one long field list. */
export function AccessSection() {
  const { auth, setAuth, readOnly, fieldError } = useSettingsSection('access');
  const rowError = (id: string) => (fieldError?.field === id ? fieldError.message : undefined);
  const [testing, setTesting] = useState(false);
  const [result, setResult] = useState<{ ok: boolean; message: string } | null>(null);

  const runTest = async () => {
    setTesting(true);
    setResult(null);
    try {
      const res = await testOIDC({
        issuer: auth.oidc_issuer,
        client_id: auth.oidc_client_id,
        client_secret: auth.oidc_client_secret,
      });
      setResult({
        ok: res.ok,
        message: res.ok ? `Discovery OK (${res.latency_ms ?? 0} ms)` : (res.error ?? 'Discovery failed.'),
      });
    } catch (err) {
      setResult({ ok: false, message: err instanceof Error ? err.message : String(err) });
    } finally {
      setTesting(false);
    }
  };

  return (
    <div className="grid gap-4">
      <SettingsCard title="Authentication">
        <SettingsRow
          label="Auth mode" htmlFor="auth-mode" lockKey="auth.mode" size="sm"
          description={MODE_HELP[auth.mode]}
        >
          <ModeField value={auth.mode} onChange={(v) => setAuth({ ...auth, mode: v })} />
        </SettingsRow>
      </SettingsCard>

      {auth.mode === 'forward_auth' && (
        <SettingsCard title="Proxy headers">
          <SettingsRow label="User header" htmlFor="auth-user-header" lockKey="auth.user_header">
            <TextField value={auth.user_header} onChange={(v) => setAuth({ ...auth, user_header: v })} />
          </SettingsRow>
          <SettingsRow label="Groups header" htmlFor="auth-groups-header" lockKey="auth.groups_header">
            <TextField value={auth.groups_header} onChange={(v) => setAuth({ ...auth, groups_header: v })} />
          </SettingsRow>
          <SettingsRow
            label="Groups separator" htmlFor="auth-groups-separator" lockKey="auth.groups_separator" size="sm"
          >
            <TextField value={auth.groups_separator} onChange={(v) => setAuth({ ...auth, groups_separator: v })} />
          </SettingsRow>
          <SettingsRow
            label="Trusted proxy CIDRs" htmlFor="auth-trusted-proxies" lockKey="auth.trusted_proxies"
            description="Required. Identity headers are ignored unless the connecting peer is inside one of these ranges."
            error={rowError('auth-trusted-proxies')}
          >
            <ListField
              value={auth.trusted_proxies} onChange={(v) => setAuth({ ...auth, trusted_proxies: v })}
              label="Trusted proxy CIDRs" placeholder="10.0.0.0/8" validate={cidrError}
            />
          </SettingsRow>
        </SettingsCard>
      )}

      {auth.mode === 'oidc' && (
        <SettingsCard
          title="Provider"
          footer={(
            <TestButton
              label="Test OIDC discovery" onTest={runTest} pending={testing} result={result}
              disabled={readOnly} disabledReason={readOnly ? 'Read-only: admin group required' : undefined}
            />
          )}
        >
          <SettingsRow
            label="Issuer" htmlFor="auth-oidc-issuer" lockKey="auth.oidc_issuer"
            error={rowError('auth-oidc-issuer')}
          >
            <TextField value={auth.oidc_issuer} onChange={(v) => setAuth({ ...auth, oidc_issuer: v })} />
          </SettingsRow>
          <SettingsRow
            label="Client ID" htmlFor="auth-oidc-client-id" lockKey="auth.oidc_client_id"
            error={rowError('auth-oidc-client-id')}
          >
            <TextField value={auth.oidc_client_id} onChange={(v) => setAuth({ ...auth, oidc_client_id: v })} />
          </SettingsRow>
          <SettingsRow label="Client secret" htmlFor="auth-oidc-client-secret" lockKey="auth.oidc_client_secret">
            <TextField
              value={auth.oidc_client_secret} type="password" placeholder="leave unchanged"
              onChange={(v) => setAuth({ ...auth, oidc_client_secret: v })}
            />
          </SettingsRow>
          <SettingsRow
            label="Redirect base URL" htmlFor="auth-oidc-redirect-base-url" lockKey="auth.oidc_redirect_base_url"
            description="Leave empty to derive from the request. The callback path is /auth/oidc/callback."
          >
            <TextField
              value={auth.oidc_redirect_base_url}
              onChange={(v) => setAuth({ ...auth, oidc_redirect_base_url: v })}
            />
          </SettingsRow>
          <SettingsRow label="Groups claim" htmlFor="auth-oidc-groups-claim" lockKey="auth.oidc_groups_claim" size="sm">
            <TextField
              value={auth.oidc_groups_claim} placeholder="groups"
              onChange={(v) => setAuth({ ...auth, oidc_groups_claim: v })}
            />
          </SettingsRow>
          <SettingsRow
            label="Display name claim" htmlFor="auth-oidc-display-claim" lockKey="auth.oidc_display_claim" size="sm"
            description="Which claim the sidebar shows for the signed-in user."
          >
            <DisplayClaimField
              value={auth.oidc_display_claim}
              onChange={(v) => setAuth({ ...auth, oidc_display_claim: v })}
            />
          </SettingsRow>
          <SettingsRow
            label="Extra scopes" htmlFor="auth-oidc-scopes" lockKey="auth.oidc_scopes"
            description="Beyond openid profile email."
          >
            <ListField
              value={auth.oidc_scopes} onChange={(v) => setAuth({ ...auth, oidc_scopes: v })} label="Extra scopes"
            />
          </SettingsRow>
        </SettingsCard>
      )}

      {(auth.mode === 'forward_auth' || auth.mode === 'oidc') && (
        <SettingsCard title="Authorization">
          <SettingsRow
            label="Admin group" htmlFor="auth-admin-group" lockKey="auth.admin_group"
            description="Members are admins; everyone else is read-only. Empty means everyone is admin."
          >
            <TextField value={auth.admin_group} onChange={(v) => setAuth({ ...auth, admin_group: v })} />
          </SettingsRow>

          {auth.mode === 'oidc' && (
            <>
              <SettingsRow label="Allowed groups" htmlFor="auth-oidc-allowed-groups" lockKey="auth.oidc_allowed_groups">
                <ListField
                  value={auth.oidc_allowed_groups} onChange={(v) => setAuth({ ...auth, oidc_allowed_groups: v })}
                  label="Allowed groups"
                />
              </SettingsRow>
              <SettingsRow label="Allowed emails" htmlFor="auth-oidc-allowed-emails" lockKey="auth.oidc_allowed_emails">
                <ListField
                  value={auth.oidc_allowed_emails} onChange={(v) => setAuth({ ...auth, oidc_allowed_emails: v })}
                  label="Allowed emails"
                />
              </SettingsRow>
              <SettingsRow
                label="Session TTL (hours)" htmlFor="auth-oidc-session-ttl" lockKey="auth.session_ttl_hours" size="sm"
              >
                <NumberField
                  value={auth.session_ttl_hours} onChange={(v) => setAuth({ ...auth, session_ttl_hours: v })}
                />
              </SettingsRow>
            </>
          )}

          <SettingsRow label="Accept API tokens as well" htmlFor="auth-allow-tokens" lockKey="auth.allow_tokens">
            <SwitchRowField checked={auth.allow_tokens} onChange={(v) => setAuth({ ...auth, allow_tokens: v })} />
          </SettingsRow>
        </SettingsCard>
      )}

      {auth.mode !== 'open' && (
        <SettingsCard
          title="API tokens"
          description={(
            <>
              Send a token as <code>Authorization: Bearer &lt;token&gt;</code>. The live-events
              stream also accepts <code>?token=</code>.
            </>
          )}
        >
          <TokenPanel />
        </SettingsCard>
      )}
    </div>
  );
}
