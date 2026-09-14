/** Shared field/button styling for the Settings page and its section
 * editors (ChannelEditor). Kept in one place so ChannelEditor does not
 * import from the Settings page module (avoiding a circular import). */
export const inputClass = 'rounded border border-line bg-surface px-2 py-1 text-fg';
export const labelClass = 'text-sm text-muted';
export const fieldClass = 'grid gap-1';
export const buttonClass = 'rounded bg-accent px-3 py-1.5 text-accent-fg disabled:opacity-50';
