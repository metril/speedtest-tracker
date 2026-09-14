import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';

export type Theme = 'system' | 'light' | 'dark';
export type Resolved = 'light' | 'dark';

const STORAGE_KEY = 'st-theme';

/** readStored returns a persisted choice, defaulting to system. Storage can
 * throw in a locked-down browser, so it never escapes. */
function readStored(): Theme {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v === 'light' || v === 'dark' || v === 'system') return v;
  } catch { /* ignore */ }
  return 'system';
}

function systemTheme(): Resolved {
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

/** applyTheme stamps the resolved theme on <html> so Tailwind's dark:
 * variant and the CSS token overrides both switch at once. */
function applyTheme(resolved: Resolved) {
  const root = document.documentElement;
  root.classList.toggle('dark', resolved === 'dark');
  root.classList.toggle('light', resolved === 'light');
}

interface Ctx { theme: Theme; resolved: Resolved; setTheme: (t: Theme) => void }
const ThemeContext = createContext<Ctx | null>(null);

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(readStored);
  const [system, setSystem] = useState<Resolved>(systemTheme);

  useEffect(() => {
    const mq = window.matchMedia?.('(prefers-color-scheme: dark)');
    if (!mq) return;
    const onChange = () => setSystem(mq.matches ? 'dark' : 'light');
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, []);

  const resolved: Resolved = theme === 'system' ? system : theme;
  useEffect(() => { applyTheme(resolved); }, [resolved]);

  const setTheme = useCallback((t: Theme) => {
    setThemeState(t);
    try { localStorage.setItem(STORAGE_KEY, t); } catch { /* ignore */ }
  }, []);

  const value = useMemo(() => ({ theme, resolved, setTheme }), [theme, resolved, setTheme]);
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): Ctx {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used inside ThemeProvider');
  return ctx;
}
