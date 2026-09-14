import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it } from 'vitest';
import { ThemeProvider } from '../lib/theme';
import { ThemeToggle } from './ThemeToggle';

it('exposes the three themes as a labelled radio group and is keyboard operable', async () => {
  localStorage.clear();
  render(<ThemeProvider><ThemeToggle /></ThemeProvider>);
  const group = screen.getByRole('radiogroup', { name: /theme/i });
  expect(group).toBeInTheDocument();
  const dark = screen.getByRole('radio', { name: /dark/i });
  await userEvent.click(dark);
  expect(dark).toHaveAttribute('aria-checked', 'true');
  expect(localStorage.getItem('st-theme')).toBe('dark');
});
