/** Shared field styling for the Settings page and its section editors
 * (ChannelEditor, AuthSection): native <select>/<textarea> elements,
 * which ui/Button and ui/Input don't cover. Kept in one place so
 * ChannelEditor does not import from the Settings page module (avoiding
 * a circular import). Re-exported from the app-wide lib/styles so every
 * native form control (not just Settings') shares one definition. */
export { inputClass } from '../../lib/styles';
