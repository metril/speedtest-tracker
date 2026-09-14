import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  render, screen, waitFor, within,
} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import * as api from '../../lib/api';
import { TagsPanel } from './TagsPanel';

function wrap(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const rendered = render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
  return { qc, ...rendered };
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
  wrap(<TagsPanel />);
  await userEvent.click(await screen.findByRole('button', { name: /delete tag wifi/i }));
  const dialog = await screen.findByRole('dialog');
  await userEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));
  await waitFor(() => expect(remove).toHaveBeenCalledWith(2));
});

it('shows an empty state when no tags exist', async () => {
  vi.spyOn(api, 'listTags').mockResolvedValue([]);
  wrap(<TagsPanel />);
  expect(await screen.findByText(/no tags yet/i)).toBeInTheDocument();
});

it('submits the rename on Enter', async () => {
  const rename = vi.spyOn(api, 'renameTag').mockResolvedValue({ id: 1, name: 'morning' });
  wrap(<TagsPanel />);
  await userEvent.click(await screen.findByRole('button', { name: /rename tag evening/i }));
  const field = screen.getByRole('textbox', { name: /tag name/i });
  await userEvent.clear(field);
  await userEvent.type(field, 'morning{Enter}');
  await waitFor(() => expect(rename).toHaveBeenCalledWith(1, 'morning'));
});

it('cancels editing on Escape without saving', async () => {
  const rename = vi.spyOn(api, 'renameTag').mockResolvedValue({ id: 1, name: 'morning' });
  wrap(<TagsPanel />);
  await userEvent.click(await screen.findByRole('button', { name: /rename tag evening/i }));
  const field = screen.getByRole('textbox', { name: /tag name/i });
  await userEvent.clear(field);
  await userEvent.type(field, 'morning{Escape}');
  expect(rename).not.toHaveBeenCalled();
  expect(screen.queryByRole('textbox', { name: /tag name/i })).not.toBeInTheDocument();
});

it('blocks an empty or whitespace-only name client-side with an inline message', async () => {
  const rename = vi.spyOn(api, 'renameTag').mockResolvedValue({ id: 1, name: 'x' });
  wrap(<TagsPanel />);
  await userEvent.click(await screen.findByRole('button', { name: /rename tag evening/i }));
  const field = screen.getByRole('textbox', { name: /tag name/i });
  await userEvent.clear(field);
  await userEvent.type(field, '   ');
  await userEvent.click(screen.getByRole('button', { name: /^save$/i }));
  expect(rename).not.toHaveBeenCalled();
  expect(await screen.findByText(/cannot be empty/i)).toBeInTheDocument();
});

it('resets editingId when the edited tag disappears from the list', async () => {
  const { qc } = wrap(<TagsPanel />);
  await userEvent.click(await screen.findByRole('button', { name: /rename tag evening/i }));
  expect(screen.getByRole('textbox', { name: /tag name/i })).toBeInTheDocument();

  qc.setQueryData(['tags'], [{ id: 2, name: 'wifi' }]);

  await waitFor(() => expect(screen.queryByRole('textbox', { name: /tag name/i })).not.toBeInTheDocument());
});
