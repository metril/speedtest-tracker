import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigationGuard } from './NavigationGuardContext';

/** useUnsavedGuard protects an unsaved edit in the Settings shell from
 * being lost three ways: a browser close/reload (`beforeunload`), an
 * in-page tab switch, and a sidebar navigation click (via
 * NavigationGuardContext, registered for as long as this hook is
 * mounted).
 *
 * The tab-switch path (`requestNavigate`) and the sidebar path (guarded
 * externally, through the registered function) use different gates and
 * different recovery: a tab switch is only blocked -- and only discards
 * -- the active section (`navDirty`/`discardActive`), naming that
 * section in the confirm copy, while leaving Settings entirely is
 * blocked by *any* dirty section (`anyDirty`) and discards all of them
 * (`discardAll`) on confirm. `confirmKind` tells the caller which of the
 * two triggered the open dialog, so it can pick the right copy. */
export function useUnsavedGuard(
  anyDirty: boolean,
  navDirty: boolean,
  discardActive: () => void,
  discardAll: () => void = discardActive,
) {
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [confirmKind, setConfirmKind] = useState<'tab' | 'leave'>('tab');
  const pending = useRef<(() => void) | null>(null);
  const { register } = useNavigationGuard();

  useEffect(() => {
    const handler = (e: BeforeUnloadEvent) => {
      if (!anyDirty) return;
      e.preventDefault();
      e.returnValue = '';
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  }, [anyDirty]);

  const requestNavigate = useCallback((action: () => void) => {
    if (!navDirty) {
      action();
      return;
    }
    setConfirmKind('tab');
    pending.current = action;
    setConfirmOpen(true);
  }, [navDirty]);

  const requestLeave = useCallback((action: () => void) => {
    if (!anyDirty) {
      action();
      return;
    }
    setConfirmKind('leave');
    pending.current = action;
    setConfirmOpen(true);
  }, [anyDirty]);

  useEffect(() => {
    register(requestLeave);
    return () => register(null);
  }, [register, requestLeave]);

  const confirmDiscard = useCallback(() => {
    if (confirmKind === 'leave') discardAll(); else discardActive();
    setConfirmOpen(false);
    const action = pending.current;
    pending.current = null;
    action?.();
  }, [confirmKind, discardActive, discardAll]);

  // Only hides the dialog. It deliberately leaves `pending` alone: the
  // dialog's Discard button calls onOpenChange(false) (closing the
  // dialog) immediately before onConfirm (confirmDiscard) runs, in the
  // same click handler, so clearing `pending` here would race it away
  // before confirmDiscard can read it. A stale `pending` after a real
  // cancel is harmless -- the next requestNavigate always overwrites it
  // before confirmOpen can become true again.
  const cancel = useCallback(() => {
    setConfirmOpen(false);
  }, []);

  return { confirmOpen, confirmKind, requestNavigate, confirmDiscard, cancel };
}
