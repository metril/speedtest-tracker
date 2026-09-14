import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import * as api from '../../lib/api';
import { TagsPanel } from './TagsPanel';

function wrap(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

beforeEach(() => {
  vi.spyOn(api, 'listTags').mockResolvedValue([{ id: 1, name: 'evening' }, { id: 2, name: 'wifi' }]);
});
afterEach(() => vi.restoreAllMocks());

it('renames a tag', async () => {
  const rename = vi.spyOn(api, 'renameTag').mockResolvedValue({ id: 1, name: 'morning' });
  wrap(<TagsPanel />);
  await userEvent.click(await screen.findByRole('button', { name: /rename tag evening/i }));
  const field = screen.getByRole('textbox', { name: /tag name/i });
  await userEvent.clear(field);
  await userEvent.type(field, 'morning');
  await userEvent.click(screen.getByRole('button', { name: /^save$/i }));
  await waitFor(() => expect(rename).toHaveBeenCalledWith(1, 'morning'));
});

it('deletes a tag after confirmation', async () => {
  const remove = vi.spyOn(api, 'deleteTag').mockResolvedValue(undefined);
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  wrap(<TagsPanel />);
  await userEvent.click(await screen.findByRole('button', { name: /delete tag wifi/i }));
  await waitFor(() => expect(remove).toHaveBeenCalledWith(2));
});

it('shows an empty state when no tags exist', async () => {
  vi.spyOn(api, 'listTags').mockResolvedValue([]);
  wrap(<TagsPanel />);
  expect(await screen.findByText(/no tags yet/i)).toBeInTheDocument();
});
