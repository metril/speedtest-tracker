import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { lazy, Suspense } from 'react';
import { BrowserRouter, Navigate, Route, Routes } from 'react-router';
import { Layout } from './components/Layout';
import { ThemeProvider } from './lib/theme';

const Dashboard = lazy(() => import('./pages/Dashboard').then((m) => ({ default: m.Dashboard })));
const Results = lazy(() => import('./pages/Results').then((m) => ({ default: m.Results })));
const Targets = lazy(() => import('./pages/Targets').then((m) => ({ default: m.Targets })));
const Schedules = lazy(() => import('./pages/Schedules').then((m) => ({ default: m.Schedules })));
const Settings = lazy(() => import('./pages/Settings').then((m) => ({ default: m.Settings })));
const GeneralSection = lazy(() =>
  import('./features/settings/GeneralSection').then((m) => ({ default: m.GeneralSection })));
const EnginesSection = lazy(() =>
  import('./features/settings/EnginesSection').then((m) => ({ default: m.EnginesSection })));
const IntegrationsSection = lazy(() =>
  import('./features/settings/IntegrationsSection').then((m) => ({ default: m.IntegrationsSection })));
const NotificationsSection = lazy(() =>
  import('./features/settings/NotificationsSection').then((m) => ({ default: m.NotificationsSection })));
const AuthSettingsSection = lazy(() =>
  import('./features/settings/AuthSettingsSection').then((m) => ({ default: m.AuthSettingsSection })));

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, refetchOnWindowFocus: false } },
});

export function App() {
  return (
    <ThemeProvider>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <Suspense fallback={<div className="p-6 text-muted">Loading…</div>}>
            <Routes>
              <Route element={<Layout />}>
                <Route index element={<Dashboard />} />
                <Route path="results" element={<Results />} />
                <Route path="targets" element={<Targets />} />
                <Route path="schedules" element={<Schedules />} />
                <Route path="settings" element={<Settings />}>
                  <Route index element={<Navigate to="general" replace />} />
                  <Route path="general" element={<GeneralSection />} />
                  <Route path="engines" element={<EnginesSection />} />
                  <Route path="integrations" element={<IntegrationsSection />} />
                  <Route path="notifications" element={<NotificationsSection />} />
                  <Route path="auth" element={<AuthSettingsSection />} />
                </Route>
              </Route>
            </Routes>
          </Suspense>
        </BrowserRouter>
      </QueryClientProvider>
    </ThemeProvider>
  );
}
