import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { FormField } from '@/components/FormField';
import { Input } from '@/components/ui/input';
import { KeyValueInput } from '../../components/settings/KeyValueInput';
import { TestButton } from '../../components/settings/TestButton';
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
  readOnly?: boolean;
}

/** ChannelEditor edits one notification channel. Changing Type keeps all
 * fields (url, headers, urls) in local state so a type flip-flop doesn't
 * lose data such as a saved Apprise URL list; fields the new type
 * doesn't use are only stripped when the form is submitted. Tags is not
 * editable here: apprise-go (the embedded delivery library) has no tag
 * option, so it would be inert; the field still exists on the model
 * because ntfy-channel migration reads it from legacy stored data. */
export function ChannelEditor({
  value, onChange, onRemove, onTest, testResult, testPending, unsaved, readOnly,
}: Props) {
  const { id } = value;

  const changeType = (type: NotifyChannelType) => {
    onChange({ ...value, type });
  };

  return (
    <div className="grid gap-3 px-4 py-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Badge variant="secondary" className="uppercase">{value.type}</Badge>
          <span className="text-sm font-medium text-fg">{value.name}</span>
        </div>
        <div className="flex items-center gap-3">
          <TestButton
            label={`Test ${value.name}`} onTest={onTest} pending={!!testPending}
            result={testResult} disabled={testPending || unsaved || readOnly}
            disabledReason={unsaved ? 'Save first to test this channel' : readOnly ? 'Read-only: admin group required' : undefined}
          />
          <Button type="button" variant="ghost" className="text-muted hover:text-bad"
            disabled={readOnly} onClick={onRemove}>
            Remove {value.name}
          </Button>
        </div>
      </div>

      <SwitchField
        id={`channel-${id}-enabled`} label="Enabled"
        checked={value.enabled}
        onCheckedChange={(checked) => onChange({ ...value, enabled: checked })}
      />

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
          <KeyValueInput
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
    </div>
  );
}
