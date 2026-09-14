import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  act, fireEvent, render, screen, waitFor, within,
} from '@testing-library/react';
import {
  createContext, useContext, useState, type ReactNode,
} from 'react';
import {
  afterEach, beforeEach, describe, expect, it, vi,
} from 'vitest';
import type { Iperf3Server, OoklaServer } from '../../lib/api';
import { EngineOptionFields, Iperf3ResultsList, OoklaResultsList } from './EngineOptionFields';

// Rendering Radix's real PopoverContent (its floating-ui Popper positioning)
// with open=true hangs jsdom for ~25-30s regardless of how `open` became
// true — confirmed with a minimal repro (Popover+PopoverAnchor+PopoverContent,
// no app code involved) while fixing the "typing after the popover closes
// itself never reopens it" bug. jsdom's ResizeObserver is already stubbed in
// test-setup.ts, so that's not it; something in floating-ui's autoUpdate loop
// against jsdom's always-zero layout never settles.
//
// So this file replaces '@/components/ui/popover' with a lightweight stand-in
// that preserves the real open/close *state machine* our components drive
// (Popover always renders its children so the anchored <input> stays
// visible; PopoverContent only renders when `open` is true) without any of
// Radix's real dismiss/positioning machinery. A hidden
// `mock-popover-force-close` button simulates what a real outside
// interaction or Escape does (call onOpenChange(false)), which lets
// "OoklaFields: reopens after closing" below exercise the actual fix
// (typing after a close sets `focused` back to true) — the outside-click
// *detection* itself (onInteractOutside) can't be exercised in jsdom, per
// the above.
const PopoverOpenContext = createContext(false);

vi.mock('@/components/ui/popover', () => ({
  Popover: ({ open, onOpenChange, children }: {
    open: boolean; onOpenChange?: (open: boolean) => void; children: ReactNode;
  }) => (
    <PopoverOpenContext.Provider value={open}>
      {children}
      <button
        type="button"
        data-testid="mock-popover-force-close"
        onClick={() => onOpenChange?.(false)}
      />
    </PopoverOpenContext.Provider>
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

/** jsonResponse builds a fetch Response-shaped stub the same way the rest
 * of this codebase's tests do (see Targets.test.tsx). */
function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status < 400, status, statusText: 'ok', text: async () => JSON.stringify(body),
  } as Response;
}

const denver: OoklaServer = {
  id: '101', name: 'Comcast', location: 'Denver, CO', country: 'United States',
  host: 'denver.example:8080', sponsor: 'Comcast', lat: 39.7, lon: -104.9, distance_km: 1.2,
};
const boulder: OoklaServer = {
  id: '102', name: 'Xfinity', location: 'Boulder, CO', country: 'United States',
  host: 'boulder.example:8080', sponsor: 'Xfinity', lat: 40.0, lon: -105.3, distance_km: 40,
};

const frankfurtIperf: Iperf3Server = {
  id: 1, host: 'iperf.example.net', port: 5201, port_end: 5210, options: '-R,-u',
  supports_reverse: true, supports_udp: true, supports_ipv6: true, gbs: '10',
  country: 'DE', continent: 'Europe', site: 'Frankfurt', provider: 'Example Net',
};
const denverIperf: Iperf3Server = {
  id: 2, host: 'speed.other.net', port: 5202, supports_reverse: false, supports_udp: false, supports_ipv6: false,
  country: 'US', site: 'Denver', provider: 'Other Net',
};

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
});
afterEach(() => {
  vi.unstubAllGlobals();
});

describe('EngineOptionFields: non-ookla engines unaffected', () => {
  it('renders a message for engines with no configuration', () => {
    wrap(<EngineOptionFields engine="fake" options={{}} onChange={vi.fn()} />);
    expect(screen.getByText('fake')).toBeInTheDocument();
  });

  it('still renders the cloudflare size fields', () => {
    wrap(<EngineOptionFields engine="cloudflare" options={{}} onChange={vi.fn()} />);
    expect(screen.getByText('Download sizes')).toBeInTheDocument();
  });

  it('renders the iperf3 host field once Custom is switched on', () => {
    wrap(<EngineOptionFields engine="iperf3" options={{}} onChange={vi.fn()} />);
    expect(screen.queryByLabelText('Host')).not.toBeInTheDocument();
    fireEvent.click(screen.getByLabelText('Custom'));
    expect(screen.getByLabelText('Host')).toBeInTheDocument();
  });
});

describe('CloudflareFields: presets', () => {
  // "1 MB" (1e6) is a preset for both download and upload sizes, so the
  // download chip is always the first of the two matches in DOM order.
  function downloadChip(name: string) {
    return screen.getAllByRole('checkbox', { name })[0];
  }

  it('checks the default sizes and latency with empty options', () => {
    wrap(<EngineOptionFields engine="cloudflare" options={{}} onChange={vi.fn()} />);
    expect(downloadChip('1 MB')).toBeChecked();
    expect(downloadChip('10 MB')).toBeChecked();
    expect(downloadChip('25 MB')).toBeChecked();
    expect(downloadChip('100 MB')).toBeChecked();
    expect(screen.getAllByRole('checkbox', { name: '100 KB' })[1]).toBeChecked(); // upload default
    expect(screen.getByLabelText('Latency samples')).toHaveValue('10');
  });

  it('unchecking a default download size emits the remaining sizes', () => {
    const onChange = vi.fn();
    wrap(<EngineOptionFields engine="cloudflare" options={{}} onChange={onChange} />);
    fireEvent.click(downloadChip('1 MB'));
    expect(onChange).toHaveBeenCalledWith({ download_sizes: [1e7, 2.5e7, 1e8] });
  });

  it('restoring the default set omits the key entirely', () => {
    const onChange = vi.fn();
    wrap(
      <EngineOptionFields
        engine="cloudflare" options={{ download_sizes: [1e7, 2.5e7, 1e8] }} onChange={onChange}
      />,
    );
    fireEvent.click(downloadChip('1 MB'));
    expect(onChange).toHaveBeenCalledWith({});
  });

  it('selecting the default latency (10) omits the key', () => {
    const onChange = vi.fn();
    wrap(<EngineOptionFields engine="cloudflare" options={{ latency_samples: 20 }} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText('Latency samples'), { target: { value: '10' } });
    expect(onChange).toHaveBeenCalledWith({});
  });

  it('selecting a non-default latency sets the key', () => {
    const onChange = vi.fn();
    wrap(<EngineOptionFields engine="cloudflare" options={{}} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText('Latency samples'), { target: { value: '20' } });
    expect(onChange).toHaveBeenCalledWith({ latency_samples: 20 });
  });
});

describe('CloudflareFields: Custom sizes toggle', () => {
  it('starts off with default options and reveals rows once switched on', () => {
    wrap(<EngineOptionFields engine="cloudflare" options={{}} onChange={vi.fn()} />);
    const toggle = screen.getByLabelText('Custom sizes');
    expect(toggle).toHaveAttribute('aria-checked', 'false');
    expect(screen.queryByLabelText('download size 1')).not.toBeInTheDocument();

    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute('aria-checked', 'true');
    expect(screen.getByLabelText('download size 1')).toBeInTheDocument();
  });

  it('starts on when a stored size is not one of the presets', () => {
    wrap(
      <EngineOptionFields engine="cloudflare" options={{ download_sizes: [123456] }} onChange={vi.fn()} />,
    );
    expect(screen.getByLabelText('Custom sizes')).toHaveAttribute('aria-checked', 'true');
    expect(screen.getByLabelText('download size 1')).toHaveValue('123456');
  });

  it('committing a custom row converts value+unit to bytes', () => {
    const onChange = vi.fn();
    wrap(
      <EngineOptionFields engine="cloudflare" options={{ download_sizes: [123456] }} onChange={onChange} />,
    );
    fireEvent.change(screen.getByLabelText('download size 1'), { target: { value: '1.5' } });
    fireEvent.change(screen.getByLabelText('download size 1 unit'), { target: { value: 'MB' } });
    expect(onChange).toHaveBeenLastCalledWith({ download_sizes: [1_500_000] });
  });
});

describe('CloudflareFields: custom rows round to whole bytes', () => {
  it('rounds a 1.5 B row up to 2 bytes', () => {
    const onChange = vi.fn();
    wrap(
      <EngineOptionFields engine="cloudflare" options={{ download_sizes: [123456] }} onChange={onChange} />,
    );
    // The seeded row starts at unit B (123456 doesn't divide evenly into KB).
    fireEvent.change(screen.getByLabelText('download size 1'), { target: { value: '1.5' } });
    expect(onChange).toHaveBeenLastCalledWith({ download_sizes: [2] });
  });

  it('rounds 16.1 MB (a non-exact float via toBytes) to a whole number', () => {
    const onChange = vi.fn();
    wrap(
      <EngineOptionFields engine="cloudflare" options={{ download_sizes: [123456] }} onChange={onChange} />,
    );
    fireEvent.change(screen.getByLabelText('download size 1'), { target: { value: '16.1' } });
    fireEvent.change(screen.getByLabelText('download size 1 unit'), { target: { value: 'MB' } });
    expect(onChange).toHaveBeenLastCalledWith({ download_sizes: [16_100_000] });
  });

  it('drops a row that rounds to <= 0', () => {
    const onChange = vi.fn();
    wrap(
      <EngineOptionFields engine="cloudflare" options={{ download_sizes: [123456] }} onChange={onChange} />,
    );
    fireEvent.change(screen.getByLabelText('download size 1'), { target: { value: '0.0001' } });
    expect(onChange).toHaveBeenLastCalledWith({});
  });
});

describe('CloudflareFields: custom latency validation', () => {
  it('rounds a fractional latency to a whole number', () => {
    const onChange = vi.fn();
    wrap(
      <EngineOptionFields engine="cloudflare" options={{ download_sizes: [123456] }} onChange={onChange} />,
    );
    fireEvent.change(screen.getByLabelText('Latency samples'), { target: { value: '2.5' } });
    expect(onChange).toHaveBeenLastCalledWith({ download_sizes: [123456], latency_samples: 3 });
  });

  it('drops a negative latency instead of sending it', () => {
    const onChange = vi.fn();
    wrap(
      <EngineOptionFields engine="cloudflare" options={{ download_sizes: [123456] }} onChange={onChange} />,
    );
    fireEvent.change(screen.getByLabelText('Latency samples'), { target: { value: '-3' } });
    expect(onChange).toHaveBeenLastCalledWith({ download_sizes: [123456] });
  });
});

describe('CloudflareFields: turning Custom sizes off', () => {
  it('drops a non-preset latency value so the preset <select> has no stale value', () => {
    const onChange = vi.fn();
    wrap(<EngineOptionFields engine="cloudflare" options={{ latency_samples: 7 }} onChange={onChange} />);
    // latency_samples: 7 isn't a preset, so Custom sizes starts on.
    expect(screen.getByLabelText('Custom sizes')).toHaveAttribute('aria-checked', 'true');
    fireEvent.click(screen.getByLabelText('Custom sizes'));
    expect(onChange).toHaveBeenCalledWith({});
  });
});

describe('CloudflareFields: emptying every preset chip', () => {
  function ControlledCloudflareFields() {
    const [options, setOptions] = useState<Record<string, unknown>>({});
    return <EngineOptionFields engine="cloudflare" options={options} onChange={setOptions} />;
  }

  it('shows "Using engine defaults" after unchecking every upload chip, and re-checks the defaults', () => {
    wrap(<ControlledCloudflareFields />);
    const uploadChips = within(screen.getByTestId('cf-upload-chips'));

    expect(screen.queryByText('Using engine defaults')).not.toBeInTheDocument();

    fireEvent.click(uploadChips.getByRole('checkbox', { name: '100 KB' }));
    fireEvent.click(uploadChips.getByRole('checkbox', { name: '1 MB' }));
    fireEvent.click(uploadChips.getByRole('checkbox', { name: '10 MB' }));

    expect(
      within(screen.getByTestId('cf-upload-chips').parentElement as HTMLElement)
        .getByText('Using engine defaults'),
    ).toBeInTheDocument();
    // The engine can't represent "no sizes", so the defaults come back checked.
    expect(uploadChips.getByRole('checkbox', { name: '100 KB' })).toBeChecked();
  });
});

describe('OoklaFields: manual server ID input', () => {
  it('keeps a manual server ID input independent of search', () => {
    const onChange = vi.fn();
    wrap(<EngineOptionFields engine="ookla" options={{ server_id: 1234 }} onChange={onChange} />);

    expect(screen.getByLabelText('Ookla server ID')).toHaveValue('1234');
    fireEvent.change(screen.getByLabelText('Ookla server ID'), { target: { value: '5678' } });
    expect(onChange).toHaveBeenCalledWith({ server_id: 5678 });
  });

  it('does not search until 2+ characters are typed', async () => {
    wrap(<EngineOptionFields engine="ookla" options={{}} onChange={vi.fn()} />);
    fireEvent.change(screen.getByLabelText('Search servers'), { target: { value: 'd' } });
    await act(async () => { await new Promise((r) => { setTimeout(r, 350); }); });
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

// The Popover mock above means these no longer risk the jsdom hang: they
// check that OoklaFields wires the debounced query and manual ID input
// correctly (and, below, that closing/reopening the picker works).
describe('OoklaFields: search wiring', () => {
  it('fetches with the typed query once the debounce settles', async () => {
    fetchMock.mockImplementation(async () => jsonResponse({ servers: [denver, boulder], near: '' }));
    wrap(<EngineOptionFields engine="ookla" options={{}} onChange={vi.fn()} />);

    fireEvent.change(screen.getByLabelText('Search servers'), { target: { value: 'denver' } });

    await waitFor(() => expect(fetchMock).toHaveBeenCalled(), { timeout: 1000 });
    const [url] = fetchMock.mock.calls[0];
    expect(String(url)).toContain('q=denver');
  });

  it('defaults the country hint from the browser locale and includes it in the query', async () => {
    const original = Object.getOwnPropertyDescriptor(window.navigator, 'language');
    Object.defineProperty(window.navigator, 'language', { value: 'en-GB', configurable: true });
    try {
      fetchMock.mockImplementation(async () => jsonResponse({ servers: [], near: '' }));
      wrap(<EngineOptionFields engine="ookla" options={{}} onChange={vi.fn()} />);

      expect(screen.getByLabelText('Country')).toHaveValue('GB');
      fireEvent.change(screen.getByLabelText('Search servers'), { target: { value: 'denver' } });
      await waitFor(() => expect(fetchMock).toHaveBeenCalled(), { timeout: 1000 });
      expect(String(fetchMock.mock.calls[0][0])).toContain('country=GB');
    } finally {
      if (original) Object.defineProperty(window.navigator, 'language', original);
    }
  });

  it('drops country from the query once "Any" is selected', async () => {
    fetchMock.mockImplementation(async () => jsonResponse({ servers: [], near: '' }));
    wrap(<EngineOptionFields engine="ookla" options={{}} onChange={vi.fn()} />);

    fireEvent.change(screen.getByLabelText('Country'), { target: { value: '' } });
    fireEvent.change(screen.getByLabelText('Search servers'), { target: { value: 'denver' } });
    await waitFor(() => expect(fetchMock).toHaveBeenCalled(), { timeout: 1000 });
    expect(String(fetchMock.mock.calls[0][0])).not.toContain('country=');
  });

  it('reveals a free-text 2-letter input when "Other…" is selected', async () => {
    fetchMock.mockImplementation(async () => jsonResponse({ servers: [], near: '' }));
    wrap(<EngineOptionFields engine="ookla" options={{}} onChange={vi.fn()} />);

    fireEvent.change(screen.getByLabelText('Country'), { target: { value: 'other' } });
    fireEvent.change(screen.getByLabelText('Country code'), { target: { value: 'ie' } });
    fireEvent.change(screen.getByLabelText('Search servers'), { target: { value: 'dublin' } });
    await waitFor(() => expect(fetchMock).toHaveBeenCalled(), { timeout: 1000 });
    expect(String(fetchMock.mock.calls[0][0])).toContain('country=IE');
  });

  it('omits country from the query while the "Other…" input is incomplete or invalid', async () => {
    fetchMock.mockImplementation(async () => jsonResponse({ servers: [], near: '' }));
    wrap(<EngineOptionFields engine="ookla" options={{}} onChange={vi.fn()} />);

    fireEvent.change(screen.getByLabelText('Country'), { target: { value: 'other' } });
    fireEvent.change(screen.getByLabelText('Country code'), { target: { value: 'i' } });
    fireEvent.change(screen.getByLabelText('Search servers'), { target: { value: 'dublin' } });
    await waitFor(() => expect(fetchMock).toHaveBeenCalled(), { timeout: 1000 });
    expect(String(fetchMock.mock.calls[0][0])).not.toContain('country=');
  });

  it('keeps the manual ID input usable when the remote search errors', async () => {
    fetchMock.mockImplementation(async () => jsonResponse({ error: 'boom' }, 502));
    const onChange = vi.fn();
    wrap(<EngineOptionFields engine="ookla" options={{}} onChange={onChange} />);

    fireEvent.change(screen.getByLabelText('Search servers'), { target: { value: 'denver' } });
    await waitFor(() => expect(fetchMock).toHaveBeenCalled(), { timeout: 1000 });

    fireEvent.change(screen.getByLabelText('Ookla server ID'), { target: { value: '42' } });
    expect(onChange).toHaveBeenCalledWith({ server_id: 42 });
  });

  // Regression test for the real-browser/Playwright bug: a click into an
  // already-focused search field (or a second click) fires Radix's
  // interact-outside on PopoverContent, closing the popover — and since
  // focus never actually changes, typing afterwards used to never reopen
  // it (onChange only updated `search`, not `focused`). The fix makes
  // onChange also set `focused` back to true. Real outside-interaction
  // *detection* can't be exercised under jsdom (see the mock's top-of-file
  // note), so "closing" is simulated via the mock's force-close button,
  // which calls the same onOpenChange(false) a real dismiss would.
  it('reopens the results list once typing resumes after the popover was closed', async () => {
    fetchMock.mockImplementation(async () => jsonResponse({ servers: [denver], near: '' }));
    wrap(<EngineOptionFields engine="ookla" options={{}} onChange={vi.fn()} />);

    fireEvent.change(screen.getByLabelText('Search servers'), { target: { value: 'denver' } });
    expect(await screen.findByText('Comcast')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('mock-popover-force-close'));
    expect(screen.queryByText('Comcast')).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText('Search servers'), { target: { value: 'denver2' } });
    expect(await screen.findByText('Comcast')).toBeInTheDocument();
  });
});

describe('OoklaResultsList (tested directly, no Popover)', () => {
  it('renders sponsor, location, country and distance for each hit', () => {
    render(
      <OoklaResultsList servers={[denver, boulder]} isFetching={false} isError={false} onSelect={vi.fn()} />,
    );
    expect(screen.getByText('Comcast')).toBeInTheDocument();
    expect(screen.getByText('Xfinity')).toBeInTheDocument();
    expect(screen.getByText(/Denver, CO/)).toBeInTheDocument();
    expect(screen.getAllByText(/United States/)).toHaveLength(2);
    expect(screen.getByText(/· 1 km/)).toBeInTheDocument();
    expect(screen.getByText(/· 40 km/)).toBeInTheDocument();
  });

  it('calls onSelect with the chosen server', () => {
    const onSelect = vi.fn();
    render(
      <OoklaResultsList servers={[denver]} isFetching={false} isError={false} onSelect={onSelect} />,
    );
    fireEvent.click(screen.getByText('Comcast'));
    expect(onSelect).toHaveBeenCalledWith(denver);
  });

  it('shows a searching indicator while fetching', () => {
    render(<OoklaResultsList servers={[]} isFetching isError={false} onSelect={vi.fn()} />);
    expect(screen.getByText('Searching…')).toBeInTheDocument();
  });

  it('shows an empty state when there are no hits', () => {
    render(<OoklaResultsList servers={[]} isFetching={false} isError={false} onSelect={vi.fn()} />);
    const empty = screen.getByText('No servers found');
    expect(empty).toBeInTheDocument();
    expect(empty).toHaveClass('px-3', 'py-6', 'text-center', 'text-sm', 'text-muted');
  });

  it('shows an error state', () => {
    render(<OoklaResultsList servers={[]} isFetching={false} isError onSelect={vi.fn()} />);
    expect(screen.getByText(/Server list unavailable/)).toBeInTheDocument();
  });

  it('shows a "Near: <near>" header row when the query resolved through geocoding', () => {
    render(
      <OoklaResultsList servers={[denver]} isFetching={false} isError={false} onSelect={vi.fn()} near="Denver, Colorado" />,
    );
    expect(screen.getByText('Near: Denver, Colorado')).toBeInTheDocument();
  });

  it('omits the near header row when there is nothing to show', () => {
    render(<OoklaResultsList servers={[denver]} isFetching={false} isError={false} onSelect={vi.fn()} />);
    expect(screen.queryByText(/^Near:/)).not.toBeInTheDocument();
  });
});

// The Popover mock at the top of this file means these are safe from the
// jsdom hang; the visible list rendering is covered separately by
// "Iperf3ResultsList (tested directly, no Popover)".
describe('Iperf3Fields: picker wiring', () => {
  it('does not query until the picker is focused or searched', async () => {
    fetchMock.mockImplementation(async () => jsonResponse({ fetched_at: '', servers: [frankfurtIperf], total: 1 }));
    vi.useFakeTimers();
    try {
      wrap(<EngineOptionFields engine="iperf3" options={{}} onChange={vi.fn()} />);

      // Merely mounting the form (picker never opened) must not fire a
      // request — see EngineOptionFields.tsx's `enabled` gate. Advance past
      // the debounce window so a delayed fetch would have fired by now.
      await act(async () => { await vi.advanceTimersByTimeAsync(1000); });
      expect(fetchMock).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });

  it('fetches the public server list (debounced) once focused, without needing a query', async () => {
    fetchMock.mockImplementation(async () => jsonResponse({ fetched_at: '', servers: [frankfurtIperf], total: 1 }));
    wrap(<EngineOptionFields engine="iperf3" options={{}} onChange={vi.fn()} />);

    fireEvent.focus(screen.getByLabelText('Pick from public list'));
    await waitFor(() => expect(fetchMock).toHaveBeenCalled(), { timeout: 1000 });
    const [url] = fetchMock.mock.calls[0];
    expect(String(url)).toContain('/iperf3/servers');
  });

  it('re-fetches with the typed query once the debounce settles', async () => {
    fetchMock.mockImplementation(async () => jsonResponse({ fetched_at: '', servers: [], total: 0 }));
    wrap(<EngineOptionFields engine="iperf3" options={{}} onChange={vi.fn()} />);
    fetchMock.mockClear();

    fireEvent.change(screen.getByLabelText('Pick from public list'), { target: { value: 'denver' } });
    await waitFor(() => expect(String(fetchMock.mock.calls.at(-1)?.[0])).toContain('q=denver'), { timeout: 1000 });
  });

  // Same regression as OoklaFields' "reopens the results list..." test
  // above: closing (simulated via the mock's force-close button) must not
  // permanently disable reopening on further typing.
  it('reopens the results list once typing resumes after the popover was closed', async () => {
    fetchMock.mockImplementation(async () => jsonResponse({ fetched_at: '', servers: [frankfurtIperf], total: 1 }));
    wrap(<EngineOptionFields engine="iperf3" options={{}} onChange={vi.fn()} />);

    // The picker only opens once focused (see the jsdom-hang rationale
    // above) — focus it to see the already-fetched default list.
    fireEvent.focus(screen.getByLabelText('Pick from public list'));
    expect(await screen.findByText('iperf.example.net:5201–5210')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('mock-popover-force-close'));
    expect(screen.queryByText('iperf.example.net:5201–5210')).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText('Pick from public list'), { target: { value: 'frank' } });
    expect(await screen.findByText('iperf.example.net:5201–5210')).toBeInTheDocument();
  });

  it('keeps the host and port fields directly editable once Custom is on', () => {
    // A username makes Custom start on (see the "Custom toggle" describe below).
    const onChange = vi.fn();
    wrap(<EngineOptionFields engine="iperf3" options={{ host: 'manual.example.net', username: 'u' }} onChange={onChange} />);
    expect(screen.getByLabelText('Host')).toHaveValue('manual.example.net');
    fireEvent.change(screen.getByLabelText('Host'), { target: { value: 'changed.example.net' } });
    expect(onChange).toHaveBeenCalledWith({ host: 'changed.example.net', username: 'u' });
    fireEvent.change(screen.getByLabelText('Port'), { target: { value: '5202' } });
    expect(onChange).toHaveBeenCalledWith({ host: 'manual.example.net', username: 'u', port: 5202 });
  });

  it('picking a server that supports -R sets reverse and shows the read-only picked block with a hint', async () => {
    fetchMock.mockImplementation(async () => jsonResponse({ fetched_at: '', servers: [frankfurtIperf], total: 1 }));
    const onChange = vi.fn();
    function ControlledIperf3Fields() {
      const [options, setOptions] = useState<Record<string, unknown>>({});
      return (
        <EngineOptionFields
          engine="iperf3" options={options}
          onChange={(next) => { setOptions(next); onChange(next); }}
        />
      );
    }
    wrap(<ControlledIperf3Fields />);

    fireEvent.focus(screen.getByLabelText('Pick from public list'));
    fireEvent.click(await screen.findByText('iperf.example.net:5201–5210'));

    expect(onChange).toHaveBeenCalledWith({
      host: 'iperf.example.net', port: 5201, reverse: true, port_range_end: 5210,
    });
    const picked = screen.getByTestId('iperf3-picked');
    expect(picked).toHaveTextContent('iperf.example.net:5201');
    expect(picked).toHaveTextContent(/Reverse \(-R\) supported/);
    expect(picked).toHaveTextContent(/ports 5201–5210/);
    expect(screen.queryByLabelText('Host')).not.toBeInTheDocument();
  });

  it('picking a server without -R support does not set reverse or a port range', async () => {
    fetchMock.mockImplementation(async () => jsonResponse({ fetched_at: '', servers: [denverIperf], total: 1 }));
    const onChange = vi.fn();
    wrap(<EngineOptionFields engine="iperf3" options={{}} onChange={onChange} />);

    fireEvent.focus(screen.getByLabelText('Pick from public list'));
    fireEvent.click(await screen.findByText('speed.other.net:5202'));

    expect(onChange).toHaveBeenCalledWith({ host: 'speed.other.net', port: 5202 });
  });

  it('clears a previous pick\'s reverse and port_range_end when the next pick supports neither', async () => {
    fetchMock.mockImplementation(async () => jsonResponse({
      fetched_at: '', servers: [frankfurtIperf, denverIperf], total: 2,
    }));
    const onChange = vi.fn();
    function ControlledIperf3Fields() {
      const [options, setOptions] = useState<Record<string, unknown>>({});
      return (
        <EngineOptionFields
          engine="iperf3" options={options}
          onChange={(next) => { setOptions(next); onChange(next); }}
        />
      );
    }
    wrap(<ControlledIperf3Fields />);

    fireEvent.focus(screen.getByLabelText('Pick from public list'));
    fireEvent.click(await screen.findByText('iperf.example.net:5201–5210'));
    expect(onChange).toHaveBeenLastCalledWith({
      host: 'iperf.example.net', port: 5201, reverse: true, port_range_end: 5210,
    });

    fireEvent.focus(screen.getByLabelText('Pick from public list'));
    fireEvent.click(await screen.findByText('speed.other.net:5202'));
    expect(onChange).toHaveBeenLastCalledWith({ host: 'speed.other.net', port: 5202 });
  });
});

describe('Iperf3Fields: Custom toggle', () => {
  it('starts off on a fresh target: no Host input, no advanced fields', () => {
    wrap(<EngineOptionFields engine="iperf3" options={{}} onChange={vi.fn()} />);
    const toggle = screen.getByLabelText('Custom');
    expect(toggle).toHaveAttribute('aria-checked', 'false');
    expect(screen.queryByLabelText('Host')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Port')).not.toBeInTheDocument();
  });

  it('starts on when editing a target with a hand-set advanced option', () => {
    wrap(<EngineOptionFields engine="iperf3" options={{ parallel: 4 }} onChange={vi.fn()} />);
    expect(screen.getByLabelText('Custom')).toHaveAttribute('aria-checked', 'true');
    expect(screen.getByLabelText('Parallel streams')).toHaveValue('4');
  });

  it('starts off for a target configured purely from the public-list pick (host/port/reverse)', () => {
    wrap(<EngineOptionFields engine="iperf3" options={{ host: 'a.example.net', port: 5202, reverse: true }} onChange={vi.fn()} />);
    expect(screen.getByLabelText('Custom')).toHaveAttribute('aria-checked', 'false');
    expect(screen.queryByLabelText('Host')).not.toBeInTheDocument();
    expect(screen.getByTestId('iperf3-picked')).toBeInTheDocument();
  });

  it('does not treat explicit reverse:false/bidir:false as a Custom-triggering option', () => {
    wrap(<EngineOptionFields engine="iperf3" options={{ reverse: false, bidir: false }} onChange={vi.fn()} />);
    expect(screen.getByLabelText('Custom')).toHaveAttribute('aria-checked', 'false');
  });

  it('toggles Host and advanced fields on and off', () => {
    wrap(<EngineOptionFields engine="iperf3" options={{}} onChange={vi.fn()} />);
    const toggle = screen.getByLabelText('Custom');

    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute('aria-checked', 'true');
    expect(screen.getByLabelText('Host')).toBeInTheDocument();
    expect(screen.getByLabelText('Port')).toBeInTheDocument();

    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute('aria-checked', 'false');
    expect(screen.queryByLabelText('Host')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Port')).not.toBeInTheDocument();
  });
});

describe('Iperf3ResultsList (tested directly, no Popover)', () => {
  it('renders host:port-port_end, site/country (continent), provider', () => {
    render(
      <Iperf3ResultsList servers={[frankfurtIperf, denverIperf]} isFetching={false} isError={false} onSelect={vi.fn()} />,
    );
    expect(screen.getByText('iperf.example.net:5201–5210')).toBeInTheDocument();
    expect(screen.getByText('speed.other.net:5202')).toBeInTheDocument();
    expect(screen.getByText('Frankfurt, DE (Europe), Example Net')).toBeInTheDocument();
    expect(screen.getByText('Denver, US, Other Net')).toBeInTheDocument();
  });

  it('shows right-aligned badge chips for gbs, -R, UDP and IPv6', () => {
    render(
      <Iperf3ResultsList servers={[frankfurtIperf]} isFetching={false} isError={false} onSelect={vi.fn()} />,
    );
    expect(screen.getByText('10G')).toBeInTheDocument();
    expect(screen.getByText('-R')).toBeInTheDocument();
    expect(screen.getByText('UDP')).toBeInTheDocument();
    expect(screen.getByText('IPv6')).toBeInTheDocument();
  });

  it('renders a non-numeric gbs value verbatim, without an appended G', () => {
    render(
      <Iperf3ResultsList servers={[{ ...frankfurtIperf, gbs: 'n/a' }]} isFetching={false} isError={false} onSelect={vi.fn()} />,
    );
    expect(screen.getByText('n/a')).toBeInTheDocument();
    expect(screen.queryByText('n/aG')).not.toBeInTheDocument();
  });

  it('omits capability badges a server does not support', () => {
    render(
      <Iperf3ResultsList servers={[denverIperf]} isFetching={false} isError={false} onSelect={vi.fn()} />,
    );
    expect(screen.queryByText('-R')).not.toBeInTheDocument();
    expect(screen.queryByText('UDP')).not.toBeInTheDocument();
    expect(screen.queryByText('IPv6')).not.toBeInTheDocument();
  });

  it('calls onSelect with the chosen server, prefilling host and port', () => {
    const onSelect = vi.fn();
    render(
      <Iperf3ResultsList servers={[frankfurtIperf]} isFetching={false} isError={false} onSelect={onSelect} />,
    );
    fireEvent.click(screen.getByText('iperf.example.net:5201–5210'));
    expect(onSelect).toHaveBeenCalledWith(frankfurtIperf);
  });

  it('shows a searching indicator while fetching', () => {
    render(<Iperf3ResultsList servers={[]} isFetching isError={false} onSelect={vi.fn()} />);
    expect(screen.getByText('Searching…')).toBeInTheDocument();
  });

  it('shows an empty state when there are no hits', () => {
    render(<Iperf3ResultsList servers={[]} isFetching={false} isError={false} onSelect={vi.fn()} />);
    const empty = screen.getByText('No servers found');
    expect(empty).toBeInTheDocument();
    expect(empty).toHaveClass('px-3', 'py-6', 'text-center', 'text-sm', 'text-muted');
  });

  it('shows an error state', () => {
    render(<Iperf3ResultsList servers={[]} isFetching={false} isError onSelect={vi.fn()} />);
    expect(screen.getByText(/Server list unavailable/)).toBeInTheDocument();
  });
});
