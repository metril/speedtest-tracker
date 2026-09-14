import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {
  afterEach, beforeEach, describe, expect, it, vi,
} from 'vitest';
import type { Target } from '../../lib/api';
import { SortableTargetList } from './SortableTargetList';

function target(id: number, name: string): Target {
  return { id, name, engine: 'fake', enabled: true, lane: 'wan', options: {}, thresholds: {}, created_at: '', updated_at: '' };
}

const targets = [target(1, 'alpha'), target(2, 'bravo'), target(3, 'charlie')];
const byID = new Map(targets.map((t) => [t.id, t]));

describe('SortableTargetList', () => {
  it('renders nothing selected as a hint', () => {
    render(<SortableTargetList selected={[]} byID={byID} onChange={vi.fn()} />);
    expect(screen.getByText(/No targets selected/)).toBeInTheDocument();
  });

  it('reorders via the ↑/↓ buttons', async () => {
    const onChange = vi.fn();
    const { rerender } = render(<SortableTargetList selected={[1, 2, 3]} byID={byID} onChange={onChange} />);

    await userEvent.click(screen.getByRole('button', { name: 'Move bravo up' }));
    expect(onChange).toHaveBeenCalledWith([2, 1, 3]);

    // The component is controlled: the parent applies the new order and
    // passes it back down as a fresh `selected` prop.
    rerender(<SortableTargetList selected={[2, 1, 3]} byID={byID} onChange={onChange} />);
    await userEvent.click(screen.getByRole('button', { name: 'Move alpha down' }));
    expect(onChange).toHaveBeenCalledWith([2, 3, 1]);
  });

  it('disables the up button on the first row and the down button on the last', () => {
    render(<SortableTargetList selected={[1, 2, 3]} byID={byID} onChange={vi.fn()} />);
    expect(screen.getByRole('button', { name: 'Move alpha up' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Move charlie down' })).toBeDisabled();
  });

  it('removes a target via the × button', async () => {
    const onChange = vi.fn();
    render(<SortableTargetList selected={[1, 2, 3]} byID={byID} onChange={onChange} />);
    await userEvent.click(screen.getByRole('button', { name: 'Remove bravo' }));
    expect(onChange).toHaveBeenCalledWith([1, 3]);
  });

  it('falls back to "#id" for a selected id missing from byID', () => {
    render(<SortableTargetList selected={[99]} byID={byID} onChange={vi.fn()} />);
    expect(screen.getByText('#99')).toBeInTheDocument();
  });

  describe('past the virtualization threshold', () => {
    // jsdom reports a zero-size layout by default, which makes
    // @tanstack/react-virtual render an empty range; give the scroll
    // container a real height so the visible rows actually mount.
    beforeEach(() => {
      vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(320);
      vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockReturnValue(400);
    });
    afterEach(() => vi.restoreAllMocks());

    it('virtualizes and disables drag, keeping the buttons working', async () => {
      const many = Array.from({ length: 60 }, (_, i) => i + 1);
      const manyByID = new Map(many.map((id) => [id, target(id, `t${id}`)]));
      const onChange = vi.fn();
      render(<SortableTargetList selected={many} byID={manyByID} onChange={onChange} />);

      // No drag handles once virtualized.
      expect(screen.queryByLabelText(/^Drag /)).not.toBeInTheDocument();
      // Keyboard buttons for at least the first visible row still work.
      await userEvent.click(screen.getByRole('button', { name: 'Move t2 up' }));
      expect(onChange).toHaveBeenCalledWith(expect.arrayContaining([2, 1]));
    });
  });
});
