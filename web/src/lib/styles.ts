/** Canonical field styling shared by every native <select>/<textarea>
 * across the app (ui/Input and ui/Button cover their own styling; this
 * is for the plain HTML elements that don't go through those
 * components). Re-exported from features/settings/styles for existing
 * callers there. */
export const inputClass = 'flex h-9 w-full rounded-md border border-line-strong bg-surface px-3 py-1 text-sm text-fg shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50';
