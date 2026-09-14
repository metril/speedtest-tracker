import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { SwitchField } from '../../components/SwitchField';
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
};

/** A CIDR line must look like "<ip>/<bits>" -- no spaces, one slash. This
 * is a client-side sanity check; the server remains the authority. */
const CIDR_RE = /^\S+\/\d{1,3}$/;

/** validateAuthSettings runs before save('auth', ...): forward_auth is
 * useless (and dangerous -- it would trust nobody, or everybody) without
 * at least one well-formed trusted proxy CIDR. */
export function validateAuthSettings(auth: AuthSettings): string | undefined {
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

      {value.mode === 'forward_auth' && (
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
