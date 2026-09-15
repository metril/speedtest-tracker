import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { KeyValueInput } from './KeyValueInput';

describe('KeyValueInput', () => {
  it('adds and removes label pairs', async () => {
    const onChange = vi.fn();
    render(<KeyValueInput label="Extra labels" value={{ host: 'pi4' }} onChange={onChange} />);
    await userEvent.click(screen.getByRole('button', { name: 'Remove host' }));
    expect(onChange).toHaveBeenCalledWith({});
  });

  it('adds a new pair from the draft row', async () => {
    const onChange = vi.fn();
    render(<KeyValueInput label="Extra labels" value={{}} onChange={onChange} />);
    await userEvent.type(screen.getByLabelText('New Extra labels key'), 'region');
    await userEvent.type(screen.getByLabelText('New Extra labels value'), 'eu');
    await userEvent.click(screen.getByRole('button', { name: 'Add label' }));
    expect(onChange).toHaveBeenCalledWith({ region: 'eu' });
  });

  it('edits an existing value', async () => {
    const onChange = vi.fn();
    render(<KeyValueInput label="Extra labels" value={{ host: 'pi4' }} onChange={onChange} />);
    await userEvent.type(screen.getByLabelText('Extra labels value for host'), '2');
    expect(onChange).toHaveBeenLastCalledWith({ host: 'pi42' });
  });

  it('keeps focus in the key input while typing a rename', async () => {
    const onChange = vi.fn();
    render(<KeyValueInput label="Extra labels" value={{ host: 'pi4' }} onChange={onChange} />);
    const keyInput = screen.getByLabelText('Extra labels key');
    await userEvent.click(keyInput);
    await userEvent.type(keyInput, 'x');
    expect(document.activeElement).toBe(keyInput);
    expect(onChange).toHaveBeenLastCalledWith({ hostx: 'pi4' });
  });

  it('flags a duplicate key and keeps only the first in the emitted map', async () => {
    const onChange = vi.fn();
    render(
      <KeyValueInput label="Extra labels" value={{ host: 'pi4', region: 'eu' }} onChange={onChange} />,
    );
    const keyInputs = screen.getAllByLabelText('Extra labels key');
    await userEvent.clear(keyInputs[1]);
    await userEvent.type(keyInputs[1], 'host');
    expect(screen.getAllByText(/Duplicate key/).length).toBeGreaterThan(0);
    expect(onChange).toHaveBeenLastCalledWith({ host: 'pi4' });
  });

  it('flags an empty key and excludes it from the emitted map', async () => {
    const onChange = vi.fn();
    render(<KeyValueInput label="Extra labels" value={{ host: 'pi4' }} onChange={onChange} />);
    await userEvent.clear(screen.getByLabelText('Extra labels key'));
    expect(screen.getByText(/Key is required/)).toBeInTheDocument();
    expect(onChange).toHaveBeenLastCalledWith({});
  });
});
