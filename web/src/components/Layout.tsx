import { NavLink, Outlet } from 'react-router';
import { LivePanel } from '../features/live/LivePanel';
import { LiveRunProvider } from '../features/live/LiveRunProvider';

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
      <div className="min-h-screen bg-slate-950 text-slate-100">
        <header className="border-b border-slate-800">
          <div className="mx-auto flex max-w-6xl items-center gap-6 px-4 py-3">
            <span className="font-semibold tracking-tight">speedtest-tracker</span>
            <nav className="flex gap-4 text-sm">
              {NAV.map(({ to, label }) => (
                <NavLink
                  key={to}
                  to={to}
                  end={to === '/'}
                  className={({ isActive }) =>
                    isActive ? 'text-sky-400' : 'text-slate-400 hover:text-slate-200'
                  }
                >
                  {label}
                </NavLink>
              ))}
            </nav>
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
