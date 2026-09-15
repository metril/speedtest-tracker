import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { describe, expect, it } from 'vitest';
import type { ExportAuth } from '../../lib/api';
import { ExportAuthFields } from './ExportAuthFields';

function Harness({ initial }: { initial: ExportAuth }) {
  const [value, setValue] = useState(initial);
  return <ExportAuthFields idPrefix="vm" label="VictoriaMetrics auth" value={value} onChange={setValue} />;
}

describe('ExportAuthFields', () => {
  it('shows no extra fields for type none', () => {
    render(<Harness initial={{ type: 'none' }} />);
    expect(screen.queryByLabelText(/username/i)).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/token/i)).not.toBeInTheDocument();
  });

  it('shows username and password fields for basic', async () => {
    render(<Harness initial={{ type: 'none' }} />);
    await userEvent.selectOptions(screen.getByLabelText('VictoriaMetrics auth'), 'basic');
    expect(screen.getByLabelText('VictoriaMetrics auth username')).toBeInTheDocument();
    expect(screen.getByLabelText('VictoriaMetrics auth password')).toBeInTheDocument();
  });

  it('shows a token field for bearer', async () => {
    render(<Harness initial={{ type: 'none' }} />);
    await userEvent.selectOptions(screen.getByLabelText('VictoriaMetrics auth'), 'bearer');
    expect(screen.getByLabelText('VictoriaMetrics auth token')).toBeInTheDocument();
    expect(screen.queryByLabelText('VictoriaMetrics auth username')).not.toBeInTheDocument();
  });

  it('shows header name and value fields for custom, swapping away basic fields', async () => {
    render(<Harness initial={{ type: 'basic' }} />);
    expect(screen.getByLabelText('VictoriaMetrics auth username')).toBeInTheDocument();
    await userEvent.selectOptions(screen.getByLabelText('VictoriaMetrics auth'), 'custom');
    expect(screen.getByLabelText('VictoriaMetrics auth header name')).toBeInTheDocument();
    expect(screen.getByLabelText('VictoriaMetrics auth header value')).toBeInTheDocument();
    expect(screen.queryByLabelText('VictoriaMetrics auth username')).not.toBeInTheDocument();
  });

  it('uses "leave unchanged" placeholders on secret fields', async () => {
    render(<Harness initial={{ type: 'bearer', token: '***' }} />);
    expect(screen.getByLabelText('VictoriaMetrics auth token')).toHaveAttribute('placeholder', 'leave unchanged');
  });
});
