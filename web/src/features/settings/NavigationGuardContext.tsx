import {
  createContext, useCallback, useContext, useMemo, useRef, type ReactNode,
} from 'react';

type GuardFn = (action: () => void) => void;

interface NavigationGuardValue {
  /** register installs (or, with null, clears) the one active guard. At
   * most one part of the app guards navigation at a time -- currently
   * only the Settings shell, while a section has unsaved edits. */
  register: (fn: GuardFn | null) => void;
  /** guard runs `action` immediately when no guard is registered (or
   * the registered guard decides to let it through), otherwise defers to
   * the registered guard (e.g. to show a confirm dialog first). */
  guard: GuardFn;
}

const defaultValue: NavigationGuardValue = { register: () => {}, guard: (action) => action() };

const NavigationGuardContext = createContext<NavigationGuardValue>(defaultValue);

/** NavigationGuardProvider lets the Settings shell intercept navigation
 * triggered elsewhere in the app (the sidebar's NavLinks) without those
 * callers needing to know Settings, or its unsaved-edit tracking,
 * exists. */
export function NavigationGuardProvider({ children }: { children: ReactNode }) {
  const fnRef = useRef<GuardFn | null>(null);
  const register = useCallback((fn: GuardFn | null) => { fnRef.current = fn; }, []);
  const guard = useCallback<GuardFn>((action) => {
    if (fnRef.current) fnRef.current(action); else action();
  }, []);
  const value = useMemo(() => ({ register, guard }), [register, guard]);
  return <NavigationGuardContext.Provider value={value}>{children}</NavigationGuardContext.Provider>;
}

export function useNavigationGuard(): NavigationGuardValue {
  return useContext(NavigationGuardContext);
}
