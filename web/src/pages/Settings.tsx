import { useEffect, useRef, useState } from 'react';
import { Outlet, useLocation, useNavigate } from 'react-router';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { SettingsFormProvider, SettingsSaveBar } from '../components/settings';
import { Tabs, TabsList, TabsTrigger } from '../components/ui/tabs';
import type { FieldError, SectionKey, SettingsOutletContext, TestResult } from '../features/settings/settingsContext';
import { stripIrrelevantChannelFields } from '../features/settings/channelHelpers';
import { validateAuthSettings } from '../features/settings/AccessSection';
import { validateThresholds } from '../features/targets/ThresholdFields';
import { useUnsavedGuard } from '../features/settings/useUnsavedGuard';
import type {
  AuthSettings, EngineSettings, ExportAuth, GeneralSettings, IntegrationSettings, NotificationSettings,
  NotifyChannel, Settings as SettingsData,
} from '../lib/api';
import { ApiError, MASKED_SECRET } from '../lib/api';
import { newId } from '../lib/id';
import {
  useMe, useSettings, useTestIntegration, useTestNotifyChannel, useUpdateSettings,
} from '../lib/queries';

const TABS: { key: SectionKey; to: string; label: string }[] = [
  { key: 'general', to: 'general', label: 'General' },
  { key: 'engines', to: 'engines', label: 'Engines' },
  { key: 'exporters', to: 'exporters', label: 'Exporters' },
  { key: 'notifications', to: 'notifications', label: 'Notifications' },
  { key: 'access', to: 'access', label: 'Access' },
];

/** equalOrMasked treats an untouched secret field as unchanged: the
 * server never echoes a stored secret back, so a masked server value
 * (MASKED_SECRET) and a still-blank draft field mean the same thing. */
function equalOrMasked(a: unknown, b: unknown): boolean {
  if (typeof a === 'string' && typeof b === 'string') {
    if (a === '' && b === MASKED_SECRET) return true;
    if (b === '' && a === MASKED_SECRET) return true;
  }
  return false;
}

/** deepEqual compares two settings values structurally, with
 * equalOrMasked's secret-field exception applied at every leaf. */
function deepEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (Array.isArray(a) && Array.isArray(b)) {
    return a.length === b.length && a.every((v, i) => deepEqual(v, b[i]));
  }
  if (a && b && typeof a === 'object' && typeof b === 'object') {
    const keys = new Set([...Object.keys(a as object), ...Object.keys(b as object)]);
    return [...keys].every((k) => deepEqual((a as Record<string, unknown>)[k], (b as Record<string, unknown>)[k]));
  }
  return equalOrMasked(a, b);
}

/** useSavedFlash shows a "Saved" message for a few seconds after a
 * successful save, per section. */
function useSavedFlash() {
  const [saved, setSaved] = useState<Record<SectionKey, boolean>>({
    general: false, engines: false, exporters: false, notifications: false, access: false,
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

/** Settings is the shell for /settings/*: a header, a sub-nav (a left
 * column on md+, a Tabs strip below it), and an <Outlet/> for the active
 * tab's content, followed by a sticky save bar for whichever section is
 * dirty. It owns every section's local edit state, the fetch, the
 * seed-once-from-server effect, dirty tracking against the latest server
 * data, and the save/discard/error/saved-flash cycle, and hands all of
 * it down through useOutletContext so switching tabs never loses an
 * in-progress, unsaved edit in another section. */
export function Settings() {
  const location = useLocation();
  const navigate = useNavigate();
  const settings = useSettings();
  const me = useMe();
  const update = useUpdateSettings();
  const test = useTestIntegration();
  const testChannel = useTestNotifyChannel();

  const [general, setGeneral] = useState<GeneralSettings | null>(null);
  const [engines, setEngines] = useState<EngineSettings | null>(null);
  const [integrations, setIntegrations] = useState<IntegrationSettings | null>(null);
  const [notifications, setNotifications] = useState<NotificationSettings | null>(null);
  const [auth, setAuth] = useState<AuthSettings | null>(null);
  const [errors, setErrors] = useState<Partial<Record<SectionKey, string>>>({});
  const [fieldErrors, setFieldErrors] = useState<Partial<Record<SectionKey, FieldError>>>({});
  const { saved, flash } = useSavedFlash();
  const [vmResult, setVmResult] = useState<TestResult | null>(null);
  const [vlResult, setVlResult] = useState<TestResult | null>(null);
  const [channelResults, setChannelResults] = useState<Record<string, TestResult>>({});
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

  // draftFor/serverFor back the dirty-tracking below: `exporters` and
  // `access` are UI-only names for the API's `integrations`/`auth`.
  const draftFor = (section: SectionKey): Record<string, unknown> | null => {
    switch (section) {
      case 'general': return general as unknown as Record<string, unknown>;
      case 'engines': return engines as unknown as Record<string, unknown>;
      case 'exporters': return integrations as unknown as Record<string, unknown>;
      case 'notifications': return notifications as unknown as Record<string, unknown>;
      case 'access': return auth as unknown as Record<string, unknown>;
      default: return null;
    }
  };
  const serverFor = (section: SectionKey): Record<string, unknown> | null => {
    if (!settings.data) return null;
    switch (section) {
      case 'general': return settings.data.general as unknown as Record<string, unknown>;
      case 'engines': return settings.data.engines as unknown as Record<string, unknown>;
      case 'exporters': return settings.data.integrations as unknown as Record<string, unknown>;
      case 'notifications': return settings.data.notifications as unknown as Record<string, unknown>;
      case 'access': return settings.data.auth as unknown as Record<string, unknown>;
      default: return null;
    }
  };
  const changedKeys = (section: SectionKey): string[] => {
    const d = draftFor(section);
    const s = serverFor(section);
    if (!d || !s) return [];
    const keys = new Set([...Object.keys(d), ...Object.keys(s)]);
    return [...keys].filter((k) => !deepEqual(d[k], s[k]));
  };

  const discard = (section: SectionKey) => {
    if (!settings.data) return;
    switch (section) {
      case 'general': setGeneral(settings.data.general); break;
      case 'engines': setEngines(settings.data.engines); break;
      case 'exporters': setIntegrations(settings.data.integrations); break;
      case 'notifications': setNotifications(settings.data.notifications); break;
      case 'access': setAuth(settings.data.auth); break;
      default: break;
    }
    setErrors((e) => ({ ...e, [section]: undefined }));
    setFieldErrors((e) => ({ ...e, [section]: undefined }));
  };

  const save = (section: SectionKey, patch: Record<string, unknown>) => {
    setErrors((e) => ({ ...e, [section]: undefined }));
    setFieldErrors((e) => ({ ...e, [section]: undefined }));
    update.mutate(patch, {
      onSuccess: (result: SettingsData) => {
        flash(section);
        // Re-seed the saved section's draft from the mutation result (not
        // a later refetch) so its dirty count clears exactly once, right
        // away -- including any secret field the server just re-masked.
        switch (section) {
          case 'general': setGeneral(result.general); break;
          case 'engines': setEngines(result.engines); break;
          case 'exporters': setIntegrations(result.integrations); break;
          case 'notifications': setNotifications(result.notifications); break;
          case 'access': setAuth(result.auth); break;
          default: break;
        }
      },
      onError: (err) => setErrors((e) => ({ ...e, [section]: message(err) })),
    });
  };

  const runTest = (target: 'vm' | 'vl', url: string, auth_: ExportAuth) => {
    const setResult = target === 'vm' ? setVmResult : setVlResult;
    setResult(null);
    test.mutate(
      { target, body: { url, auth: auth_ } },
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
      id: newId(8), type: 'webhook', name: 'New channel', enabled: true, url: '',
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

  const commitAccess = () => {
    if (!auth) return;
    const normalized: AuthSettings = {
      ...auth,
      trusted_proxies: auth.trusted_proxies.map((l) => l.trim()).filter(Boolean),
    };
    const authError = validateAuthSettings(normalized);
    if (authError) {
      setErrors((e) => ({ ...e, access: authError.message }));
      setFieldErrors((e) => ({ ...e, access: authError }));
      return;
    }
    save('access', { auth: normalized });
  };

  const commitNotifications = () => {
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

  const commit: Partial<Record<SectionKey, () => void>> = {
    general: () => general && save('general', { general }),
    engines: () => engines && save('engines', { engines }),
    exporters: () => integrations && save('exporters', { integrations }),
    notifications: commitNotifications,
    access: commitAccess,
  };

  const active = TABS.find((tab) => location.pathname.endsWith(`/${tab.to}`))?.key ?? TABS[0].key;
  const activeLabel = TABS.find((tab) => tab.key === active)?.label ?? '';
  const activeDirtyKeys = settings.data ? changedKeys(active) : [];
  const anyDirty = settings.data ? TABS.some((tab) => changedKeys(tab.key).length > 0) : false;

  const { confirmOpen, confirmKind, requestNavigate, confirmDiscard, cancel } = useUnsavedGuard(
    anyDirty,
    activeDirtyKeys.length > 0,
    () => discard(active),
    () => TABS.forEach((tab) => discard(tab.key)),
  );

  if (settings.isLoading || !general || !engines || !integrations || !notifications || !auth) {
    return <h1 className="text-xl font-semibold tracking-tight">Settings</h1>;
  }

  const readOnly = me.data ? !me.data.is_admin : false;
  const locked = settings.data?.locked ?? [];

  const context: SettingsOutletContext = {
    locked,
    readOnly,
    saving: update.isPending,
    errors,
    fieldErrors,
    saved,
    save,
    general, setGeneral,
    engines, setEngines,
    integrations, setIntegrations,
    testPending: test.isPending,
    vmResult, vlResult, runTest,
    notifications, setNotifications,
    savedChannels: settings.data?.notifications.channels,
    addChannel, updateChannel, removeChannel, runChannelTest, channelResults, testingChannelId,
    auth, setAuth,
  };

  return (
    <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] gap-4">
      <div className="mx-auto grid w-full min-w-0 max-w-[880px] grid-cols-[minmax(0,1fr)] gap-4">
        <h1 className="text-xl font-semibold tracking-tight">Settings</h1>

        <Tabs
          value={active}
          onValueChange={(key) => requestNavigate(() => navigate(`/settings/${key}`, { replace: true }))}
        >
          <div className="min-w-0 overflow-x-auto">
            <TabsList className="w-max min-w-full justify-start">
              {TABS.map((tab) => (
                <TabsTrigger key={tab.key} value={tab.key} aria-controls={undefined}>{tab.label}</TabsTrigger>
              ))}
            </TabsList>
          </div>
        </Tabs>

        <SettingsFormProvider readOnly={readOnly} locked={locked}>
          <div className="grid gap-4">
            <Outlet context={context} />
          </div>

          <SettingsSaveBar
            dirtyCount={activeDirtyKeys.length}
            saving={update.isPending}
            onSave={() => commit[active]?.()}
            onDiscard={() => discard(active)}
            error={errors[active]}
            readOnly={readOnly}
            savedFlash={saved[active]}
          />
        </SettingsFormProvider>
      </div>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={(open) => { if (!open) cancel(); }}
        title={confirmKind === 'leave' ? 'Leave Settings?' : 'Discard unsaved changes?'}
        description={confirmKind === 'leave'
          ? 'Your unsaved settings changes will be lost.'
          : `Your edits to ${activeLabel} have not been saved.`}
        confirmLabel="Discard"
        cancelLabel="Keep editing"
        onConfirm={confirmDiscard}
      />
    </div>
  );
}
