import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigationGuard } from './NavigationGuardContext';

/** useUnsavedGuard protects an unsaved edit in the Settings shell from
 * being lost three ways: a browser close/reload (`beforeunload`), an
 * in-page tab switch, and a sidebar navigation click (via
 * NavigationGuardContext, registered for as long as this hook is
 * mounted). `anyDirty` gates the beforeunload prompt (any section);
 * `navDirty` gates the in-app confirm dialog (normally just the active
 * section, since that's the one the confirm copy names). `discard` reset
 * that section back to its last-saved state.
 *
 * Callers drive navigation through `requestNavigate(action)`: `action`
 * runs immediately when clean, or is held until the dialog is resolved
 * when dirty. */
export function useUnsavedGuard(anyDirty: boolean, navDirty: boolean, discard: () => void) {
  const [confirmOpen, setConfirmOpen] = useState(false);
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
    pending.current = action;
    setConfirmOpen(true);
  }, [navDirty]);

  useEffect(() => {
    register(requestNavigate);
    return () => register(null);
  }, [register, requestNavigate]);

  const confirmDiscard = useCallback(() => {
    discard();
    setConfirmOpen(false);
    const action = pending.current;
    pending.current = null;
    action?.();
  }, [discard]);

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

  return { confirmOpen, requestNavigate, confirmDiscard, cancel };
}
