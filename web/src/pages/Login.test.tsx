import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router';
import { describe, expect, it } from 'vitest';
import { Login } from './Login';

describe('Login', () => {
  it('links "Sign in with SSO" to the OIDC start endpoint with the return path', () => {
    render(
      <MemoryRouter initialEntries={[{ pathname: '/login', state: { from: '/targets' } }]}>
        <Login />
      </MemoryRouter>,
    );
    expect(screen.getByRole('link', { name: 'Sign in with SSO' }))
      .toHaveAttribute('href', '/auth/oidc/start?return_to=%2Ftargets');
  });

  it('defaults the return path to / when none was carried in location state', () => {
    render(
      <MemoryRouter initialEntries={['/login']}>
        <Login />
      </MemoryRouter>,
    );
    expect(screen.getByRole('link', { name: 'Sign in with SSO' }))
      .toHaveAttribute('href', '/auth/oidc/start?return_to=%2F');
  });

  it('shows a "View error" button that opens the error dialog for a known error code', async () => {
    render(
      <MemoryRouter initialEntries={['/login?error=forbidden']}>
        <Routes>
          <Route path="/login" element={<Login />} />
        </Routes>
      </MemoryRouter>,
    );
    expect(screen.getByText('Sign-in failed')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'View error' }));
    expect(screen.getByText(/not in an allowed group/)).toBeInTheDocument();
  });

  it('renders no error UI when there is no error param', () => {
    render(
      <MemoryRouter initialEntries={['/login']}>
        <Login />
      </MemoryRouter>,
    );
    expect(screen.queryByRole('button', { name: 'View error' })).not.toBeInTheDocument();
  });
});
