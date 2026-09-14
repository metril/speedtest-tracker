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

it('moves selection and focus with arrow keys (roving tabindex)', async () => {
  localStorage.clear();
  const user = userEvent.setup();
  render(<ThemeProvider><ThemeToggle /></ThemeProvider>);

  const system = screen.getByRole('radio', { name: /system/i });
  const light = screen.getByRole('radio', { name: /light/i });
  const dark = screen.getByRole('radio', { name: /dark/i });

  // Only the checked option is in the tab order.
  expect(system).toHaveAttribute('tabindex', '0');
  expect(light).toHaveAttribute('tabindex', '-1');
  expect(dark).toHaveAttribute('tabindex', '-1');

  system.focus();
  await user.keyboard('{ArrowRight}');
  expect(light).toHaveAttribute('aria-checked', 'true');
  expect(light).toHaveFocus();
  expect(localStorage.getItem('st-theme')).toBe('light');

  await user.keyboard('{ArrowRight}');
  expect(dark).toHaveAttribute('aria-checked', 'true');
  expect(dark).toHaveFocus();

  await user.keyboard('{ArrowRight}');
  expect(system).toHaveAttribute('aria-checked', 'true');
  expect(system).toHaveFocus();

  await user.keyboard('{ArrowLeft}');
  expect(dark).toHaveAttribute('aria-checked', 'true');
  expect(dark).toHaveFocus();

  await user.keyboard('{Home}');
  expect(system).toHaveAttribute('aria-checked', 'true');
  await user.keyboard('{End}');
  expect(dark).toHaveAttribute('aria-checked', 'true');
});
