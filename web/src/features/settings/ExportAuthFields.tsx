import { FormField } from '@/components/FormField';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import type { ExportAuth } from '../../lib/api';
import { LockedBadge } from './LockedBadge';
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

/** ExportAuthFields is the auth block shared by the VictoriaMetrics and
 * VictoriaLogs integrations: a type select (None/Basic/Bearer/Custom
 * header) that swaps in only the fields that type uses. Secrets
 * (password, token, header value) are never echoed back by the server,
 * so they use the same "leave unchanged" placeholder convention as every
 * other masked secret field. */
export function ExportAuthFields({ idPrefix, label, value, onChange, locked, readOnly }: Props) {
  const isLocked = (key: string) => locked?.(`${idPrefix}_${key}`) ?? false;
  const isDisabled = (key: string) => isLocked(key) || (readOnly ?? false);
  const typeId = `${idPrefix}-auth-type`;

  return (
    <div className="grid gap-2">
      <div className="flex items-center gap-2">
        <Label htmlFor={typeId}>{label}</Label>
        {isLocked('auth_type') && <LockedBadge />}
      </div>
      <select
        id={typeId} className={inputClass} value={value.type}
        disabled={isDisabled('auth_type')}
        onChange={(e) => onChange({ ...value, type: e.target.value as ExportAuth['type'] })}
      >
        <option value="none">None</option>
        <option value="basic">Basic</option>
        <option value="bearer">Bearer</option>
        <option value="custom">Custom header</option>
      </select>

      {value.type === 'basic' && (
        <>
          <FormField id={`${idPrefix}-auth-username`} label={`${label} username`}>
            <Input
              id={`${idPrefix}-auth-username`} value={value.username ?? ''}
              disabled={isDisabled('auth_username')}
              onChange={(e) => onChange({ ...value, username: e.target.value })}
            />
          </FormField>
          <FormField id={`${idPrefix}-auth-password`} label={`${label} password`}>
            <Input
              id={`${idPrefix}-auth-password`} type="password" placeholder="leave unchanged"
              value={value.password ?? ''}
              disabled={isDisabled('auth_password')}
              onChange={(e) => onChange({ ...value, password: e.target.value })}
            />
          </FormField>
        </>
      )}

      {value.type === 'bearer' && (
        <FormField id={`${idPrefix}-auth-token`} label={`${label} token`}>
          <Input
            id={`${idPrefix}-auth-token`} type="password" placeholder="leave unchanged"
            value={value.token ?? ''}
            disabled={isDisabled('auth_token')}
            onChange={(e) => onChange({ ...value, token: e.target.value })}
          />
        </FormField>
      )}

      {value.type === 'custom' && (
        <>
          <FormField id={`${idPrefix}-auth-header-name`} label={`${label} header name`}>
            <Input
              id={`${idPrefix}-auth-header-name`} value={value.header_name ?? ''}
              disabled={isDisabled('auth_header_name')}
              onChange={(e) => onChange({ ...value, header_name: e.target.value })}
            />
          </FormField>
          <FormField id={`${idPrefix}-auth-header-value`} label={`${label} header value`}>
            <Input
              id={`${idPrefix}-auth-header-value`} type="password" placeholder="leave unchanged"
              value={value.header_value ?? ''}
              disabled={isDisabled('auth_header_value')}
              onChange={(e) => onChange({ ...value, header_value: e.target.value })}
            />
          </FormField>
        </>
      )}
    </div>
  );
}
