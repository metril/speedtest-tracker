/** LockedBadge marks a settings field whose value is pinned by an ST_
 * environment variable while ST_LOCK_ENV is on. It never disables anything
 * itself -- the caller does that -- it only explains why. */
export function LockedBadge() {
  return (
    <span
      className="rounded-full border border-line px-2 py-0.5 text-xs text-faint"
      title="Set by an ST_ environment variable. ST_LOCK_ENV is on, so this field cannot be changed here."
    >
      set by environment
    </span>
  );
}
