import { LabelsEditor } from '../../components/LabelsEditor';
import type { NotifyChannel, NotifyChannelType } from '../../lib/api';
import { buttonClass, fieldClass, inputClass, labelClass } from './styles';

interface Props {
  value: NotifyChannel;
  onChange: (next: NotifyChannel) => void;
  onRemove: () => void;
  onTest: () => void;
  testResult?: { ok: boolean; message: string } | null;
  testPending?: boolean;
}

/** ChannelEditor edits one notification channel. Changing Type keeps the
 * id/name/url/enabled fields and drops whatever the previous type used
 * that the new type does not (headers, token, priority, tags, urls). */
export function ChannelEditor({
  value, onChange, onRemove, onTest, testResult, testPending,
}: Props) {
  const { id } = value;

  const changeType = (type: NotifyChannelType) => {
    onChange({
      id: value.id, type, name: value.name, enabled: value.enabled, url: value.url,
    });
  };

  return (
    <div className="grid gap-3 rounded border border-line p-3">
      <div className={fieldClass}>
        <label htmlFor={`channel-${id}-name`} className={labelClass}>Name</label>
        <input id={`channel-${id}-name`} className={inputClass} value={value.name}
          onChange={(e) => onChange({ ...value, name: e.target.value })} />
      </div>

      <div className={fieldClass}>
        <label htmlFor={`channel-${id}-type`} className={labelClass}>Type</label>
        <select id={`channel-${id}-type`} className={inputClass} value={value.type}
          onChange={(e) => changeType(e.target.value as NotifyChannelType)}>
          <option value="webhook">webhook</option>
          <option value="ntfy">ntfy</option>
          <option value="apprise">apprise</option>
        </select>
      </div>

      <div className="flex items-center gap-2 text-sm text-muted">
        <input id={`channel-${id}-enabled`} type="checkbox" checked={value.enabled}
          onChange={(e) => onChange({ ...value, enabled: e.target.checked })} />
        <label htmlFor={`channel-${id}-enabled`}>Enabled</label>
      </div>

      <div className={fieldClass}>
        <label htmlFor={`channel-${id}-url`} className={labelClass}>URL</label>
        <input id={`channel-${id}-url`} className={inputClass} value={value.url}
          onChange={(e) => onChange({ ...value, url: e.target.value })} />
      </div>

      {value.type === 'webhook' && (
        <div aria-label="Headers">
          <LabelsEditor
            label="Headers"
            value={value.headers ?? {}}
            onChange={(v) => onChange({ ...value, headers: v })}
          />
        </div>
      )}

      {(value.type === 'ntfy' || value.type === 'apprise') && (
        <div className={fieldClass}>
          <label htmlFor={`channel-${id}-token`} className={labelClass}>Token</label>
          <input id={`channel-${id}-token`} type="password" placeholder="leave unchanged" className={inputClass}
            value={value.token ?? ''} onChange={(e) => onChange({ ...value, token: e.target.value })} />
        </div>
      )}

      {value.type === 'ntfy' && (
        <div className={fieldClass}>
          <label htmlFor={`channel-${id}-priority`} className={labelClass}>Priority</label>
          <select id={`channel-${id}-priority`} className={inputClass} value={value.priority ?? 'default'}
            onChange={(e) => onChange({ ...value, priority: e.target.value })}>
            <option value="min">min</option>
            <option value="low">low</option>
            <option value="default">default</option>
            <option value="high">high</option>
            <option value="max">max</option>
          </select>
        </div>
      )}

      {(value.type === 'ntfy' || value.type === 'apprise') && (
        <div className={fieldClass}>
          <label htmlFor={`channel-${id}-tags`} className={labelClass}>Tags</label>
          <input id={`channel-${id}-tags`} className={inputClass}
            value={(value.tags ?? []).join(', ')}
            onChange={(e) => onChange({
              ...value,
              tags: e.target.value.split(',').map((t) => t.trim()).filter(Boolean),
            })} />
        </div>
      )}

      {value.type === 'apprise' && (
        <div className={fieldClass}>
          <label htmlFor={`channel-${id}-urls`} className={labelClass}>Apprise URLs</label>
          <textarea id={`channel-${id}-urls`} className={inputClass}
            value={(value.urls ?? []).join('\n')}
            onChange={(e) => onChange({ ...value, urls: e.target.value.split('\n') })} />
        </div>
      )}

      <div className="flex items-center gap-3">
        <button type="button" className={buttonClass} disabled={testPending} onClick={onTest}>
          Test {value.name}
        </button>
        <button type="button" className="text-muted hover:text-bad" onClick={onRemove}>
          Remove {value.name}
        </button>
        {testResult && (
          <p className={testResult.ok ? 'text-sm text-ok' : 'text-sm text-bad'}>{testResult.message}</p>
        )}
      </div>
    </div>
  );
}
