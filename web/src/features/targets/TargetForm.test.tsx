import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { TargetForm } from './TargetForm';

function wrap(node: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{node}</QueryClientProvider>);
}

describe('TargetForm', () => {
  it('shows the iperf3 option fields when iperf3 is chosen', () => {
    wrap(<TargetForm onSubmit={vi.fn()} onCancel={vi.fn()} submitting={false} />);

    fireEvent.change(screen.getByLabelText('Engine'), { target: { value: 'iperf3' } });
    expect(screen.getByLabelText('Host')).toBeInTheDocument();
    expect(screen.getByLabelText('Port')).toBeInTheDocument();
    expect(screen.getByLabelText('Parallel streams')).toBeInTheDocument();
    expect(screen.getByLabelText('Reverse (-R)')).toBeInTheDocument();
  });

  it('shows the cloudflare size fields when cloudflare is chosen', () => {
    wrap(<TargetForm onSubmit={vi.fn()} onCancel={vi.fn()} submitting={false} />);

    fireEvent.change(screen.getByLabelText('Engine'), { target: { value: 'cloudflare' } });
    expect(screen.getByLabelText('Download sizes (bytes, comma separated)')).toBeInTheDocument();
    expect(screen.getByLabelText('Latency samples')).toBeInTheDocument();
  });

  it('submits name, engine, lane and typed options', () => {
    const onSubmit = vi.fn();
    wrap(<TargetForm onSubmit={onSubmit} onCancel={vi.fn()} submitting={false} />);

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'NAS' } });
    fireEvent.change(screen.getByLabelText('Engine'), { target: { value: 'iperf3' } });
    fireEvent.change(screen.getByLabelText('Lane'), { target: { value: 'lan' } });
    fireEvent.change(screen.getByLabelText('Host'), { target: { value: '10.0.0.5' } });
    fireEvent.change(screen.getByLabelText('Port'), { target: { value: '5201' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save target' }));

    expect(onSubmit).toHaveBeenCalledWith({
      name: 'NAS',
      engine: 'iperf3',
      enabled: true,
      lane: 'lan',
      options: { host: '10.0.0.5', port: 5201 },
    });
  });

  it('refuses to submit without a name', () => {
    const onSubmit = vi.fn();
    wrap(<TargetForm onSubmit={onSubmit} onCancel={vi.fn()} submitting={false} />);

    fireEvent.click(screen.getByRole('button', { name: 'Save target' }));
    expect(onSubmit).not.toHaveBeenCalled();
    expect(screen.getByText('Name is required')).toBeInTheDocument();
  });

  it('pre-fills from an existing target', () => {
    wrap(
      <TargetForm
        initial={{
          id: 4, name: 'Home', engine: 'ookla', enabled: false, lane: 'wan',
          options: { server_id: 1234 }, thresholds: {},
          created_at: '', updated_at: '',
        }}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        submitting={false}
      />,
    );
    expect(screen.getByLabelText('Name')).toHaveValue('Home');
    expect(screen.getByLabelText('Enabled')).not.toBeChecked();
    expect(screen.getByLabelText('Ookla server ID')).toHaveValue('1234');
  });
});
