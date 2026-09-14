import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { RangePicker } from './RangePicker';

it('reports the selected range and marks the current one', async () => {
  const onChange = vi.fn();
  render(<RangePicker value="24h" onChange={onChange} />);
  expect(screen.getByRole('radio', { name: '24h' })).toHaveAttribute('aria-checked', 'true');
  await userEvent.click(screen.getByRole('radio', { name: '7d' }));
  expect(onChange).toHaveBeenCalledWith('7d');
});
