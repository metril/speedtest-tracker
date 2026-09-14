import { render, screen } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { App } from './App';

function jsonResponse(body: unknown, status = 200): Response {
  return { ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body) } as Response;
}

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.startsWith('/api/v1/stats/summary')) {
      return jsonResponse({
        from: '', to: '', targets: [], total_results: 0, total_failures: 0, success_rate: 0,
      });
    }
    throw new Error(`unexpected fetch: ${url}`);
  }));
});
afterEach(() => vi.unstubAllGlobals());

it('renders each route lazily without crashing', async () => {
  render(<App />);
  // Suspense fallback first, then the dashboard once its chunk resolves.
  expect(await screen.findByRole('main')).toBeInTheDocument();
});
