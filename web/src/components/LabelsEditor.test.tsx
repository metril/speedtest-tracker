import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { LabelsEditor } from './LabelsEditor';

describe('LabelsEditor', () => {
  it('adds and removes label pairs', async () => {
    const onChange = vi.fn();
    render(<LabelsEditor label="Extra labels" value={{ host: 'pi4' }} onChange={onChange} />);
    await userEvent.click(screen.getByRole('button', { name: 'Remove host' }));
    expect(onChange).toHaveBeenCalledWith({});
  });

  it('adds a new pair from the draft row', async () => {
    const onChange = vi.fn();
    render(<LabelsEditor label="Extra labels" value={{}} onChange={onChange} />);
    await userEvent.type(screen.getByLabelText('New Extra labels key'), 'region');
    await userEvent.type(screen.getByLabelText('New Extra labels value'), 'eu');
    await userEvent.click(screen.getByRole('button', { name: 'Add label' }));
    expect(onChange).toHaveBeenCalledWith({ region: 'eu' });
  });

  it('edits an existing value', async () => {
    const onChange = vi.fn();
    render(<LabelsEditor label="Extra labels" value={{ host: 'pi4' }} onChange={onChange} />);
    await userEvent.type(screen.getByLabelText('Extra labels value for host'), '2');
    expect(onChange).toHaveBeenLastCalledWith({ host: 'pi42' });
  });
});
