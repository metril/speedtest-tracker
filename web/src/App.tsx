import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { lazy, Suspense, useState, type ReactNode } from 'react';
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router';
import { Layout } from './components/Layout';
import { NavigationGuardProvider } from './features/settings/NavigationGuardContext';
import { ApiError } from './lib/api';
import { useMe } from './lib/queries';
import { ThemeProvider } from './lib/theme';

const Login = lazy(() => import('./pages/Login').then((m) => ({ default: m.Login })));
const Dashboard = lazy(() => import('./pages/Dashboard').then((m) => ({ default: m.Dashboard })));
const Results = lazy(() => import('./pages/Results').then((m) => ({ default: m.Results })));
const Targets = lazy(() => import('./pages/Targets').then((m) => ({ default: m.Targets })));
const Schedules = lazy(() => import('./pages/Schedules').then((m) => ({ default: m.Schedules })));
const Settings = lazy(() => import('./pages/Settings').then((m) => ({ default: m.Settings })));
const GeneralSection = lazy(() =>
  import('./features/settings/GeneralSection').then((m) => ({ default: m.GeneralSection })));
const EnginesSection = lazy(() =>
  import('./features/settings/EnginesSection').then((m) => ({ default: m.EnginesSection })));
const ExportersSection = lazy(() =>
  import('./features/settings/ExportersSection').then((m) => ({ default: m.ExportersSection })));
const NotificationsSection = lazy(() =>
  import('./features/settings/NotificationsSection').then((m) => ({ default: m.NotificationsSection })));
const AccessSection = lazy(() =>
  import('./features/settings/AccessSection').then((m) => ({ default: m.AccessSection })));

/** RequireAuth gates the main app shell behind an identity check: while
 * useMe() is loading it renders nothing, a 401 sends the caller to /login
 * (remembering where they were headed), and any other error falls through
 * to the existing behaviour (the wrapped route renders and handles it
 * itself, same as before this gate existed). */
function RequireAuth({ children }: { children: ReactNode }) {
  const location = useLocation();
  const me = useMe();

  if (me.isLoading) return null;
  if (me.isError && me.error instanceof ApiError && me.error.status === 401) {
    return <Navigate to="/login" state={{ from: location.pathname }} replace />;
  }
  return <>{children}</>;
}

export function App() {
  // Created per mount (not module scope) so each <App/> instance -- and
  // each test render -- starts from a clean query cache.
  const [queryClient] = useState(() => new QueryClient({
    defaultOptions: { queries: { staleTime: 30_000, refetchOnWindowFocus: false } },
  }));

  return (
    <ThemeProvider>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <NavigationGuardProvider>
            <Suspense fallback={<div className="p-6 text-muted">Loading…</div>}>
              <Routes>
                <Route path="/login" element={<Login />} />
                <Route element={<RequireAuth><Layout /></RequireAuth>}>
                  <Route index element={<Dashboard />} />
                  <Route path="results" element={<Results />} />
                  <Route path="targets" element={<Targets />} />
                  <Route path="schedules" element={<Schedules />} />
                  <Route path="settings" element={<Settings />}>
                    <Route index element={<Navigate to="general" replace />} />
                    <Route path="general" element={<GeneralSection />} />
                    <Route path="engines" element={<EnginesSection />} />
                    <Route path="exporters" element={<ExportersSection />} />
                    <Route path="integrations" element={<Navigate to="/settings/exporters" replace />} />
                    <Route path="notifications" element={<NotificationsSection />} />
                    <Route path="access" element={<AccessSection />} />
                    <Route path="auth" element={<Navigate to="/settings/access" replace />} />
                  </Route>
                </Route>
              </Routes>
            </Suspense>
          </NavigationGuardProvider>
        </BrowserRouter>
      </QueryClientProvider>
    </ThemeProvider>
  );
}
