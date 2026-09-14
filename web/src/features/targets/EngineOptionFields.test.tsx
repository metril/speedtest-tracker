import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  act, fireEvent, render, screen, waitFor,
} from '@testing-library/react';
import type { ReactNode } from 'react';
import {
  afterEach, beforeEach, describe, expect, it, vi,
} from 'vitest';
import type { OoklaServer } from '../../lib/api';
import { EngineOptionFields, OoklaResultsList } from './EngineOptionFields';

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
    expect(screen.getByLabelText('Download sizes (bytes, comma separated)')).toBeInTheDocument();
  });

  it('still renders the iperf3 host field', () => {
    wrap(<EngineOptionFields engine="iperf3" options={{}} onChange={vi.fn()} />);
    expect(screen.getByLabelText('Host')).toBeInTheDocument();
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

// Note: these tests deliberately never let the search Popover actually
// open. Rendering Radix's PopoverContent (Popper positioning) in jsdom
// hangs the test runner for ~30s per the task brief's jsdom note, whether
// triggered by a click or, as found here, simply by controlled `open`
// becoming true. The picker's visible rendering (sponsor/location/country/
// distance, empty/error/loading states, selection) is covered instead by
// the "OoklaResultsList (tested directly, no Popover)" suite below; here we
// only check that OoklaFields wires the debounced query and manual ID
// input correctly, which doesn't require the popover to mount.
describe('OoklaFields: search wiring (popover stays closed)', () => {
  it('fetches with the typed query once the debounce settles', async () => {
    fetchMock.mockImplementation(async () => jsonResponse([denver, boulder]));
    wrap(<EngineOptionFields engine="ookla" options={{}} onChange={vi.fn()} />);

    fireEvent.change(screen.getByLabelText('Search servers'), { target: { value: 'denver' } });

    await waitFor(() => expect(fetchMock).toHaveBeenCalled(), { timeout: 1000 });
    const [url] = fetchMock.mock.calls[0];
    expect(String(url)).toContain('q=denver');
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
    expect(screen.getByText('No servers found')).toBeInTheDocument();
  });

  it('shows an error state', () => {
    render(<OoklaResultsList servers={[]} isFetching={false} isError onSelect={vi.fn()} />);
    expect(screen.getByText(/Server list unavailable/)).toBeInTheDocument();
  });
});
