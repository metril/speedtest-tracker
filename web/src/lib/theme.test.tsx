import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it } from 'vitest';
import { ThemeProvider, useTheme } from './theme';

function Probe() {
  const { theme, resolved, setTheme } = useTheme();
  return (
    <div>
      <span data-testid="state">{theme}/{resolved}</span>
      <button onClick={() => setTheme('light')}>light</button>
      <button onClick={() => setTheme('system')}>system</button>
    </div>
  );
}

describe('theme', () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.className = '';
  });

  it('defaults to system and stamps the resolved class on <html>', () => {
    render(<ThemeProvider><Probe /></ThemeProvider>);
    expect(screen.getByTestId('state').textContent).toMatch(/^system\/(light|dark)$/);
    expect(document.documentElement.classList.contains('dark')
      || document.documentElement.classList.contains('light')).toBe(true);
  });

  it('persists an explicit choice and applies it', async () => {
    render(<ThemeProvider><Probe /></ThemeProvider>);
    await userEvent.click(screen.getByRole('button', { name: 'light' }));
    expect(screen.getByTestId('state').textContent).toBe('light/light');
    expect(document.documentElement.classList.contains('dark')).toBe(false);
    expect(localStorage.getItem('st-theme')).toBe('light');
  });

  it('restores a stored choice on mount', () => {
    localStorage.setItem('st-theme', 'dark');
    render(<ThemeProvider><Probe /></ThemeProvider>);
    expect(screen.getByTestId('state').textContent).toBe('dark/dark');
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });
});
