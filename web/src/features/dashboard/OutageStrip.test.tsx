import { render, screen } from '@testing-library/react';
import { expect, it } from 'vitest';
import type { Incident } from '../../lib/api';
import { OutageStrip } from './OutageStrip';

const incidents: Incident[] = [
  { target_id: 1, target_name: 'home', kind: 'result', status: 'failed',
    started_at: '2026-09-13T10:00:00.000Z', ended_at: '2026-09-13T10:30:00.000Z', count: 3, error: 'timeout' },
  { target_id: null, target_name: 'nightly', kind: 'skipped', status: 'skipped',
    started_at: '2026-09-13T12:00:00.000Z', ended_at: '2026-09-13T12:00:00.000Z', count: 1, error: 'still running' },
];

it('renders one focusable bar per incident with a descriptive label', () => {
  render(<OutageStrip incidents={incidents} from="2026-09-13T00:00:00.000Z" to="2026-09-14T00:00:00.000Z" />);
  const bars = screen.getAllByRole('button');
  expect(bars).toHaveLength(2);
  expect(bars[0]).toHaveAccessibleName(/home.*failed.*3/i);
  expect(bars[1]).toHaveAccessibleName(/nightly.*skipped/i);
});

it('says so when the window was clean', () => {
  render(<OutageStrip incidents={[]} from="2026-09-13T00:00:00.000Z" to="2026-09-14T00:00:00.000Z" />);
  expect(screen.getByText(/no outages/i)).toBeInTheDocument();
});
