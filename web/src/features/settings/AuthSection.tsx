import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { SwitchField } from '../../components/SwitchField';
import { testOIDC } from '../../lib/api';
import type { AuthMode, AuthSettings } from '../../lib/api';
import { LockedBadge } from './LockedBadge';
import { inputClass } from './styles';

interface Props {
  value: AuthSettings;
  locked: string[];
  onChange: (next: AuthSettings) => void;
}

const MODE_HELP: Record<AuthMode, string> = {
  open: 'No authentication. Anyone who can reach this instance can change settings and run tests.',
  forward_auth: 'Trusts identity headers set by a reverse proxy. Requires the trusted proxy CIDRs below.',
  token: 'Requires an API token in the Authorization header for every request.',
  oidc: 'Users sign in through your OpenID Connect provider; admin decided by the admin group.',
};

/** splitList/joinList convert between a string[] setting and the
 * comma/space-separated text a single input edits. */
function splitList(text: string): string[] {
  return text.split(/[,\s]+/).map((s) => s.trim()).filter(Boolean);
}
function joinList(values: string[]): string {
  return values.join(', ');
}

/** A CIDR line must look like "<ip>/<bits>" -- no spaces, one slash. This
 * is a client-side sanity check; the server remains the authority. */
const CIDR_RE = /^\S+\/\d{1,3}$/;

/** validateAuthSettings runs before save('auth', ...): forward_auth is
 * useless (and dangerous -- it would trust nobody, or everybody) without
 * at least one well-formed trusted proxy CIDR. */
export function validateAuthSettings(auth: AuthSettings): string | undefined {
  if (auth.mode === 'oidc') {
    if (!auth.oidc_issuer.trim()) return 'oidc requires an issuer URL.';
    if (!auth.oidc_client_id.trim()) return 'oidc requires a client ID.';
    return undefined;
  }
  if (auth.mode !== 'forward_auth') return undefined;
  const lines = auth.trusted_proxies.map((l) => l.trim()).filter(Boolean);
  if (lines.length === 0) {
    return 'forward_auth requires at least one trusted proxy CIDR.';
  }
  const bad = lines.find((l) => !CIDR_RE.test(l));
  if (bad) return `"${bad}" does not look like a CIDR (e.g. 10.0.0.0/8).`;
  return undefined;
}

function FieldLabel({
  htmlFor, text, lockKey, locked,
}: { htmlFor: string; text: string; lockKey: string; locked: string[] }) {
  return (
    <div className="flex items-center gap-2">
      <Label htmlFor={htmlFor}>{text}</Label>
      {locked.includes(lockKey) && <LockedBadge />}
    </div>
  );
}

/** AuthSection edits AuthSettings. Every control is disabled when its key
 * is present in `locked` (set by an ST_ environment variable while
 * ST_LOCK_ENV is on), with a LockedBadge next to its label explaining why. */
export function AuthSection({ value, locked, onChange }: Props) {
  const isLocked = (key: string) => locked.includes(key);

  return (
    <>
      <div className="grid gap-1">
        <FieldLabel htmlFor="auth-mode" text="Auth mode" lockKey="auth.mode" locked={locked} />
        <select id="auth-mode" className={inputClass} value={value.mode} disabled={isLocked('auth.mode')}
          onChange={(e) => onChange({ ...value, mode: e.target.value as AuthMode })}>
          <option value="open">open</option>
          <option value="forward_auth">forward_auth</option>
          <option value="token">token</option>
          <option value="oidc">oidc</option>
        </select>
        <p className="text-sm text-faint">{MODE_HELP[value.mode]}</p>
      </div>

      <div className="grid gap-1">
        <FieldLabel htmlFor="auth-user-header" text="User header" lockKey="auth.user_header" locked={locked} />
        <Input id="auth-user-header" value={value.user_header}
          disabled={isLocked('auth.user_header')}
          onChange={(e) => onChange({ ...value, user_header: e.target.value })} />
      </div>

      <div className="grid gap-1">
        <FieldLabel htmlFor="auth-groups-header" text="Groups header" lockKey="auth.groups_header" locked={locked} />
        <Input id="auth-groups-header" value={value.groups_header}
          disabled={isLocked('auth.groups_header')}
          onChange={(e) => onChange({ ...value, groups_header: e.target.value })} />
      </div>

      <div className="grid gap-1">
        <FieldLabel htmlFor="auth-groups-separator" text="Groups separator" lockKey="auth.groups_separator" locked={locked} />
        <Input id="auth-groups-separator" value={value.groups_separator}
          disabled={isLocked('auth.groups_separator')}
          onChange={(e) => onChange({ ...value, groups_separator: e.target.value })} />
      </div>

      <div className="grid gap-1">
        <FieldLabel htmlFor="auth-admin-group" text="Admin group" lockKey="auth.admin_group" locked={locked} />
        <Input id="auth-admin-group" value={value.admin_group}
          disabled={isLocked('auth.admin_group')}
          onChange={(e) => onChange({ ...value, admin_group: e.target.value })} />
      </div>

      <div className="grid gap-1">
        <FieldLabel htmlFor="auth-trusted-proxies" text="Trusted proxy CIDRs" lockKey="auth.trusted_proxies" locked={locked} />
        <textarea id="auth-trusted-proxies" className={inputClass} rows={3}
          disabled={isLocked('auth.trusted_proxies')}
          value={value.trusted_proxies.join('\n')}
          onChange={(e) => onChange({ ...value, trusted_proxies: e.target.value.split('\n') })} />
        <p className="text-sm text-faint">
          Required. Identity headers are ignored unless the connecting peer is inside one of these ranges.
        </p>
      </div>

      {value.mode === 'oidc' && <OidcFields value={value} locked={locked} onChange={onChange} />}

      {(value.mode === 'forward_auth' || value.mode === 'oidc') && (
        <div className="flex items-center gap-2">
          <div className="flex-1">
            <SwitchField
              id="auth-allow-tokens" label="Accept API tokens as well"
              checked={value.allow_tokens}
              disabled={isLocked('auth.allow_tokens')}
              onCheckedChange={(checked) => onChange({ ...value, allow_tokens: checked })}
            />
          </div>
          {isLocked('auth.allow_tokens') && <LockedBadge />}
        </div>
      )}
    </>
  );
}

/** OidcFields is the mode==='oidc' fieldset: provider config plus a
 * "Test OIDC discovery" action that hits the server's discovery-test
 * endpoint with the values currently in the form (not the saved ones). */
function OidcFields({ value, locked, onChange }: Props) {
  const isLocked = (key: string) => locked.includes(key);
  const [testing, setTesting] = useState(false);
  const [result, setResult] = useState<{ ok: boolean; message: string } | null>(null);

  const runTest = async () => {
    setTesting(true);
    setResult(null);
    try {
      const res = await testOIDC({
        issuer: value.oidc_issuer,
        client_id: value.oidc_client_id,
        client_secret: value.oidc_client_secret,
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
    <>
      <div className="grid gap-1">
        <FieldLabel htmlFor="auth-oidc-issuer" text="Issuer" lockKey="auth.oidc_issuer" locked={locked} />
        <Input id="auth-oidc-issuer" value={value.oidc_issuer}
          disabled={isLocked('auth.oidc_issuer')}
          onChange={(e) => onChange({ ...value, oidc_issuer: e.target.value })} />
      </div>

      <div className="grid gap-1">
        <FieldLabel htmlFor="auth-oidc-client-id" text="Client ID" lockKey="auth.oidc_client_id" locked={locked} />
        <Input id="auth-oidc-client-id" value={value.oidc_client_id}
          disabled={isLocked('auth.oidc_client_id')}
          onChange={(e) => onChange({ ...value, oidc_client_id: e.target.value })} />
      </div>

      <div className="grid gap-1">
        <FieldLabel htmlFor="auth-oidc-client-secret" text="Client secret" lockKey="auth.oidc_client_secret" locked={locked} />
        <Input id="auth-oidc-client-secret" type="password" placeholder="leave unchanged"
          value={value.oidc_client_secret}
          disabled={isLocked('auth.oidc_client_secret')}
          onChange={(e) => onChange({ ...value, oidc_client_secret: e.target.value })} />
      </div>

      <div className="flex items-center gap-3">
        <Button type="button" variant="outline" disabled={testing} onClick={runTest}>
          Test OIDC discovery
        </Button>
        {result && (
          <p className={result.ok ? 'text-sm text-ok' : 'text-sm text-bad'}>{result.message}</p>
        )}
      </div>

      <div className="grid gap-1">
        <FieldLabel htmlFor="auth-oidc-redirect-base-url" text="Redirect base URL" lockKey="auth.oidc_redirect_base_url" locked={locked} />
        <Input id="auth-oidc-redirect-base-url" value={value.oidc_redirect_base_url}
          disabled={isLocked('auth.oidc_redirect_base_url')}
          onChange={(e) => onChange({ ...value, oidc_redirect_base_url: e.target.value })} />
        <p className="text-sm text-faint">
          Leave empty to derive from the request. The callback path is /auth/oidc/callback.
        </p>
      </div>

      <div className="grid gap-1">
        <FieldLabel htmlFor="auth-oidc-scopes" text="Extra scopes" lockKey="auth.oidc_scopes" locked={locked} />
        <Input id="auth-oidc-scopes" value={joinList(value.oidc_scopes)}
          disabled={isLocked('auth.oidc_scopes')}
          onChange={(e) => onChange({ ...value, oidc_scopes: splitList(e.target.value) })} />
      </div>

      <div className="grid gap-1">
        <FieldLabel htmlFor="auth-oidc-groups-claim" text="Groups claim" lockKey="auth.oidc_groups_claim" locked={locked} />
        <Input id="auth-oidc-groups-claim" value={value.oidc_groups_claim}
          placeholder="groups"
          disabled={isLocked('auth.oidc_groups_claim')}
          onChange={(e) => onChange({ ...value, oidc_groups_claim: e.target.value })} />
      </div>

      <div className="grid gap-1">
        <FieldLabel htmlFor="auth-oidc-allowed-groups" text="Allowed groups" lockKey="auth.oidc_allowed_groups" locked={locked} />
        <Input id="auth-oidc-allowed-groups" value={joinList(value.oidc_allowed_groups)}
          disabled={isLocked('auth.oidc_allowed_groups')}
          onChange={(e) => onChange({ ...value, oidc_allowed_groups: splitList(e.target.value) })} />
      </div>

      <div className="grid gap-1">
        <FieldLabel htmlFor="auth-oidc-allowed-emails" text="Allowed emails" lockKey="auth.oidc_allowed_emails" locked={locked} />
        <Input id="auth-oidc-allowed-emails" value={joinList(value.oidc_allowed_emails)}
          disabled={isLocked('auth.oidc_allowed_emails')}
          onChange={(e) => onChange({ ...value, oidc_allowed_emails: splitList(e.target.value) })} />
      </div>

      <div className="grid gap-1">
        <FieldLabel htmlFor="auth-oidc-session-ttl" text="Session TTL (hours)" lockKey="auth.session_ttl_hours" locked={locked} />
        <Input id="auth-oidc-session-ttl" type="number" value={value.session_ttl_hours}
          disabled={isLocked('auth.session_ttl_hours')}
          onChange={(e) => onChange({ ...value, session_ttl_hours: Number(e.target.value) })} />
      </div>
    </>
  );
}
