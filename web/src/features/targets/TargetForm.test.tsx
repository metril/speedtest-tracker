import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { createContext, useContext, type ReactNode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { TargetForm } from './TargetForm';

// TargetForm renders EngineOptionFields' Ookla/iperf3 server pickers, which
// wrap their results in a Radix Popover. Rendering Radix's real
// PopoverContent (its floating-ui Popper positioning) with open=true hangs
// jsdom for ~25-30s regardless of how `open` became true — see the same
// mock and its longer rationale comment in EngineOptionFields.test.tsx. This
// file doesn't test popover open/close behavior itself, so a minimal
// stand-in that just renders children (always, for the always-visible
// anchored input; only-when-open for the results list) is enough to keep
// the tests below from tripping over it.
const PopoverOpenContext = createContext(false);

vi.mock('@/components/ui/popover', () => ({
  Popover: ({ open, children }: { open: boolean; children: ReactNode }) => (
    <PopoverOpenContext.Provider value={open}>{children}</PopoverOpenContext.Provider>
  ),
  PopoverAnchor: ({ children }: { children: ReactNode }) => children,
  PopoverContent: ({ children }: { children: ReactNode }) => {
    const open = useContext(PopoverOpenContext);
    return open ? <div>{children}</div> : null;
  },
}));

function wrap(node: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{node}</QueryClientProvider>);
}

describe('TargetForm', () => {
  it('shows the iperf3 option fields when iperf3 is chosen', () => {
    wrap(<TargetForm onSubmit={vi.fn()} onCancel={vi.fn()} submitting={false} />);

    fireEvent.change(screen.getByLabelText('Engine'), { target: { value: 'iperf3' } });
    expect(screen.getByLabelText('Host')).toBeInTheDocument();
    // Advanced fields start collapsed behind the disclosure on a fresh target.
    expect(screen.queryByLabelText('Port')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Advanced options' }));
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
    fireEvent.click(screen.getByRole('button', { name: 'Advanced options' }));
    fireEvent.change(screen.getByLabelText('Port'), { target: { value: '5201' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save target' }));

    expect(onSubmit).toHaveBeenCalledWith({
      name: 'NAS',
      engine: 'iperf3',
      enabled: true,
      lane: 'lan',
      options: { host: '10.0.0.5', port: 5201 },
      thresholds: {},
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

  it('includes a filled iperf3 password in the submitted options', () => {
    const onSubmit = vi.fn();
    wrap(<TargetForm onSubmit={onSubmit} onCancel={vi.fn()} submitting={false} />);

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'NAS' } });
    fireEvent.change(screen.getByLabelText('Engine'), { target: { value: 'iperf3' } });
    fireEvent.change(screen.getByLabelText('Host'), { target: { value: '10.0.0.5' } });
    fireEvent.click(screen.getByRole('button', { name: 'Advanced options' }));
    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'alice' } });
    fireEvent.change(screen.getByLabelText('RSA public key path'), { target: { value: '/keys/pub.pem' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'sekrit' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save target' }));

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ options: expect.objectContaining({ password: 'sekrit' }) }),
    );
  });

  it('exposes the iperf3 password field as a password input', () => {
    wrap(<TargetForm onSubmit={vi.fn()} onCancel={vi.fn()} submitting={false} />);
    fireEvent.change(screen.getByLabelText('Engine'), { target: { value: 'iperf3' } });
    fireEvent.click(screen.getByRole('button', { name: 'Advanced options' }));
    expect(screen.getByLabelText('Password')).toHaveAttribute('type', 'password');
  });

  it('disables reverse and bidir from being checked together', () => {
    wrap(<TargetForm onSubmit={vi.fn()} onCancel={vi.fn()} submitting={false} />);
    fireEvent.change(screen.getByLabelText('Engine'), { target: { value: 'iperf3' } });
    fireEvent.click(screen.getByRole('button', { name: 'Advanced options' }));

    fireEvent.click(screen.getByLabelText('Reverse (-R)'));
    expect(screen.getByLabelText('Bidirectional (--bidir)')).toBeDisabled();

    fireEvent.click(screen.getByLabelText('Reverse (-R)'));
    fireEvent.click(screen.getByLabelText('Bidirectional (--bidir)'));
    expect(screen.getByLabelText('Reverse (-R)')).toBeDisabled();
  });

  it('lets legacy options with both reverse and bidir true be unchecked', () => {
    wrap(
      <TargetForm
        initial={{
          id: 9, name: 'Legacy', engine: 'iperf3', enabled: true, lane: 'wan',
          options: { reverse: true, bidir: true }, thresholds: {},
          created_at: '', updated_at: '',
        }}
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        submitting={false}
      />,
    );

    // Neither box is locked out just because both happen to be checked.
    expect(screen.getByLabelText('Reverse (-R)')).not.toBeDisabled();
    expect(screen.getByLabelText('Bidirectional (--bidir)')).not.toBeDisabled();

    fireEvent.click(screen.getByLabelText('Reverse (-R)'));
    expect(screen.getByLabelText('Reverse (-R)')).not.toBeChecked();
    expect(screen.getByLabelText('Bidirectional (--bidir)')).toBeChecked();
  });

  it('only enables udp bitrate when protocol is udp, and disables bidir for udp', () => {
    wrap(<TargetForm onSubmit={vi.fn()} onCancel={vi.fn()} submitting={false} />);
    fireEvent.change(screen.getByLabelText('Engine'), { target: { value: 'iperf3' } });
    fireEvent.click(screen.getByRole('button', { name: 'Advanced options' }));

    expect(screen.getByLabelText('UDP bitrate')).toBeDisabled();
    expect(screen.getByLabelText('Bidirectional (--bidir)')).not.toBeDisabled();

    fireEvent.change(screen.getByLabelText('Protocol'), { target: { value: 'udp' } });
    expect(screen.getByLabelText('UDP bitrate')).not.toBeDisabled();
    expect(screen.getByLabelText('Bidirectional (--bidir)')).toBeDisabled();
  });

  it('requires a username and rsa key when a password is set, blocking submit', () => {
    const onSubmit = vi.fn();
    wrap(<TargetForm onSubmit={onSubmit} onCancel={vi.fn()} submitting={false} />);

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'NAS' } });
    fireEvent.change(screen.getByLabelText('Engine'), { target: { value: 'iperf3' } });
    fireEvent.change(screen.getByLabelText('Host'), { target: { value: '10.0.0.5' } });
    fireEvent.click(screen.getByRole('button', { name: 'Advanced options' }));
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'sekrit' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save target' }));

    expect(onSubmit).not.toHaveBeenCalled();
    // The inline hint next to the Password field and the form-level
    // message next to Save both show the same blocking error.
    expect(screen.getAllByText('Password requires a username and RSA public key path')).toHaveLength(2);
  });

  it('shows the blocking options error next to Save and reopens a collapsed advanced disclosure on failed submit', () => {
    const onSubmit = vi.fn();
    wrap(<TargetForm onSubmit={onSubmit} onCancel={vi.fn()} submitting={false} />);

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'NAS' } });
    fireEvent.change(screen.getByLabelText('Engine'), { target: { value: 'iperf3' } });
    fireEvent.change(screen.getByLabelText('Host'), { target: { value: '10.0.0.5' } });

    // Open the disclosure just to set the password, then collapse it again
    // -- Save must still surface the error and reopen it, not fail silently.
    const toggle = screen.getByRole('button', { name: 'Advanced options' });
    fireEvent.click(toggle);
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'sekrit' } });
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByLabelText('Password')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Save target' }));

    expect(onSubmit).not.toHaveBeenCalled();
    expect(toggle).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByLabelText('Password')).toHaveValue('sekrit');
    expect(screen.getAllByText('Password requires a username and RSA public key path')).toHaveLength(2);
  });

  it('debounces the ookla server search so one request fires per pause', async () => {
    vi.useFakeTimers();
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'ok',
      text: async () => JSON.stringify([]),
    });
    vi.stubGlobal('fetch', fetchMock);

    wrap(<TargetForm onSubmit={vi.fn()} onCancel={vi.fn()} submitting={false} />);
    const search = screen.getByLabelText('Search servers');
    fireEvent.change(search, { target: { value: 'lo' } });
    fireEvent.change(search, { target: { value: 'lon' } });
    fireEvent.change(search, { target: { value: 'lond' } });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(299);
    });
    expect(fetchMock).not.toHaveBeenCalled();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(String(fetchMock.mock.calls[0][0])).toContain('lond');

    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('submits per-target thresholds, omitting blank fields', async () => {
    const onSubmit = vi.fn();
    wrap(<TargetForm onSubmit={onSubmit} onCancel={() => {}} submitting={false} />);
    await userEvent.type(screen.getByLabelText('Name'), 'Home');
    await userEvent.type(screen.getByLabelText('Min download (Mbps)'), '100');
    await userEvent.click(screen.getByLabelText('Notify on failed test'));
    await userEvent.click(screen.getByRole('button', { name: 'Save target' }));
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({
      thresholds: { download_mbps_min: 100, notify_on_failure: true },
    }));
  });

  it('seeds threshold fields from an existing target', async () => {
    wrap(<TargetForm initial={{
      id: 4, name: 'Home', engine: 'ookla', enabled: true, lane: 'wan', options: {},
      thresholds: { ping_ms_max: 40 }, created_at: '', updated_at: '',
    }} onSubmit={() => {}} onCancel={() => {}} submitting={false} />);
    expect(screen.getByLabelText('Max ping (ms)')).toHaveValue(40);
    expect(screen.getByLabelText('Min download (Mbps)')).toHaveValue(null);
  });

  it('rejects a negative threshold', async () => {
    const onSubmit = vi.fn();
    wrap(<TargetForm onSubmit={onSubmit} onCancel={() => {}} submitting={false} />);
    await userEvent.type(screen.getByLabelText('Name'), 'Home');
    await userEvent.type(screen.getByLabelText('Max packet loss (%)'), '-5');
    await userEvent.click(screen.getByRole('button', { name: 'Save target' }));
    expect(onSubmit).not.toHaveBeenCalled();
    expect(screen.getByText(/must be zero or more/i)).toBeInTheDocument();
  });

  it('submits a per-target SLA plan override, omitting blank fields', async () => {
    const onSubmit = vi.fn();
    wrap(<TargetForm onSubmit={onSubmit} onCancel={() => {}} submitting={false} />);
    await userEvent.type(screen.getByLabelText('Name'), 'Home');
    await userEvent.type(screen.getByLabelText('SLA plan download override (Mbps)'), '500');
    await userEvent.click(screen.getByRole('button', { name: 'Save target' }));
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({
      thresholds: { sla_download_mbps: 500 },
    }));
  });

  it('rejects a negative SLA plan override', async () => {
    const onSubmit = vi.fn();
    wrap(<TargetForm onSubmit={onSubmit} onCancel={() => {}} submitting={false} />);
    await userEvent.type(screen.getByLabelText('Name'), 'Home');
    await userEvent.type(screen.getByLabelText('SLA plan upload override (Mbps)'), '-1');
    await userEvent.click(screen.getByRole('button', { name: 'Save target' }));
    expect(onSubmit).not.toHaveBeenCalled();
    expect(screen.getByText(/must be zero or more/i)).toBeInTheDocument();
  });
});
