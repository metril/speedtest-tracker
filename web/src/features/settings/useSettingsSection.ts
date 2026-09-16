import { useOutletContext } from 'react-router';
import type { SectionKey, SettingsOutletContext } from './settingsContext';

/** useSettingsSection reads the shared Settings shell context (fetched
 * settings, local edit state, save/discard/error cycle) and slices out
 * the error/fieldError/saved trio for one section key, so each tab's
 * content component doesn't have to. */
export function useSettingsSection(key: SectionKey) {
  const ctx = useOutletContext<SettingsOutletContext>();
  return {
    ...ctx,
    error: ctx.errors[key],
    fieldError: ctx.fieldErrors[key],
    saved: ctx.saved[key],
  };
}
