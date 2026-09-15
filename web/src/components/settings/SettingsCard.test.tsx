import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { SettingsCard } from './SettingsCard';

describe('SettingsCard', () => {
  it('renders title, description, header action, rows and footer', () => {
    render(
      <SettingsCard
        title="General"
        description="Basics"
        headerAction={<button>Action</button>}
        footer={<span>Footer text</span>}
      >
        <div>Row one</div>
      </SettingsCard>,
    );
    expect(screen.getByRole('region')).toBeInTheDocument();
    expect(screen.getByText('General')).toBeInTheDocument();
    expect(screen.getByText('Basics')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Action' })).toBeInTheDocument();
    expect(screen.getByText('Row one')).toBeInTheDocument();
    expect(screen.getByText('Footer text')).toBeInTheDocument();
  });

  it('omits the footer when not given', () => {
    render(
      <SettingsCard title="General">
        <div>Row</div>
      </SettingsCard>,
    );
    expect(screen.queryByText('Footer text')).not.toBeInTheDocument();
  });
});
