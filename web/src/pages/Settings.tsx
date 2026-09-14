import type { ReactNode } from 'react';
import { useEffect, useRef, useState } from 'react';
import { LabelsEditor } from '../components/LabelsEditor';
import { AuthSection, validateAuthSettings } from '../features/settings/AuthSection';
import { ChannelEditor } from '../features/settings/ChannelEditor';
import {
  buttonClass, fieldClass, inputClass, labelClass,
} from '../features/settings/styles';
import { ThresholdFields, validateThresholds } from '../features/targets/ThresholdFields';
import type {
  AuthSettings, EngineSettings, GeneralSettings, IntegrationSettings, NotificationSettings, NotifyChannel,
} from '../lib/api';
import { isChannelUnsaved, stripIrrelevantChannelFields } from '../features/settings/channelHelpers';
import { ApiError } from '../lib/api';
import {
  useSettings, useTestIntegration, useTestNotifyChannel, useUpdateSettings,
} from '../lib/queries';

type SectionKey = 'general' | 'engines' | 'integrations' | 'notifications' | 'auth';

function Section({
  id, title, onSave, saving, error, saved, children,
}: {
  id: string;
  title: string;
  onSave: () => void;
  saving: boolean;
  error?: string;
  saved: boolean;
  children: ReactNode;
}) {
  return (
    <section aria-labelledby={id} className="space-y-4 rounded-lg border border-line bg-surface p-4">
      <h2 id={id} className="text-lg font-semibold text-fg">{title}</h2>
      <div className="grid gap-4">{children}</div>
      <div className="flex items-center gap-3">
        <button type="button" className={buttonClass} disabled={saving} onClick={onSave}>
          Save {title}
        </button>
        {saved && <p className="text-sm text-ok">Saved</p>}
      </div>
      {error && <p role="alert" className="text-sm text-bad">{error}</p>}
    </section>
  );
}

/** useSavedFlash shows a "Saved" message for a few seconds after a
 * successful save, per section. */
function useSavedFlash() {
  const [saved, setSaved] = useState<Record<SectionKey, boolean>>({
    general: false, engines: false, integrations: false, notifications: false, auth: false,
  });
  const timers = useRef<Partial<Record<SectionKey, ReturnType<typeof setTimeout>>>>({});

  useEffect(() => () => {
    Object.values(timers.current).forEach((t) => t && clearTimeout(t));
  }, []);

  const flash = (section: SectionKey) => {
    setSaved((s) => ({ ...s, [section]: true }));
    const existing = timers.current[section];
    if (existing) clearTimeout(existing);
    timers.current[section] = setTimeout(() => setSaved((s) => ({ ...s, [section]: false })), 3000);
  };

  return { saved, flash };
}

export function Settings() {
  const settings = useSettings();
  const update = useUpdateSettings();
  const test = useTestIntegration();
  const testChannel = useTestNotifyChannel();

  const [general, setGeneral] = useState<GeneralSettings | null>(null);
  const [engines, setEngines] = useState<EngineSettings | null>(null);
  const [integrations, setIntegrations] = useState<IntegrationSettings | null>(null);
  const [notifications, setNotifications] = useState<NotificationSettings | null>(null);
  const [auth, setAuth] = useState<AuthSettings | null>(null);
  const [errors, setErrors] = useState<Partial<Record<SectionKey, string>>>({});
  const { saved, flash } = useSavedFlash();
  const [vmResult, setVmResult] = useState<{ ok: boolean; message: string } | null>(null);
  const [vlResult, setVlResult] = useState<{ ok: boolean; message: string } | null>(null);
  const [channelResults, setChannelResults] = useState<Record<string, { ok: boolean; message: string }>>({});
  const [testingChannelId, setTestingChannelId] = useState<string | null>(null);

  // Seed local edit state from the fetched settings exactly once. Refetches
  // (invalidation after save, background refresh, etc.) must never clobber
  // an in-progress, unsaved edit in any section.
  const seeded = useRef(false);
  useEffect(() => {
    if (!settings.data || seeded.current) return;
    seeded.current = true;
    setGeneral(settings.data.general);
    setEngines(settings.data.engines);
    setIntegrations(settings.data.integrations);
    setNotifications(settings.data.notifications);
    setAuth(settings.data.auth);
  }, [settings.data]);

  const message = (err: unknown) => (err instanceof ApiError ? err.message : err ? String(err) : 'Save failed.');

  const save = (section: SectionKey, patch: Record<string, unknown>) => {
    setErrors((e) => ({ ...e, [section]: undefined }));
    update.mutate(patch, {
      onSuccess: () => flash(section),
      onError: (err) => setErrors((e) => ({ ...e, [section]: message(err) })),
    });
  };

  const runTest = (
    target: 'vm' | 'vl',
    url: string,
    authHeader: string,
    setResult: (r: { ok: boolean; message: string } | null) => void,
  ) => {
    setResult(null);
    test.mutate(
      { target, body: { url, auth_header: authHeader } },
      {
        onSuccess: (result) => {
          setResult(result.ok
            ? { ok: true, message: `Connected in ${result.latency_ms ?? 0} ms` }
            : { ok: false, message: result.error ?? 'Connection failed.' });
        },
        onError: (err) => setResult({ ok: false, message: message(err) }),
      },
    );
  };

  const addChannel = () => {
    if (!notifications) return;
    const channel: NotifyChannel = {
      id: crypto.randomUUID().slice(0, 8), type: 'ntfy', name: 'New channel', enabled: true, url: '',
    };
    setNotifications({ ...notifications, channels: [...notifications.channels, channel] });
  };

  const updateChannel = (id: string, next: NotifyChannel) => {
    if (!notifications) return;
    setNotifications({
      ...notifications,
      channels: notifications.channels.map((c) => (c.id === id ? next : c)),
    });
  };

  const removeChannel = (id: string) => {
    if (!notifications) return;
    setNotifications({ ...notifications, channels: notifications.channels.filter((c) => c.id !== id) });
    setChannelResults((r) => Object.fromEntries(Object.entries(r).filter(([key]) => key !== id)));
  };

  const runChannelTest = (channel: NotifyChannel) => {
    setTestingChannelId(channel.id);
    testChannel.mutate(channel.id, {
      onSuccess: (result) => {
        setChannelResults((r) => ({
          ...r,
          [channel.id]: result.ok
            ? { ok: true, message: `Sent in ${result.latency_ms ?? 0} ms` }
            : { ok: false, message: result.error ?? 'Test failed.' },
        }));
      },
      onError: (err) => setChannelResults((r) => ({ ...r, [channel.id]: { ok: false, message: message(err) } })),
      onSettled: () => setTestingChannelId((id) => (id === channel.id ? null : id)),
    });
  };

  const saveAuth = () => {
    if (!auth) return;
    const authError = validateAuthSettings(auth);
    if (authError) {
      setErrors((e) => ({ ...e, auth: authError }));
      return;
    }
    save('auth', { auth });
  };

  const saveNotifications = () => {
    if (!notifications) return;
    const thresholdError = validateThresholds(notifications.default_thresholds);
    if (thresholdError) {
      setErrors((e) => ({ ...e, notifications: thresholdError }));
      return;
    }
    save('notifications', {
      notifications: { ...notifications, channels: notifications.channels.map(stripIrrelevantChannelFields) },
    });
  };

  if (settings.isLoading || !general || !engines || !integrations || !notifications || !auth) {
    return <h1 className="text-xl font-semibold">Settings</h1>;
  }

  return (
    <div className="grid gap-4">
      <h1 className="text-xl font-semibold tracking-tight">Settings</h1>
      <div className="space-y-8">
        <Section
          id="auth-heading" title="Auth" saving={update.isPending}
          error={errors.auth} saved={saved.auth}
          onSave={saveAuth}
        >
          <AuthSection value={auth} locked={settings.data?.locked ?? []} onChange={setAuth} />
        </Section>

        <Section
          id="general-heading" title="General" saving={update.isPending}
          error={errors.general} saved={saved.general}
          onSave={() => save('general', { general })}
        >
          <div className={fieldClass}>
            <label htmlFor="general-base-url" className={labelClass}>Base URL</label>
            <input id="general-base-url" className={inputClass} value={general.base_url}
              onChange={(e) => setGeneral({ ...general, base_url: e.target.value })} />
          </div>
          <div className={fieldClass}>
            <label htmlFor="general-timezone" className={labelClass}>Timezone</label>
            <input id="general-timezone" className={inputClass} value={general.timezone}
              onChange={(e) => setGeneral({ ...general, timezone: e.target.value })} />
          </div>
          <div className={fieldClass}>
            <label htmlFor="general-units" className={labelClass}>Units</label>
            <select id="general-units" className={inputClass} value={general.units}
              onChange={(e) => setGeneral({ ...general, units: e.target.value })}>
              <option value="Mbps">Mbps</option>
              <option value="MB/s">MB/s</option>
            </select>
          </div>
          <div className={fieldClass}>
            <label htmlFor="general-log-level" className={labelClass}>Log level</label>
            <select id="general-log-level" className={inputClass} value={general.log_level}
              onChange={(e) => setGeneral({ ...general, log_level: e.target.value })}>
              <option value="debug">debug</option>
              <option value="info">info</option>
              <option value="warn">warn</option>
              <option value="error">error</option>
            </select>
          </div>
          <div className={fieldClass}>
            <label htmlFor="general-retention-results" className={labelClass}>Results retention (days)</label>
            <input id="general-retention-results" type="number" min={1} className={inputClass}
              value={general.retention_days_results}
              onChange={(e) => setGeneral({ ...general, retention_days_results: Number(e.target.value) })} />
          </div>
          <div className={fieldClass}>
            <label htmlFor="general-retention-runs" className={labelClass}>Runs retention (days)</label>
            <input id="general-retention-runs" type="number" min={1} className={inputClass}
              value={general.retention_days_runs}
              onChange={(e) => setGeneral({ ...general, retention_days_runs: Number(e.target.value) })} />
          </div>
          <div className={fieldClass}>
            <label htmlFor="general-prune-interval" className={labelClass}>Prune interval (minutes)</label>
            <input id="general-prune-interval" type="number" min={1} className={inputClass}
              value={general.retention_prune_interval_minutes}
              onChange={(e) => setGeneral({ ...general, retention_prune_interval_minutes: Number(e.target.value) })} />
          </div>
        </Section>

        <Section
          id="engines-heading" title="Engines" saving={update.isPending}
          error={errors.engines} saved={saved.engines}
          onSave={() => save('engines', { engines })}
        >
          <div className={fieldClass}>
            <label htmlFor="engines-speedtest-bin" className={labelClass}>Speedtest binary path</label>
            <input id="engines-speedtest-bin" className={inputClass} value={engines.speedtest_bin}
              onChange={(e) => setEngines({ ...engines, speedtest_bin: e.target.value })} />
          </div>
          <div className={fieldClass}>
            <label htmlFor="engines-iperf3-bin" className={labelClass}>iperf3 binary path</label>
            <input id="engines-iperf3-bin" className={inputClass} value={engines.iperf3_bin}
              onChange={(e) => setEngines({ ...engines, iperf3_bin: e.target.value })} />
          </div>
          <label className="flex items-center gap-2 text-sm text-muted">
            <input type="checkbox" checked={engines.ookla_accept_license}
              onChange={(e) => setEngines({ ...engines, ookla_accept_license: e.target.checked })} />
            Accept Ookla license
          </label>
          <label className="flex items-center gap-2 text-sm text-muted">
            <input type="checkbox" checked={engines.ookla_accept_gdpr}
              onChange={(e) => setEngines({ ...engines, ookla_accept_gdpr: e.target.checked })} />
            Accept Ookla GDPR terms
          </label>
          <div className={fieldClass}>
            <label htmlFor="engines-ttl" className={labelClass}>Server list TTL (seconds)</label>
            <input id="engines-ttl" type="number" min={1} className={inputClass}
              value={engines.server_list_ttl_seconds}
              onChange={(e) => setEngines({ ...engines, server_list_ttl_seconds: Number(e.target.value) })} />
          </div>
        </Section>

        <Section
          id="integrations-heading" title="Integrations" saving={update.isPending}
          error={errors.integrations} saved={saved.integrations}
          onSave={() => save('integrations', { integrations })}
        >
          <label className="flex items-center gap-2 text-sm text-muted">
            <input type="checkbox" checked={integrations.vm_enabled}
              onChange={(e) => setIntegrations({ ...integrations, vm_enabled: e.target.checked })} />
            Enable VictoriaMetrics
          </label>
          <div className={fieldClass}>
            <label htmlFor="vm-url" className={labelClass}>VictoriaMetrics URL</label>
            <input id="vm-url" className={inputClass} value={integrations.vm_url}
              onChange={(e) => setIntegrations({ ...integrations, vm_url: e.target.value })} />
          </div>
          <div className={fieldClass}>
            <label htmlFor="vm-auth" className={labelClass}>VictoriaMetrics auth header</label>
            <input id="vm-auth" type="password" placeholder="leave unchanged" className={inputClass}
              value={integrations.vm_auth_header}
              onChange={(e) => setIntegrations({ ...integrations, vm_auth_header: e.target.value })} />
          </div>
          <LabelsEditor
            label="VictoriaMetrics extra labels"
            value={integrations.vm_extra_labels}
            onChange={(v) => setIntegrations({ ...integrations, vm_extra_labels: v })}
          />
          <div className="flex items-center gap-3">
            <button type="button" className={buttonClass} disabled={test.isPending}
              onClick={() => runTest('vm', integrations.vm_url, integrations.vm_auth_header, setVmResult)}>
              Test VictoriaMetrics
            </button>
            {vmResult && (
              <p className={vmResult.ok ? 'text-sm text-ok' : 'text-sm text-bad'}>{vmResult.message}</p>
            )}
          </div>

          <label className="flex items-center gap-2 text-sm text-muted">
            <input type="checkbox" checked={integrations.vl_enabled}
              onChange={(e) => setIntegrations({ ...integrations, vl_enabled: e.target.checked })} />
            Enable VictoriaLogs
          </label>
          <div className={fieldClass}>
            <label htmlFor="vl-url" className={labelClass}>VictoriaLogs URL</label>
            <input id="vl-url" className={inputClass} value={integrations.vl_url}
              onChange={(e) => setIntegrations({ ...integrations, vl_url: e.target.value })} />
          </div>
          <div className={fieldClass}>
            <label htmlFor="vl-auth" className={labelClass}>VictoriaLogs auth header</label>
            <input id="vl-auth" type="password" placeholder="leave unchanged" className={inputClass}
              value={integrations.vl_auth_header}
              onChange={(e) => setIntegrations({ ...integrations, vl_auth_header: e.target.value })} />
          </div>
          <LabelsEditor
            label="Extra stream fields"
            value={integrations.vl_stream_fields}
            onChange={(v) => setIntegrations({ ...integrations, vl_stream_fields: v })}
          />
          <div className="flex items-center gap-3">
            <button type="button" className={buttonClass} disabled={test.isPending}
              onClick={() => runTest('vl', integrations.vl_url, integrations.vl_auth_header, setVlResult)}>
              Test VictoriaLogs
            </button>
            {vlResult && (
              <p className={vlResult.ok ? 'text-sm text-ok' : 'text-sm text-bad'}>{vlResult.message}</p>
            )}
          </div>

          <label className="flex items-center gap-2 text-sm text-muted">
            <input type="checkbox" checked={integrations.metrics_enabled}
              onChange={(e) => setIntegrations({ ...integrations, metrics_enabled: e.target.checked })} />
            Enable /metrics endpoint
          </label>
          <p className="text-sm text-faint">/metrics answers 404 while disabled</p>
        </Section>

        <Section
          id="notifications-heading" title="Notifications" saving={update.isPending}
          error={errors.notifications} saved={saved.notifications}
          onSave={saveNotifications}
        >
          <label className="flex items-center gap-2 text-sm text-muted">
            <input type="checkbox" checked={notifications.enabled}
              onChange={(e) => setNotifications({ ...notifications, enabled: e.target.checked })} />
            Enabled
          </label>
          <p className="text-sm text-faint">Nothing is delivered while this is off.</p>

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
                unsaved={isChannelUnsaved(channel, settings.data?.notifications.channels)}
              />
            ))}
            <button type="button" className={buttonClass} onClick={addChannel}>Add channel</button>
          </div>

          <div className={fieldClass}>
            <label htmlFor="notifications-cooldown" className={labelClass}>Cooldown (minutes)</label>
            <input id="notifications-cooldown" type="number" min={1} className={inputClass}
              value={notifications.cooldown_minutes}
              onChange={(e) => setNotifications({ ...notifications, cooldown_minutes: Number(e.target.value) })} />
          </div>
          <div className={fieldClass}>
            <label htmlFor="notifications-quiet-start" className={labelClass}>Quiet hours start</label>
            <input id="notifications-quiet-start" type="time" className={inputClass}
              value={notifications.quiet_hours_start}
              onChange={(e) => setNotifications({ ...notifications, quiet_hours_start: e.target.value })} />
          </div>
          <div className={fieldClass}>
            <label htmlFor="notifications-quiet-end" className={labelClass}>Quiet hours end</label>
            <input id="notifications-quiet-end" type="time" className={inputClass}
              value={notifications.quiet_hours_end}
              onChange={(e) => setNotifications({ ...notifications, quiet_hours_end: e.target.value })} />
          </div>
          <label className="flex items-center gap-2 text-sm text-muted">
            <input type="checkbox" checked={notifications.notify_recovery}
              onChange={(e) => setNotifications({ ...notifications, notify_recovery: e.target.checked })} />
            Send recovery notifications
          </label>

          <div className="grid gap-2">
            <h3 className="text-sm font-semibold text-fg">Default thresholds</h3>
            <p className="text-sm text-faint">Targets can override any of these in the target form.</p>
            <ThresholdFields
              value={notifications.default_thresholds}
              onChange={(next) => setNotifications({ ...notifications, default_thresholds: next })}
            />
          </div>
        </Section>
      </div>
    </div>
  );
}
