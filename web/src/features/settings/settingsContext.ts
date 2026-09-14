import type {
  AuthSettings, EngineSettings, GeneralSettings, IntegrationSettings, NotificationSettings, NotifyChannel,
} from '../../lib/api';

export type SectionKey = 'general' | 'engines' | 'integrations' | 'notifications' | 'auth';

export interface TestResult {
  ok: boolean;
  message: string;
}

/** SettingsOutletContext is the bundle the Settings shell (pages/Settings.tsx)
 * owns and hands down to each tab's <Outlet/> content via React Router's
 * useOutletContext, so all five sections share one fetch, one seed-once
 * effect and one save/error/saved-flash cycle regardless of which tab is
 * currently mounted. */
export interface SettingsOutletContext {
  locked: string[];
  saving: boolean;
  errors: Partial<Record<SectionKey, string>>;
  saved: Record<SectionKey, boolean>;
  save: (key: SectionKey, patch: Record<string, unknown>) => void;

  general: GeneralSettings;
  setGeneral: (next: GeneralSettings) => void;

  engines: EngineSettings;
  setEngines: (next: EngineSettings) => void;

  integrations: IntegrationSettings;
  setIntegrations: (next: IntegrationSettings) => void;
  testPending: boolean;
  vmResult: TestResult | null;
  vlResult: TestResult | null;
  runTest: (target: 'vm' | 'vl', url: string, authHeader: string) => void;

  notifications: NotificationSettings;
  setNotifications: (next: NotificationSettings) => void;
  savedChannels?: NotifyChannel[];
  addChannel: () => void;
  updateChannel: (id: string, next: NotifyChannel) => void;
  removeChannel: (id: string) => void;
  runChannelTest: (channel: NotifyChannel) => void;
  channelResults: Record<string, TestResult>;
  testingChannelId: string | null;
  saveNotifications: () => void;

  auth: AuthSettings;
  setAuth: (next: AuthSettings) => void;
  saveAuth: () => void;
}
