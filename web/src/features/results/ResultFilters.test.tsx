import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import {
  afterAll, afterEach, beforeAll, describe, expect, it, vi,
} from 'vitest';
import type { ResultFilters } from '../../lib/api';
import { ResultFiltersBar } from './ResultFilters';

function wrap(node: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{node}</QueryClientProvider>);
}

// Pin the environment's timezone to something offset from UTC so the
// datetime-local <-> ISO conversion is actually exercised (a UTC test
// environment would let a naive "append Z" bug pass silently).
const ORIGINAL_TZ = process.env.TZ;
beforeAll(() => {
  process.env.TZ = 'America/New_York'; // UTC-5 in January (no DST)
});
afterAll(() => {
  process.env.TZ = ORIGINAL_TZ;
});

beforeAll(() => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    statusText: 'ok',
    text: async () => JSON.stringify([{ id: 1, name: 'night' }, { id: 2, name: 'isp' }]),
  }));
});
afterEach(() => {
  vi.mocked(fetch).mockClear();
});

describe('ResultFiltersBar date conversion', () => {
  it('converts a local datetime-local value to a UTC ISO string outbound', () => {
    const onChange = vi.fn();
    wrap(<ResultFiltersBar value={{}} onChange={onChange} targets={[]} />);

    fireEvent.change(screen.getByLabelText('From'), { target: { value: '2024-01-15T10:30' } });

    const patch = onChange.mock.calls[0][0] as ResultFilters;
    // America/New_York is UTC-5 in January, so 10:30 local is 15:30 UTC.
    expect(patch.from).toBe('2024-01-15T15:30:00.000Z');
  });

  it('renders a stored UTC ISO string back as the equivalent local value', () => {
    wrap(
      <ResultFiltersBar
        value={{ from: '2024-01-15T15:30:00.000Z' }}
        onChange={vi.fn()}
        targets={[]}
      />,
    );
    expect(screen.getByLabelText('From')).toHaveValue('2024-01-15T10:30');
  });

  it('clears the filter when the input is emptied', () => {
    const onChange = vi.fn();
    wrap(
      <ResultFiltersBar
        value={{ to: '2024-01-15T15:30:00.000Z' }}
        onChange={onChange}
        targets={[]}
      />,
    );
    fireEvent.change(screen.getByLabelText('To'), { target: { value: '' } });
    expect(onChange.mock.calls[0][0].to).toBeUndefined();
  });
});

describe('ResultFiltersBar tag filter', () => {
  it('lists tags from useTags and wires the selection into the filter', async () => {
    const onChange = vi.fn();
    wrap(<ResultFiltersBar value={{}} onChange={onChange} targets={[]} />);

    const select = screen.getByLabelText('Tag');
    expect(await screen.findByRole('option', { name: 'night' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'isp' })).toBeInTheDocument();

    fireEvent.change(select, { target: { value: 'night' } });
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ tag: 'night' }));
  });
});
