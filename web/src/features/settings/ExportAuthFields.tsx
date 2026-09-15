import { Input } from '@/components/ui/input';
import { SettingsFormProvider, SettingsRow, useSettingsRowField } from '@/components/settings';
import type { ExportAuth } from '../../lib/api';
import { inputClass } from './styles';

interface Props {
  idPrefix: 'vm' | 'vl';
  label: string;
  value: ExportAuth;
  onChange: (next: ExportAuth) => void;
  /** locked reports whether a field key (e.g. "vm_auth_type") is pinned
   * by an ST_ environment variable, same convention as AuthSection. */
  locked?: (key: string) => boolean;
  /** readOnly disables every input/select regardless of locked, for a
   * non-admin viewer under oidc/forward_auth (same convention as the
   * other settings fields). */
  readOnly?: boolean;
}

const AUTH_KEYS = ['auth_type', 'auth_username', 'auth_password', 'auth_token', 'auth_header_name', 'auth_header_value'] as const;

function AuthTypeField({ value, onChange }: { value: ExportAuth['type']; onChange: (v: ExportAuth['type']) => void }) {
  const props = useSettingsRowField();
  return (
    <select
      {...props} className={inputClass} value={value}
      onChange={(e) => onChange(e.target.value as ExportAuth['type'])}
    >
      <option value="none">None</option>
      <option value="basic">Basic</option>
      <option value="bearer">Bearer</option>
      <option value="custom">Custom header</option>
    </select>
  );
}

function TextField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const props = useSettingsRowField();
  return <Input {...props} value={value} onChange={(e) => onChange(e.target.value)} />;
}

function SecretField({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const props = useSettingsRowField();
  return <Input {...props} type="password" placeholder="leave unchanged" value={value} onChange={(e) => onChange(e.target.value)} />;
}

/** ExportAuthFields is the auth block shared by the VictoriaMetrics and
 * VictoriaLogs integrations: a type select (None/Basic/Bearer/Custom
 * header) that swaps in only the fields that type uses. Secrets
 * (password, token, header value) are never echoed back by the server,
 * so they use the same "leave unchanged" placeholder convention as every
 * other masked secret field.
 *
 * Renders a flat list of SettingsRows (a Fragment, not a wrapping div)
 * so it drops straight into a SettingsCard's divide-y body alongside the
 * caller's other rows. It carries its own SettingsFormProvider, fed by
 * its `locked`/`readOnly` props translated into real settings keys
 * (`integrations.vm_auth_type`, ...), so its rows disable correctly
 * independent of whatever ambient form context the caller sits in. */
export function ExportAuthFields({ idPrefix, label, value, onChange, locked, readOnly }: Props) {
  const lockedKeys = AUTH_KEYS
    .filter((key) => locked?.(`${idPrefix}_${key}`) ?? false)
    .map((key) => `integrations.${idPrefix}_${key}`);
  const keyFor = (key: string) => `integrations.${idPrefix}_${key}`;
  const idFor = (key: string) => `${idPrefix}-${key.replace(/_/g, '-')}`;

  return (
    <SettingsFormProvider readOnly={readOnly ?? false} locked={lockedKeys}>
      <SettingsRow label={label} htmlFor={idFor('auth_type')} lockKey={keyFor('auth_type')} size="sm">
        <AuthTypeField value={value.type} onChange={(type) => onChange({ ...value, type })} />
      </SettingsRow>

      {value.type === 'basic' && (
        <>
          <SettingsRow label={`${label} username`} htmlFor={idFor('auth_username')} lockKey={keyFor('auth_username')}>
            <TextField value={value.username ?? ''} onChange={(v) => onChange({ ...value, username: v })} />
          </SettingsRow>
          <SettingsRow label={`${label} password`} htmlFor={idFor('auth_password')} lockKey={keyFor('auth_password')}>
            <SecretField value={value.password ?? ''} onChange={(v) => onChange({ ...value, password: v })} />
          </SettingsRow>
        </>
      )}

      {value.type === 'bearer' && (
        <SettingsRow label={`${label} token`} htmlFor={idFor('auth_token')} lockKey={keyFor('auth_token')}>
          <SecretField value={value.token ?? ''} onChange={(v) => onChange({ ...value, token: v })} />
        </SettingsRow>
      )}

      {value.type === 'custom' && (
        <>
          <SettingsRow label={`${label} header name`} htmlFor={idFor('auth_header_name')} lockKey={keyFor('auth_header_name')}>
            <TextField value={value.header_name ?? ''} onChange={(v) => onChange({ ...value, header_name: v })} />
          </SettingsRow>
          <SettingsRow label={`${label} header value`} htmlFor={idFor('auth_header_value')} lockKey={keyFor('auth_header_value')}>
            <SecretField value={value.header_value ?? ''} onChange={(v) => onChange({ ...value, header_value: v })} />
          </SettingsRow>
        </>
      )}
    </SettingsFormProvider>
  );
}
