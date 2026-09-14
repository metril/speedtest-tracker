import { useEffect, useRef, useState } from 'react';
import { NavLink, Outlet, useLocation } from 'react-router';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import type { SectionKey, SettingsOutletContext, TestResult } from '../features/settings/settingsContext';
import { stripIrrelevantChannelFields } from '../features/settings/channelHelpers';
import { validateAuthSettings } from '../features/settings/AuthSection';
import { validateThresholds } from '../features/targets/ThresholdFields';
import type {
  AuthSettings, EngineSettings, GeneralSettings, IntegrationSettings, NotificationSettings, NotifyChannel,
} from '../lib/api';
import { ApiError } from '../lib/api';
import {
  useSettings, useTestIntegration, useTestNotifyChannel, useUpdateSettings,
} from '../lib/queries';

const TABS: { key: SectionKey; to: string; label: string }[] = [
  { key: 'general', to: 'general', label: 'General' },
  { key: 'engines', to: 'engines', label: 'Engines' },
  { key: 'integrations', to: 'integrations', label: 'Integrations' },
  { key: 'notifications', to: 'notifications', label: 'Notifications' },
  { key: 'auth', to: 'auth', label: 'Auth' },
];

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

/** Settings is the shell for /settings/*: a header, a sub-nav (a left
 * column on md+, a Tabs strip below it), and an <Outlet/> for the active
 * tab's content. It owns every section's local edit state, the fetch, the
 * seed-once-from-server effect and the save/error/saved-flash cycle, and
 * hands all of it down through useOutletContext so switching tabs never
 * loses an in-progress, unsaved edit in another section. */
export function Settings() {
  const location = useLocation();
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

  const save = (section: SectionKey, patch: Record<string, unknown>) => {
    setErrors((e) => ({ ...e, [section]: undefined }));
    update.mutate(patch, {
      onSuccess: () => flash(section),
      onError: (err) => setErrors((e) => ({ ...e, [section]: message(err) })),
    });
  };

  const runTest = (target: 'vm' | 'vl', url: string, authHeader: string) => {
    const setResult = target === 'vm' ? setVmResult : setVlResult;
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
    const normalized: AuthSettings = {
      ...auth,
      trusted_proxies: auth.trusted_proxies.map((l) => l.trim()).filter(Boolean),
    };
    const authError = validateAuthSettings(normalized);
    if (authError) {
      setErrors((e) => ({ ...e, auth: authError }));
      return;
    }
    save('auth', { auth: normalized });
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

  const activeTab = TABS.find((t) => location.pathname.includes(`/settings/${t.to}`))?.key ?? 'general';

  const context: SettingsOutletContext = {
    locked: settings.data?.locked ?? [],
    saving: update.isPending,
    errors,
    saved,
    save,
    general, setGeneral,
    engines, setEngines,
    integrations, setIntegrations,
    testPending: test.isPending,
    vmResult, vlResult, runTest,
    notifications, setNotifications,
    savedChannels: settings.data?.notifications.channels,
    addChannel, updateChannel, removeChannel, runChannelTest, channelResults, testingChannelId, saveNotifications,
    auth, setAuth, saveAuth,
  };

  return (
    <div className="grid gap-4">
      <h1 className="text-xl font-semibold tracking-tight">Settings</h1>

      <Tabs value={activeTab} onValueChange={() => {}} className="md:hidden">
        <TabsList className="w-full justify-start overflow-x-auto">
          {TABS.map((tab) => (
            <TabsTrigger key={tab.key} value={tab.key} asChild>
              <NavLink to={`/settings/${tab.to}`}>{tab.label}</NavLink>
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>

      <div className="grid gap-6 md:grid-cols-[12rem_1fr]">
        <nav className="hidden md:block" aria-label="Settings sections">
          <ul className="grid gap-1">
            {TABS.map((tab) => (
              <li key={tab.key}>
                <NavLink
                  to={`/settings/${tab.to}`}
                  className={({ isActive }) =>
                    `block rounded-md px-3 py-2 text-sm font-medium transition-colors ${
                      isActive ? 'bg-raised text-accent' : 'text-muted hover:bg-raised hover:text-fg'
                    }`
                  }
                >
                  {tab.label}
                </NavLink>
              </li>
            ))}
          </ul>
        </nav>

        <div className="min-w-0">
          <Outlet context={context} />
        </div>
      </div>
    </div>
  );
}
