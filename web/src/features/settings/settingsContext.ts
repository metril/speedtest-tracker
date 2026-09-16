import type {
  AuthSettings, EngineSettings, ExportAuth, GeneralSettings, IntegrationSettings, NotificationSettings,
  NotifyChannel,
} from '../../lib/api';

/** SectionKey names the five settings tabs. Two of them (exporters,
 * access) are UI-only renames: the API request/response bodies still use
 * the older `integrations`/`auth` keys, mapped at the save boundary in
 * pages/Settings.tsx. */
export type SectionKey = 'general' | 'engines' | 'exporters' | 'notifications' | 'access';

export interface TestResult {
  ok: boolean;
  message: string;
}

/** A field-level validation error, keyed by the htmlFor id of the
 * offending SettingsRow so the section can render it inline in addition
 * to the save bar's summary message. */
export interface FieldError {
  field: string;
  message: string;
}

/** SettingsOutletContext is the bundle the Settings shell (pages/Settings.tsx)
 * owns and hands down to each tab's <Outlet/> content via React Router's
 * useOutletContext, so all five sections share one fetch, one seed-once
 * effect and one save/discard/error/saved-flash cycle regardless of which
 * tab is currently mounted. */
export interface SettingsOutletContext {
  locked: string[];
  /** readOnly is true for a signed-in, non-admin viewer (oidc/forward_auth
   * with a groups-based admin check): every Save and connection-Test
   * action is disabled, with a hint explaining why. */
  readOnly: boolean;
  saving: boolean;
  errors: Partial<Record<SectionKey, string>>;
  fieldErrors: Partial<Record<SectionKey, FieldError>>;
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
  runTest: (target: 'vm' | 'vl', url: string, auth: ExportAuth) => void;

  notifications: NotificationSettings;
  setNotifications: (next: NotificationSettings) => void;
  savedChannels?: NotifyChannel[];
  addChannel: () => void;
  updateChannel: (id: string, next: NotifyChannel) => void;
  removeChannel: (id: string) => void;
  runChannelTest: (channel: NotifyChannel) => void;
  channelResults: Record<string, TestResult>;
  testingChannelId: string | null;

  auth: AuthSettings;
  setAuth: (next: AuthSettings) => void;
}
