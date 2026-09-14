/** Shared field/button styling for the Settings page and its section
 * editors (ChannelEditor). Kept in one place so ChannelEditor does not
 * import from the Settings page module (avoiding a circular import). */
export const inputClass = 'flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm text-fg shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50';
export const labelClass = 'text-sm text-muted';
export const fieldClass = 'grid gap-1';
export const buttonClass = 'inline-flex h-9 items-center justify-center gap-2 whitespace-nowrap rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground shadow transition-colors hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50';
