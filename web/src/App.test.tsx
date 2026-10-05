import { render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { App } from './App';

function jsonResponse(body: unknown, status = 200): Response {
  return { ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body) } as Response;
}

afterEach(() => vi.unstubAllGlobals());

it('renders each route lazily without crashing', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.startsWith('/api/v1/me')) return jsonResponse({ mode: 'open', user: '', groups: [], is_admin: true });
    if (url.startsWith('/api/v1/stats/summary')) {
      return jsonResponse({
        from: '', to: '', targets: [], total_results: 0, total_failures: 0, success_rate: 0,
      });
    }
    throw new Error(`unexpected fetch: ${url}`);
  }));
  render(<App />);
  // Suspense fallback first, then the dashboard once its chunk resolves.
  expect(await screen.findByRole('main')).toBeInTheDocument();
});

it('redirects to /login when /me answers 401', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.startsWith('/api/v1/me')) {
      return jsonResponse({ error: { code: 'unauthenticated', message: 'not signed in' } }, 401);
    }
    if (url.startsWith('/auth/mode')) return jsonResponse({ mode: 'oidc' });
    throw new Error(`unexpected fetch: ${url}`);
  }));
  render(<App />);
  expect(await screen.findByRole('link', { name: 'Sign in with SSO' })).toBeInTheDocument();
});

it('redirects to /login when a later query answers 401 outside open mode', async () => {
  let meCalls = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.startsWith('/api/v1/me')) {
      meCalls += 1;
      return meCalls === 1
        ? jsonResponse({ mode: 'oidc', user: 'a', groups: [], is_admin: true })
        : jsonResponse({ error: { code: 'unauthenticated', message: 'expired' } }, 401);
    }
    if (url.startsWith('/api/v1/stats/summary')) {
      return jsonResponse({ error: { code: 'unauthenticated', message: 'expired' } }, 401);
    }
    if (url.startsWith('/auth/mode')) return jsonResponse({ mode: 'oidc' });
    return jsonResponse({});
  }));
  render(<App />);
  expect(await screen.findByRole('link', { name: 'Sign in with SSO' })).toBeInTheDocument();
});
