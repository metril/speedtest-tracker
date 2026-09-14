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

  it('shows a hint when no error is present', () => {
    render(
      <FormField id="f-hint" label="Name" hint="Pick something memorable">
        <input id="f-hint" />
      </FormField>
    );
    expect(screen.getByText('Pick something memorable')).toBeInTheDocument();
  });

  it('shows an error instead of the hint', () => {
    render(
      <FormField id="f-error" label="Name" hint="Pick something memorable" error="Name is required">
        <input id="f-error" />
      </FormField>
    );
    expect(screen.getByText('Name is required')).toBeInTheDocument();
    expect(screen.queryByText('Pick something memorable')).not.toBeInTheDocument();
  });

  it('renders no hint or error text when neither is given', () => {
    const { container } = render(
      <FormField id="f-plain" label="Name">
        <input id="f-plain" />
      </FormField>
    );
    expect(container.querySelectorAll('p')).toHaveLength(0);
  });
});
