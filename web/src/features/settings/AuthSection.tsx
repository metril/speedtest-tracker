/** AuthSection's fields moved into AccessSection as SettingsCards; this
 * file survives only to re-export validateAuthSettings, which
 * pages/Settings.tsx still imports by this path. */
export { validateAuthSettings } from './AccessSection';
