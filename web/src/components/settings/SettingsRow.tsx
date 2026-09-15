import { createContext, useContext, useMemo, type ReactNode } from 'react';
import { Label } from '@/components/ui/label';
import { LockedBadge } from '@/features/settings/LockedBadge';
import { isLocked, useSettingsForm } from './settingsFormContext';

interface RowFieldValue {
  id: string;
  disabled: boolean;
  'aria-invalid': boolean | undefined;
  'aria-describedby': string | undefined;
}

const RowFieldContext = createContext<RowFieldValue | null>(null);

/** useSettingsRowField spreads onto a row's control so it picks up the
 * row's id, disabled state (read-only form or a locked key) and error
 * wiring without the control needing to know about SettingsFormProvider
 * itself. Must be used inside a SettingsRow. */
export function useSettingsRowField(): RowFieldValue {
  const ctx = useContext(RowFieldContext);
  if (!ctx) throw new Error('useSettingsRowField must be used within a SettingsRow');
  return ctx;
}

interface Props {
  label: string;
  description?: ReactNode;
  htmlFor: string;
  lockKey?: string;
  error?: string;
  size?: 'sm';
  children: ReactNode;
}

/** SettingsRow is the label/control grid row shared by every field in a
 * redesigned settings card: label + description on the left, the control
 * on the right, error text below it. `lockKey` shows a LockedBadge and
 * disables the control (via useSettingsRowField) when that key is in the
 * form's locked list. */
export function SettingsRow({ label, description, htmlFor, lockKey, error, size, children }: Props) {
  const { readOnly, locked } = useSettingsForm();
  const locked_ = isLocked(locked, lockKey);
  const errorId = error ? `${htmlFor}-error` : undefined;

  const fieldValue = useMemo<RowFieldValue>(
    () => ({
      id: htmlFor,
      disabled: readOnly || locked_,
      'aria-invalid': error ? true : undefined,
      'aria-describedby': errorId,
    }),
    [htmlFor, readOnly, locked_, error, errorId],
  );

  return (
    <div className="grid gap-y-1 px-4 py-3 md:grid-cols-[minmax(0,2fr)_minmax(0,3fr)] md:gap-x-6 md:items-start">
      <div>
        <Label htmlFor={htmlFor} className="text-sm font-medium text-fg">
          {label}
          {locked_ && (
            <>
              {' '}
              <LockedBadge />
            </>
          )}
        </Label>
        {description && <p className="text-sm text-faint">{description}</p>}
      </div>
      <div className={size === 'sm' ? 'min-w-0 md:max-w-[12rem]' : 'min-w-0'}>
        <RowFieldContext.Provider value={fieldValue}>{children}</RowFieldContext.Provider>
        {error && <p id={errorId} className="text-sm text-bad">{error}</p>}
      </div>
    </div>
  );
}
