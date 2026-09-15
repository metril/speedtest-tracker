import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { FormField } from '@/components/FormField';
import { Input } from '@/components/ui/input';
import { LabelsEditor } from '../../components/LabelsEditor';
import { SwitchField } from '../../components/SwitchField';
import type { NotifyChannel, NotifyChannelType } from '../../lib/api';
import { inputClass } from './styles';

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
 * fields (url, headers, tags, urls) in local state so a type flip-flop
 * doesn't lose data such as a saved Apprise URL list; fields the new
 * type doesn't use are only stripped when the form is submitted. */
export function ChannelEditor({
  value, onChange, onRemove, onTest, testResult, testPending, unsaved,
}: Props) {
  const { id } = value;

  const changeType = (type: NotifyChannelType) => {
    onChange({ ...value, type });
  };

  return (
    <div className="grid gap-3 rounded-md border border-line p-3">
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

      <FormField id={`channel-${id}-name`} label="Name">
        <Input id={`channel-${id}-name`} value={value.name}
          onChange={(e) => onChange({ ...value, name: e.target.value })} />
      </FormField>

      <FormField id={`channel-${id}-type`} label="Type">
        <select id={`channel-${id}-type`} className={inputClass} value={value.type}
          onChange={(e) => changeType(e.target.value as NotifyChannelType)}>
          <option value="webhook">webhook</option>
          <option value="apprise">apprise</option>
        </select>
      </FormField>

      {value.type === 'webhook' && (
        <FormField id={`channel-${id}-url`} label="URL">
          <Input id={`channel-${id}-url`} value={value.url}
            onChange={(e) => onChange({ ...value, url: e.target.value })} />
        </FormField>
      )}

      {value.type === 'webhook' && (
        <div aria-label="Headers">
          <LabelsEditor
            label="Headers"
            value={value.headers ?? {}}
            onChange={(v) => onChange({ ...value, headers: v })}
          />
        </div>
      )}

      {value.type === 'apprise' && (
        <FormField id={`channel-${id}-urls`} label="Apprise URLs">
          <textarea id={`channel-${id}-urls`} className={inputClass}
            value={(value.urls ?? []).join('\n')}
            onChange={(e) => onChange({ ...value, urls: e.target.value.split('\n') })} />
        </FormField>
      )}

      {value.type === 'apprise' && (
        <FormField id={`channel-${id}-tags`} label="Tags">
          <Input id={`channel-${id}-tags`}
            value={(value.tags ?? []).join(', ')}
            onChange={(e) => onChange({
              ...value,
              tags: e.target.value.split(',').map((t) => t.trim()).filter(Boolean),
            })} />
        </FormField>
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
