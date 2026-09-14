/** Shared field styling for the Settings page and its section editors
 * (ChannelEditor, AuthSection): native <select>/<textarea> elements
 * (which ui/Button and ui/Input don't cover) plus the field/label
 * wrapper classes. Kept in one place so ChannelEditor does not import
 * from the Settings page module (avoiding a circular import). Buttons
 * and text/number/password/time inputs use ui/Button and ui/Input
 * directly instead. */
export const inputClass = 'flex h-9 w-full rounded-md border border-line-strong bg-surface px-3 py-1 text-sm text-fg shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50';
export const labelClass = 'text-sm text-muted';
export const fieldClass = 'grid gap-1';
