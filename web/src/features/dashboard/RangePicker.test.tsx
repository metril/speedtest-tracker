import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { expect, it, vi } from 'vitest';
import type { Range } from '../../lib/api';
import { RangePicker } from './RangePicker';

it('reports the selected range and marks the current one', async () => {
  const onChange = vi.fn();
  render(<RangePicker value="24h" onChange={onChange} />);
  expect(screen.getByRole('radio', { name: '24h' })).toHaveAttribute('aria-checked', 'true');
  await userEvent.click(screen.getByRole('radio', { name: '7d' }));
  expect(onChange).toHaveBeenCalledWith('7d');
});

/** Controlled wrapper so ArrowRight's effect (a new `value`, and the
 * roving tabIndex it drives) actually reaches the DOM, the way it would
 * under the real Dashboard page. */
function ControlledPicker() {
  const [value, setValue] = useState<Range>('24h');
  return <RangePicker value={value} onChange={setValue} />;
}

it('is arrow-key operable: Tab into the group, ArrowRight selects and focuses the next range', async () => {
  const user = userEvent.setup();
  render(<ControlledPicker />);
  await user.tab();
  expect(screen.getByRole('radio', { name: '24h' })).toHaveFocus();

  await user.keyboard('{ArrowRight}');
  expect(screen.getByRole('radio', { name: '7d' })).toHaveAttribute('aria-checked', 'true');
  expect(screen.getByRole('radio', { name: '7d' })).toHaveFocus();

  await user.keyboard('{ArrowRight}');
  expect(screen.getByRole('radio', { name: '30d' })).toHaveAttribute('aria-checked', 'true');
  expect(screen.getByRole('radio', { name: '30d' })).toHaveFocus();

  await user.keyboard('{Home}');
  expect(screen.getByRole('radio', { name: '24h' })).toHaveAttribute('aria-checked', 'true');
  expect(screen.getByRole('radio', { name: '24h' })).toHaveFocus();

  await user.keyboard('{ArrowLeft}');
  expect(screen.getByRole('radio', { name: '30d' })).toHaveAttribute('aria-checked', 'true');
  expect(screen.getByRole('radio', { name: '30d' })).toHaveFocus();
});
