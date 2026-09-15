import { createContext, useContext, useMemo, type ReactNode } from 'react';

interface SettingsFormValue {
  readOnly: boolean;
  locked: string[];
}

const SettingsFormContext = createContext<SettingsFormValue>({ readOnly: false, locked: [] });

interface Props {
  readOnly?: boolean;
  locked?: string[];
  children: ReactNode;
}

/** SettingsFormProvider carries the two things every field in a settings
 * form needs but doesn't want threaded through props: whether the whole
 * form is read-only (non-admin viewer) and which keys are pinned by an
 * ST_ environment variable while ST_LOCK_ENV is on. */
export function SettingsFormProvider({ readOnly = false, locked = [], children }: Props) {
  const value = useMemo(() => ({ readOnly, locked }), [readOnly, locked]);
  return <SettingsFormContext.Provider value={value}>{children}</SettingsFormContext.Provider>;
}

export function useSettingsForm(): SettingsFormValue {
  return useContext(SettingsFormContext);
}

export function isLocked(locked: string[], key: string | undefined): boolean {
  return key !== undefined && locked.includes(key);
}
