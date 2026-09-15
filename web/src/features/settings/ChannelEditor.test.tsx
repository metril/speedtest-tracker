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
    expect(screen.queryByLabelText('Apprise URLs')).not.toBeInTheDocument();
  });

  it('keeps prior-type fields (e.g. Apprise URLs) in state across a type flip-flop', async () => {
    // A stateful harness stands in for Settings.tsx's controlled state, so
    // this exercises the real onChange -> value round trip a flip-flop
    // (apprise -> webhook -> apprise) goes through in the app.
    function Harness() {
      const [value, setValue] = useState<NotifyChannel>({
        id: 'c2', type: 'apprise', name: 'phone', enabled: true, url: '', urls: ['ntfy://example.com/topic'],
      });
      return <ChannelEditor value={value} onChange={setValue} onRemove={vi.fn()} onTest={vi.fn()} />;
    }
    render(<Harness />);
    await userEvent.selectOptions(screen.getByLabelText('Type'), 'webhook');
    expect(screen.queryByLabelText('Apprise URLs')).not.toBeInTheDocument();
    await userEvent.selectOptions(screen.getByLabelText('Type'), 'apprise');
    expect(screen.getByLabelText('Apprise URLs')).toHaveValue('ntfy://example.com/topic');
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
    const appriseChannel: NotifyChannel = {
      id: 'c2', type: 'apprise', name: 'phone', enabled: true, url: '', tags: ['a', 'b'],
    };
    render(
      <ChannelEditor value={appriseChannel} onChange={onChange} onRemove={vi.fn()} onTest={vi.fn()} />,
    );
    expect(screen.getByLabelText('Tags')).toHaveValue('a, b');
    await userEvent.type(screen.getByLabelText('Tags'), 'c');
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ tags: ['a', 'bc'] }));
  });
});
