import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { LabelsEditor } from '../../components/LabelsEditor';
import { SwitchField } from '../../components/SwitchField';
import type { NotifyChannel, NotifyChannelType } from '../../lib/api';
import { fieldClass, inputClass, labelClass } from './styles';

interface Props {
  value: NotifyChannel;
  onChange: (next: NotifyChannel) => void;
  onRemove: () => void;
  onTest: () => void;
  testResult?: { ok: boolean; message: string } | null;
  testPending?: boolean;
  /** True when this channel has unsaved edits (or is not yet saved), so
   * testing it would test stale/nonexistent server state. */
  unsaved?: boolean;
}

/** ChannelEditor edits one notification channel. Changing Type keeps all
 * fields (headers, token, priority, tags, urls) in local state so a
 * type flip-flop doesn't lose data such as a token; fields the new type
 * doesn't use are only stripped when the form is submitted. */
export function ChannelEditor({
  value, onChange, onRemove, onTest, testResult, testPending, unsaved,
}: Props) {
  const { id } = value;

  const changeType = (type: NotifyChannelType) => {
    onChange({ ...value, type });
  };

  return (
    <div className="grid gap-3 rounded border border-line p-3">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Badge variant="secondary" className="uppercase">{value.type}</Badge>
          <span className="text-sm font-medium text-fg">{value.name}</span>
        </div>
        <SwitchField
          id={`channel-${id}-enabled`} label="Enabled"
          checked={value.enabled}
          onCheckedChange={(checked) => onChange({ ...value, enabled: checked })}
        />
      </div>

      <div className={fieldClass}>
        <label htmlFor={`channel-${id}-name`} className={labelClass}>Name</label>
        <Input id={`channel-${id}-name`} value={value.name}
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

      <div className={fieldClass}>
        <label htmlFor={`channel-${id}-url`} className={labelClass}>URL</label>
        <Input id={`channel-${id}-url`} value={value.url}
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
          <Input id={`channel-${id}-token`} type="password" placeholder="leave unchanged"
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
          <Input id={`channel-${id}-tags`}
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
        <Button type="button" variant="outline" disabled={testPending || unsaved}
          title={unsaved ? 'Save first to test this channel' : undefined} onClick={onTest}>
          Test {value.name}
        </Button>
        <Button type="button" variant="ghost" className="text-muted hover:text-bad" onClick={onRemove}>
          Remove {value.name}
        </Button>
        {unsaved && <p className="text-sm text-faint">Save first to test this channel</p>}
        {testResult && (
          <p className={testResult.ok ? 'text-sm text-ok' : 'text-sm text-bad'}>{testResult.message}</p>
        )}
      </div>
    </div>
  );
}
