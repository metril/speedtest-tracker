import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { FormField } from './FormField';

describe('FormField', () => {
  it('associates the label with the control', () => {
    render(
      <FormField id="f-name" label="Name">
        <input id="f-name" />
      </FormField>
    );
    expect(screen.getByLabelText('Name')).toBeInTheDocument();
  });

  it('shows a hint tooltip trigger when a hint is given', () => {
    render(
      <FormField id="f-hint" label="Name" hint="Pick something memorable">
        <input id="f-hint" />
      </FormField>
    );
    expect(screen.getByRole('button', { name: 'Pick something memorable' })).toBeInTheDocument();
  });

  it('shows the error and still renders the hint tooltip', () => {
    render(
      <FormField id="f-error" label="Name" hint="Pick something memorable" error="Name is required">
        <input id="f-error" />
      </FormField>
    );
    expect(screen.getByText('Name is required')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Pick something memorable' })).toBeInTheDocument();
  });

  it('renders no hint tooltip, action, or error when none are given', () => {
    const { container } = render(
      <FormField id="f-plain" label="Name">
        <input id="f-plain" />
      </FormField>
    );
    expect(container.querySelectorAll('button, p')).toHaveLength(0);
  });

  it('renders an action in the label row', () => {
    render(
      <FormField id="f-action" label="Name" action={<button type="button">Do thing</button>}>
        <input id="f-action" />
      </FormField>
    );
    expect(screen.getByRole('button', { name: 'Do thing' })).toBeInTheDocument();
  });
});
