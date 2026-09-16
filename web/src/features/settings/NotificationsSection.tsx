import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { SettingsCard, SettingsRow, useSettingsRowField } from '@/components/settings';
import { ThresholdFields } from '../targets/ThresholdFields';
import { ChannelEditor } from './ChannelEditor';
import { channelErrorTarget, isChannelUnsaved } from './channelHelpers';
import { useSettingsSection } from './useSettingsSection';

function CooldownField({ value, onChange }: { value: number; onChange: (v: number) => void }) {
  const props = useSettingsRowField();
  return <Input {...props} type="number" min={1} value={value} onChange={(e) => onChange(Number(e.target.value))} />;
}

function RecoverySwitchField({ checked, onChange }: { checked: boolean; onChange: (v: boolean) => void }) {
  const props = useSettingsRowField();
  return <Switch {...props} checked={checked} onCheckedChange={onChange} />;
}

function QuietHoursFields({
  start, end, onChangeStart, onChangeEnd,
}: { start: string; end: string; onChangeStart: (v: string) => void; onChangeEnd: (v: string) => void }) {
  const { id, ...rest } = useSettingsRowField();
  return (
    <div className="flex items-center gap-2">
      <Input
        {...rest} id={id} type="time" aria-label="Quiet hours start" className="min-w-0"
        value={start} onChange={(e) => onChangeStart(e.target.value)}
      />
      <span className="text-sm text-faint">to</span>
      <Input
        {...rest} id="notifications-quiet-end" type="time" aria-label="Quiet hours end" className="min-w-0"
        value={end} onChange={(e) => onChangeEnd(e.target.value)}
      />
    </div>
  );
}

export function NotificationsSection() {
  const {
    notifications, setNotifications, readOnly, error,
    addChannel, updateChannel, removeChannel, runChannelTest, channelResults, testingChannelId, savedChannels,
  } = useSettingsSection('notifications');

  const errorTarget = error ? channelErrorTarget(error) : null;

  return (
    <div className="grid gap-4">
      <SettingsCard
        title="Delivery"
        description="Nothing is delivered while this is off."
        headerAction={(
          <Switch
            id="notify-enabled" aria-label="Enabled" checked={notifications.enabled} disabled={readOnly}
            onCheckedChange={(checked) => setNotifications({ ...notifications, enabled: checked })}
          />
        )}
      >
        <SettingsRow
          label="Cooldown (minutes)" htmlFor="notifications-cooldown" lockKey="notifications.cooldown_minutes"
          size="sm"
        >
          <CooldownField
            value={notifications.cooldown_minutes}
            onChange={(v) => setNotifications({ ...notifications, cooldown_minutes: v })}
          />
        </SettingsRow>
        <SettingsRow
          label="Quiet hours" description="No notifications sent in this window."
          htmlFor="notifications-quiet-start" lockKey="notifications.quiet_hours_start"
        >
          <QuietHoursFields
            start={notifications.quiet_hours_start}
            end={notifications.quiet_hours_end}
            onChangeStart={(v) => setNotifications({ ...notifications, quiet_hours_start: v })}
            onChangeEnd={(v) => setNotifications({ ...notifications, quiet_hours_end: v })}
          />
        </SettingsRow>
        <SettingsRow
          label="Send recovery notifications" htmlFor="notifications-notify-recovery"
          lockKey="notifications.notify_recovery"
        >
          <RecoverySwitchField
            checked={notifications.notify_recovery}
            onChange={(checked) => setNotifications({ ...notifications, notify_recovery: checked })}
          />
        </SettingsRow>
      </SettingsCard>

      <SettingsCard
        title="Channels"
        description="Where alerts are delivered."
        footer={<Button type="button" disabled={readOnly} onClick={addChannel}>Add channel</Button>}
      >
        {notifications.channels.length === 0 && (
          <p className="px-4 py-3 text-sm text-faint">No channels yet — nothing will be delivered.</p>
        )}
        {notifications.channels.map((channel) => {
          const invalid = !!errorTarget && (errorTarget === channel.id || errorTarget === channel.name);
          return (
            <ChannelEditor
              key={channel.id}
              value={channel}
              onChange={(next) => updateChannel(channel.id, next)}
              onRemove={() => removeChannel(channel.id)}
              onTest={() => runChannelTest(channel)}
              testResult={channelResults[channel.id]}
              testPending={testingChannelId === channel.id}
              unsaved={isChannelUnsaved(channel, savedChannels)}
              readOnly={readOnly}
              invalid={invalid}
              errorMessage={invalid ? error : undefined}
            />
          );
        })}
      </SettingsCard>

      <SettingsCard title="Default thresholds" description="Targets can override any of these.">
        <div className="px-4 py-3">
          <ThresholdFields
            value={notifications.default_thresholds}
            onChange={(next) => setNotifications({ ...notifications, default_thresholds: next })}
            allowDisable={false}
            showCustomToggle={false}
          />
        </div>
      </SettingsCard>
    </div>
  );
}
