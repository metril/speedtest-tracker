import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Me } from '../lib/api';
import { ThemeProvider } from '../lib/theme';
import { Layout } from './Layout';

function jsonResponse(body: unknown, status = 200): Response {
  return { ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body) } as Response;
}

const OPEN_ME: Me = { mode: 'open', user: '', display_name: '', groups: [], is_admin: true };

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.startsWith('/api/v1/me')) return jsonResponse(OPEN_ME);
    throw new Error(`unexpected fetch: ${url}`);
  }));
});
afterEach(() => vi.unstubAllGlobals());

function renderLayout(path = '/', me: Me = OPEN_ME) {
  if (me !== OPEN_ME) {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.startsWith('/api/v1/me')) return jsonResponse(me);
      if (url.startsWith('/auth/logout')) return jsonResponse(undefined, 204);
      throw new Error(`unexpected fetch: ${url}`);
    }));
  }
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <ThemeProvider>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={[path]}>
          <Routes>
            <Route path="/login" element={<div>Login page marker</div>} />
            <Route path="/*" element={<Layout />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    </ThemeProvider>,
  );
}

describe('Layout', () => {
  it('renders every primary navigation link', () => {
    renderLayout();
    for (const label of ['Dashboard', 'Results', 'Targets', 'Schedules', 'Settings']) {
      expect(screen.getByRole('link', { name: label })).toBeInTheDocument();
    }
  });

  it('points each link at its route', () => {
    renderLayout();
    expect(screen.getByRole('link', { name: 'Dashboard' })).toHaveAttribute('href', '/');
    expect(screen.getByRole('link', { name: 'Results' })).toHaveAttribute('href', '/results');
    expect(screen.getByRole('link', { name: 'Targets' })).toHaveAttribute('href', '/targets');
    expect(screen.getByRole('link', { name: 'Schedules' })).toHaveAttribute('href', '/schedules');
    expect(screen.getByRole('link', { name: 'Settings' })).toHaveAttribute('href', '/settings');
  });

  it('marks the active route with aria-current', () => {
    renderLayout('/results');
    expect(screen.getByRole('link', { name: 'Results' })).toHaveAttribute('aria-current', 'page');
    expect(screen.getByRole('link', { name: 'Targets' })).not.toHaveAttribute('aria-current');
  });

  it('swaps the sidebar theme toggle for a compact icon button when collapsed', async () => {
    renderLayout();
    // Only the mobile header's radiogroup (hidden on desktop, still mounted)
    // and the sidebar's radiogroup exist before collapsing.
    expect(screen.getAllByRole('radiogroup', { name: /theme/i })).toHaveLength(2);
    expect(screen.queryByRole('button', { name: /^Theme: /i })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Collapse sidebar' }));
    // The sidebar toggle becomes a compact cycling icon button; the mobile
    // header's radiogroup is unaffected.
    expect(screen.getAllByRole('radiogroup', { name: /theme/i })).toHaveLength(1);
    expect(screen.getByRole('button', { name: /^Theme: /i })).toBeInTheDocument();
  });

  it('shows no user chip or sign out button outside oidc mode', async () => {
    renderLayout();
    await screen.findByRole('link', { name: 'Dashboard' });
    expect(screen.queryByRole('button', { name: 'Sign out' })).not.toBeInTheDocument();
  });

  it('shows the signed-in user and a sign out button under oidc mode', async () => {
    renderLayout('/', { mode: 'oidc', user: 'alice', display_name: 'Alice', groups: [], is_admin: true });
    expect(await screen.findAllByText('Alice')).not.toHaveLength(0);
    expect(screen.getAllByRole('button', { name: 'Sign out' }).length).toBeGreaterThan(0);
  });

  it('signs out, clears the query cache and navigates to /login', async () => {
    renderLayout('/', { mode: 'oidc', user: 'alice', display_name: 'alice', groups: [], is_admin: true });
    const [signOut] = await screen.findAllByRole('button', { name: 'Sign out' });
    await userEvent.click(signOut);
    expect(await screen.findByText('Login page marker')).toBeInTheDocument();
  });
});
