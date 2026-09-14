import { useOutletContext } from 'react-router';
import type { SectionKey, SettingsOutletContext } from './settingsContext';

/** useSettingsSection reads the shared Settings shell context (fetched
 * settings, local edit state, save/error/saved-flash cycle) and slices
 * out the saving/error/saved trio for one section key, so each tab's
 * content component doesn't have to. */
export function useSettingsSection(key: SectionKey) {
  const ctx = useOutletContext<SettingsOutletContext>();
  return {
    ...ctx,
    error: ctx.errors[key],
    saved: ctx.saved[key],
  };
}
