import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Login } from './Login';

/** jsonResponse builds a fetch Response-shaped stub, matching the
 * convention used across this codebase's other page tests. */
function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body),
    json: async () => body,
  } as Response;
}

function mockAuthMode(mode: string) {
  vi.stubGlobal('fetch', vi.fn(async () => jsonResponse({ mode })));
}

afterEach(() => {
  vi.unstubAllGlobals();
});

function renderLogin(initialEntries: (string | { pathname: string; state?: unknown })[] = ['/login']) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={initialEntries}>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/" element={<div>home</div>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('Login', () => {
  beforeEach(() => mockAuthMode('oidc'));

  it('links "Sign in with SSO" to the OIDC start endpoint with the return path', async () => {
    renderLogin([{ pathname: '/login', state: { from: '/targets' } }]);
    expect(await screen.findByRole('link', { name: 'Sign in with SSO' }))
      .toHaveAttribute('href', '/auth/oidc/start?return_to=%2Ftargets');
  });

  it('defaults the return path to / when none was carried in location state', async () => {
    renderLogin(['/login']);
    expect(await screen.findByRole('link', { name: 'Sign in with SSO' }))
      .toHaveAttribute('href', '/auth/oidc/start?return_to=%2F');
  });

  it('shows a "View error" button that opens the error dialog for a known error code', async () => {
    renderLogin(['/login?error=forbidden']);
    expect(await screen.findByText('Sign-in failed')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'View error' }));
    expect(screen.getByText(/not in an allowed group/)).toBeInTheDocument();
  });

  it('renders no error UI when there is no error param', async () => {
    renderLogin(['/login']);
    await screen.findByRole('link', { name: 'Sign in with SSO' });
    expect(screen.queryByRole('button', { name: 'View error' })).not.toBeInTheDocument();
  });

  it('renders nothing while the auth mode is loading', () => {
    vi.stubGlobal('fetch', vi.fn(() => new Promise(() => {})));
    const { container } = renderLogin(['/login']);
    expect(container).toBeEmptyDOMElement();
  });

  it('shows a token-mode explanation with no sign-in action', async () => {
    mockAuthMode('token');
    renderLogin(['/login']);
    expect(await screen.findByText(/requires an API token/)).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Sign in with SSO' })).not.toBeInTheDocument();
  });

  it('shows a forward_auth explanation with no sign-in action', async () => {
    mockAuthMode('forward_auth');
    renderLogin(['/login']);
    expect(await screen.findByText(/reverse proxy did not send/)).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Sign in with SSO' })).not.toBeInTheDocument();
  });

  it('redirects to / in open mode', async () => {
    mockAuthMode('open');
    renderLogin(['/login']);
    await waitFor(() => expect(screen.getByText('home')).toBeInTheDocument());
  });
});
