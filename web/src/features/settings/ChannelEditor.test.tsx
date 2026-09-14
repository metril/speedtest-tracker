import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import type { NotifyChannel } from '../../lib/api';
import { ChannelEditor } from './ChannelEditor';

const webhookChannel: NotifyChannel = {
  id: 'c1', type: 'webhook', name: 'hook', enabled: true, url: 'https://hook', headers: { 'X-Foo': 'bar' },
};

describe('ChannelEditor', () => {
  it('shows only the fields the selected type uses', async () => {
    const onChange = vi.fn();
    render(
      <ChannelEditor value={webhookChannel} onChange={onChange} onRemove={vi.fn()} onTest={vi.fn()} />,
    );
    expect(screen.getByLabelText('Headers')).toBeInTheDocument();
    expect(screen.queryByLabelText('Priority')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Token')).not.toBeInTheDocument();
  });

  it('keeps prior-type fields (e.g. a token) in state across a type flip-flop', async () => {
    // A stateful harness stands in for Settings.tsx's controlled state, so
    // this exercises the real onChange -> value round trip a flip-flop
    // (ntfy -> webhook -> ntfy) goes through in the app.
    function Harness() {
      const [value, setValue] = useState<NotifyChannel>({
        id: 'c2', type: 'ntfy', name: 'phone', enabled: true, url: 'https://ntfy.sh/x', token: 'secret-token',
      });
      return <ChannelEditor value={value} onChange={setValue} onRemove={vi.fn()} onTest={vi.fn()} />;
    }
    render(<Harness />);
    await userEvent.selectOptions(screen.getByLabelText('Type'), 'webhook');
    expect(screen.queryByLabelText('Token')).not.toBeInTheDocument();
    await userEvent.selectOptions(screen.getByLabelText('Type'), 'ntfy');
    expect(screen.getByLabelText('Token')).toHaveValue('secret-token');
  });

  it('calls onTest and shows the returned result', async () => {
    const onTest = vi.fn();
    render(
      <ChannelEditor value={webhookChannel} onChange={vi.fn()} onRemove={vi.fn()} onTest={onTest}
        testResult={{ ok: false, message: 'connection refused' }} />,
    );
    await userEvent.click(screen.getByRole('button', { name: 'Test hook' }));
    expect(onTest).toHaveBeenCalled();
    expect(screen.getByText('connection refused')).toBeInTheDocument();
  });

  it('calls onRemove from the Remove button', async () => {
    const onRemove = vi.fn();
    render(
      <ChannelEditor value={webhookChannel} onChange={vi.fn()} onRemove={onRemove} onTest={vi.fn()} />,
    );
    await userEvent.click(screen.getByRole('button', { name: 'Remove hook' }));
    expect(onRemove).toHaveBeenCalled();
  });

  it('disables Test and shows a save-first hint when unsaved', async () => {
    const onTest = vi.fn();
    render(
      <ChannelEditor value={webhookChannel} onChange={vi.fn()} onRemove={vi.fn()} onTest={onTest} unsaved />,
    );
    const testButton = screen.getByRole('button', { name: 'Test hook' });
    expect(testButton).toBeDisabled();
    expect(screen.getByText('Save first to test this channel')).toBeInTheDocument();
    await userEvent.click(testButton);
    expect(onTest).not.toHaveBeenCalled();
  });

  it('joins tags for display and splits edits back into an array', async () => {
    const onChange = vi.fn();
    const ntfyChannel: NotifyChannel = {
      id: 'c2', type: 'ntfy', name: 'phone', enabled: true, url: 'https://ntfy.sh/x', tags: ['a', 'b'],
    };
    render(
      <ChannelEditor value={ntfyChannel} onChange={onChange} onRemove={vi.fn()} onTest={vi.fn()} />,
    );
    expect(screen.getByLabelText('Tags')).toHaveValue('a, b');
    await userEvent.type(screen.getByLabelText('Tags'), 'c');
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ tags: ['a', 'bc'] }));
  });
});
