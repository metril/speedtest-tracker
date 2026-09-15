import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { ListInput } from './ListInput';

describe('ListInput', () => {
  it('adds a value on Enter', async () => {
    const onChange = vi.fn();
    render(<ListInput id="l" label="Tags" value={[]} onChange={onChange} />);
    await userEvent.type(screen.getByLabelText('Tags'), 'foo{Enter}');
    expect(onChange).toHaveBeenCalledWith(['foo']);
  });

  it('adds a value on comma', async () => {
    const onChange = vi.fn();
    render(<ListInput id="l" label="Tags" value={[]} onChange={onChange} />);
    await userEvent.type(screen.getByLabelText('Tags'), 'foo,');
    expect(onChange).toHaveBeenCalledWith(['foo']);
  });

  it('commits on blur', async () => {
    const onChange = vi.fn();
    render(
      <>
        <ListInput id="l" label="Tags" value={[]} onChange={onChange} />
        <button>elsewhere</button>
      </>,
    );
    await userEvent.type(screen.getByLabelText('Tags'), 'foo');
    await userEvent.click(screen.getByText('elsewhere'));
    expect(onChange).toHaveBeenCalledWith(['foo']);
  });

  it('dedupes values', async () => {
    const onChange = vi.fn();
    render(<ListInput id="l" label="Tags" value={['foo']} onChange={onChange} />);
    await userEvent.type(screen.getByLabelText('Tags'), 'foo{Enter}');
    expect(onChange).not.toHaveBeenCalled();
  });

  it('removes a chip via its remove button', async () => {
    const onChange = vi.fn();
    render(<ListInput id="l" label="Tags" value={['foo', 'bar']} onChange={onChange} />);
    await userEvent.click(screen.getByRole('button', { name: 'Remove foo' }));
    expect(onChange).toHaveBeenCalledWith(['bar']);
  });

  it('removes the last chip on Backspace when draft is empty', async () => {
    const onChange = vi.fn();
    render(<ListInput id="l" label="Tags" value={['foo', 'bar']} onChange={onChange} />);
    await userEvent.type(screen.getByLabelText('Tags'), '{Backspace}');
    expect(onChange).toHaveBeenCalledWith(['foo']);
  });

  it('marks invalid chips using validate', () => {
    render(
      <ListInput
        id="l"
        label="Tags"
        value={['bad']}
        onChange={vi.fn()}
        validate={(s) => (s === 'bad' ? 'nope' : undefined)}
      />,
    );
    expect(screen.getByText('bad')).toHaveAttribute('title', 'nope');
  });

  it('disables the input and remove buttons', () => {
    render(<ListInput id="l" label="Tags" value={['foo']} onChange={vi.fn()} disabled />);
    expect(screen.getByLabelText('Tags')).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Remove foo' })).toBeDisabled();
  });
});
