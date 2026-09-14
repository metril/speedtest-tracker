import {
  CalendarClock, ChevronsLeft, ChevronsRight, Gauge, ListChecks, Menu, Settings as SettingsIcon, Target,
} from 'lucide-react';
import { useState, type ComponentType } from 'react';
import { NavLink, Outlet } from 'react-router';
import { Button } from '@/components/ui/button';
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet';
import { LivePanel } from '../features/live/LivePanel';
import { LiveRunProvider } from '../features/live/LiveRunProvider';
import { OpenModeBanner } from './OpenModeBanner';
import { ThemeToggle } from './ThemeToggle';

const NAV: { to: string; label: string; icon: ComponentType<{ className?: string }> }[] = [
  { to: '/', label: 'Dashboard', icon: Gauge },
  { to: '/results', label: 'Results', icon: ListChecks },
  { to: '/targets', label: 'Targets', icon: Target },
  { to: '/schedules', label: 'Schedules', icon: CalendarClock },
  { to: '/settings', label: 'Settings', icon: SettingsIcon },
];

function NavLinks({ collapsed, onNavigate }: { collapsed?: boolean; onNavigate?: () => void }) {
  return (
    <nav className="flex flex-col gap-1">
      {NAV.map(({ to, label, icon: Icon }) => (
        <NavLink
          key={to}
          to={to}
          end={to === '/'}
          onClick={onNavigate}
          title={collapsed ? label : undefined}
          className={({ isActive }) =>
            `flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors ${
              isActive ? 'bg-raised text-accent' : 'text-muted hover:bg-raised hover:text-fg'
            } ${collapsed ? 'justify-center' : ''}`
          }
        >
          <Icon className="h-4 w-4 shrink-0" />
          <span className={collapsed ? 'sr-only' : ''}>{label}</span>
        </NavLink>
      ))}
    </nav>
  );
}

/** Layout is the app shell: a collapsible icon+label sidebar on md and up,
 * a top bar with a Sheet-based nav drawer below that, both wrapping the
 * routed page content. OpenModeBanner, LivePanel and ThemeToggle stay
 * mounted regardless of which nav surface is showing. */
export function Layout() {
  const [collapsed, setCollapsed] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);

  return (
    <LiveRunProvider>
      <div className="min-h-screen bg-app text-fg">
        <OpenModeBanner />
        <div className="flex min-h-screen">
          <aside
            className={`hidden shrink-0 border-r border-line bg-surface md:flex md:flex-col ${
              collapsed ? 'md:w-16' : 'md:w-56'
            } transition-[width]`}
          >
            <div className={`flex items-center gap-2 px-4 py-4 ${collapsed ? 'justify-center px-2' : ''}`}>
              <span className={`font-semibold tracking-tight text-fg ${collapsed ? 'sr-only' : ''}`}>
                speedtest-tracker
              </span>
            </div>
            <div className="flex-1 px-2">
              <NavLinks collapsed={collapsed} />
            </div>
            <div className={`flex items-center gap-2 border-t border-line p-2 ${collapsed ? 'flex-col' : 'justify-between'}`}>
              <ThemeToggle />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
                onClick={() => setCollapsed((v) => !v)}
              >
                {collapsed ? <ChevronsRight className="h-4 w-4" /> : <ChevronsLeft className="h-4 w-4" />}
              </Button>
            </div>
          </aside>

          <div className="flex min-w-0 flex-1 flex-col">
            <header className="flex items-center gap-3 border-b border-line px-4 py-3 md:hidden">
              <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label="Open navigation"
                  onClick={() => setMobileOpen(true)}
                >
                  <Menu className="h-5 w-5" />
                </Button>
                <SheetContent side="left" className="w-64">
                  <SheetTitle>speedtest-tracker</SheetTitle>
                  <div className="mt-4">
                    <NavLinks onNavigate={() => setMobileOpen(false)} />
                  </div>
                </SheetContent>
              </Sheet>
              <span className="font-semibold tracking-tight">speedtest-tracker</span>
              <div className="ml-auto"><ThemeToggle /></div>
            </header>

            <LivePanel />
            <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-6">
              <Outlet />
            </main>
          </div>
        </div>
      </div>
    </LiveRunProvider>
  );
}
