import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
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

  it('drops the previous type fields when the type changes', async () => {
    const onChange = vi.fn();
    render(
      <ChannelEditor value={webhookChannel} onChange={onChange} onRemove={vi.fn()} onTest={vi.fn()} />,
    );
    await userEvent.selectOptions(screen.getByLabelText('Type'), 'ntfy');
    expect(onChange).toHaveBeenCalledWith({
      id: 'c1', type: 'ntfy', name: 'hook', enabled: true, url: 'https://hook',
    });
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
