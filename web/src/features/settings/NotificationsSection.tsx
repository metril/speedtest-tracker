import { Button } from '@/components/ui/button';
import { FormField } from '@/components/FormField';
import { Input } from '@/components/ui/input';
import { SwitchField } from '../../components/SwitchField';
import { ThresholdFields } from '../targets/ThresholdFields';
import { ChannelEditor } from './ChannelEditor';
import { isChannelUnsaved } from './channelHelpers';
import { Section } from './Section';
import { useSettingsSection } from './useSettingsSection';

export function NotificationsSection() {
  const {
    notifications, setNotifications, saving, error, saved, saveNotifications, readOnly,
    addChannel, updateChannel, removeChannel, runChannelTest, channelResults, testingChannelId, savedChannels,
  } = useSettingsSection('notifications');

  return (
    <Section
      id="notifications-heading" title="Notifications" saving={saving}
      error={error} saved={saved} readOnly={readOnly}
      onSave={saveNotifications}
    >
      <p className="text-sm text-faint">
        Send a message when a target fails its thresholds (or recovers), through one or more channels below.
      </p>
      <div className="flex justify-end">
        <Button type="button" disabled={readOnly} onClick={addChannel}>Add channel</Button>
      </div>

      <SwitchField
        id="notifications-enabled" label="Enabled"
        hint="Nothing is delivered while this is off."
        checked={notifications.enabled}
        onCheckedChange={(checked) => setNotifications({ ...notifications, enabled: checked })}
      />

      <div className="grid gap-3">
        {notifications.channels.map((channel) => (
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
          />
        ))}
      </div>

      <FormField id="notifications-cooldown" label="Cooldown (minutes)">
        <Input id="notifications-cooldown" type="number" min={1}
          value={notifications.cooldown_minutes}
          onChange={(e) => setNotifications({ ...notifications, cooldown_minutes: Number(e.target.value) })} />
      </FormField>
      <FormField id="notifications-quiet-start" label="Quiet hours start">
        <Input id="notifications-quiet-start" type="time"
          value={notifications.quiet_hours_start}
          onChange={(e) => setNotifications({ ...notifications, quiet_hours_start: e.target.value })} />
      </FormField>
      <FormField id="notifications-quiet-end" label="Quiet hours end">
        <Input id="notifications-quiet-end" type="time"
          value={notifications.quiet_hours_end}
          onChange={(e) => setNotifications({ ...notifications, quiet_hours_end: e.target.value })} />
      </FormField>
      <SwitchField
        id="notifications-notify-recovery" label="Send recovery notifications"
        checked={notifications.notify_recovery}
        onCheckedChange={(checked) => setNotifications({ ...notifications, notify_recovery: checked })}
      />

      <div className="grid gap-2">
        <h3 className="text-sm font-semibold text-fg">Default thresholds</h3>
        <p className="text-sm text-faint">Targets can override any of these in the target form.</p>
        <ThresholdFields
          value={notifications.default_thresholds}
          onChange={(next) => setNotifications({ ...notifications, default_thresholds: next })}
          allowDisable={false}
        />
      </div>
    </Section>
  );
}
