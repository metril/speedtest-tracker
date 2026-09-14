import { NavLink, Outlet } from 'react-router';
import { LivePanel } from '../features/live/LivePanel';
import { LiveRunProvider } from '../features/live/LiveRunProvider';
import { OpenModeBanner } from './OpenModeBanner';
import { ThemeToggle } from './ThemeToggle';

const NAV = [
  { to: '/', label: 'Dashboard' },
  { to: '/results', label: 'Results' },
  { to: '/targets', label: 'Targets' },
  { to: '/schedules', label: 'Schedules' },
  { to: '/settings', label: 'Settings' },
] as const;

export function Layout() {
  return (
    <LiveRunProvider>
      <div className="min-h-screen bg-app text-fg">
        <OpenModeBanner />
        <header className="border-b border-line">
          <div className="mx-auto flex max-w-6xl items-center gap-6 px-4 py-3">
            <span className="font-semibold tracking-tight">speedtest-tracker</span>
            <nav className="flex gap-4 text-sm">
              {NAV.map(({ to, label }) => (
                <NavLink
                  key={to}
                  to={to}
                  end={to === '/'}
                  className={({ isActive }) =>
                    isActive ? 'text-accent' : 'text-muted hover:text-fg'
                  }
                >
                  {label}
                </NavLink>
              ))}
            </nav>
            <div className="ml-auto"><ThemeToggle /></div>
          </div>
        </header>
        <LivePanel />
        <main className="mx-auto max-w-6xl px-4 py-6">
          <Outlet />
        </main>
      </div>
    </LiveRunProvider>
  );
}
