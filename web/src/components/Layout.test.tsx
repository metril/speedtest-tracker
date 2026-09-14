import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router';
import { describe, expect, it } from 'vitest';
import { ThemeProvider } from '../lib/theme';
import { Layout } from './Layout';

function renderLayout(path = '/') {
  const client = new QueryClient();
  return render(
    <ThemeProvider>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={[path]}>
          <Layout />
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
});
