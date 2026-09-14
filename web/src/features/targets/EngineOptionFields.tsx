import { useEffect, useRef, useState } from 'react';
import { ChevronRight } from 'lucide-react';
import { useIperf3Servers, useOoklaServers } from '../../lib/queries';
import type { Iperf3Server, OoklaServer } from '../../lib/api';
import { regionFromLocale } from '../../lib/locale';
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover';
import { Command, CommandEmpty, CommandGroup, CommandItem, CommandList } from '@/components/ui/command';
import { Checkbox } from '@/components/ui/checkbox';
import { Badge } from '@/components/ui/badge';

export type Options = Record<string, unknown>;

interface Props {
  engine: string;
  options: Options;
  onChange: (next: Options) => void;
  /** Bumped by the parent form on a failed submit blocked by
   * validateEngineOptions, to force the iperf3 advanced disclosure open so
   * the blocking error (rendered next to the field it's about) isn't
   * hidden behind a collapsed section. Ignored by engines other than
   * iperf3. */
  forceOpenAdvancedSignal?: number;
}

const field = 'w-full rounded border border-line bg-surface px-2 py-1 text-sm text-fg focus:border-accent focus:outline-none';
const label = 'block text-xs font-medium uppercase tracking-wide text-muted';

/** setOption writes a key, deleting it when the value is empty. */
function setOption(options: Options, key: string, value: unknown): Options {
  const next = { ...options };
  if (value === '' || value === undefined || value === null || value === false) {
    delete next[key];
  } else {
    next[key] = value;
  }
  return next;
}

function numberOr(value: string): number | '' {
  if (value.trim() === '') return '';
  const n = Number(value);
  return Number.isFinite(n) ? n : '';
}

/** validateEngineOptions returns a blocking error message for the current options, if any. */
export function validateEngineOptions(engine: string, options: Options): string | undefined {
  if (engine !== 'iperf3') return undefined;
  const password = options.password;
  if (typeof password === 'string' && password !== '') {
    if (!options.username || !options.rsa_public_key_path) {
      return 'Password requires a username and RSA public key path';
    }
  }
  return undefined;
}

const DEBOUNCE_MS = 300;

/** iperf3 option keys tucked behind the "Advanced options" disclosure —
 * everything but host and the public-list picker. */
const IPERF3_ADVANCED_KEYS = [
  'port', 'port_range_end', 'protocol', 'parallel', 'duration_s', 'udp_bitrate', 'bind',
  'username', 'rsa_public_key_path', 'password', 'reverse', 'bidir',
] as const;

/** COMMON_COUNTRIES is a short list of ISO 3166-1 alpha-2 codes covering
 * the countries most speedtest targets are likely to be in, shown as the
 * Ookla search's country hint <select>. "Other…" reveals a free 2-letter
 * input for anything not listed. */
const COMMON_COUNTRIES: readonly [string, string][] = [
  ['US', 'United States'], ['GB', 'United Kingdom'], ['CA', 'Canada'], ['AU', 'Australia'],
  ['DE', 'Germany'], ['FR', 'France'], ['ES', 'Spain'], ['IT', 'Italy'], ['NL', 'Netherlands'],
  ['SE', 'Sweden'], ['NO', 'Norway'], ['DK', 'Denmark'], ['FI', 'Finland'], ['PL', 'Poland'],
  ['PT', 'Portugal'], ['IE', 'Ireland'], ['CH', 'Switzerland'], ['AT', 'Austria'], ['BE', 'Belgium'],
  ['CZ', 'Czechia'], ['GR', 'Greece'], ['JP', 'Japan'], ['KR', 'South Korea'], ['CN', 'China'],
  ['IN', 'India'], ['BR', 'Brazil'], ['MX', 'Mexico'], ['ZA', 'South Africa'], ['NZ', 'New Zealand'],
  ['SG', 'Singapore'],
];

function hasAdvancedIperf3Options(options: Options): boolean {
  return IPERF3_ADVANCED_KEYS.some((k) => {
    const v = options[k];
    if (k === 'reverse' || k === 'bidir') return v === true;
    return v !== undefined && v !== '';
  });
}

/** summarizeIperf3Advanced renders the closed-disclosure summary line,
 * e.g. "port 5201 · TCP · 10 s · -R". */
function summarizeIperf3Advanced(options: Options): string {
  const port = options.port !== undefined ? options.port : 5201;
  const protocol = (options.protocol === 'udp' ? 'udp' : 'tcp').toUpperCase();
  const duration = options.duration_s !== undefined ? options.duration_s : 10;
  const parts: string[] = [`port ${port}`, protocol, `${duration} s`];
  if (options.parallel !== undefined) parts.push(`x${options.parallel}`);
  if (protocol === 'UDP' && options.udp_bitrate) parts.push(String(options.udp_bitrate));
  if (options.bind) parts.push(`bind ${options.bind}`);
  if (options.username) parts.push('auth');
  if (options.reverse === true) parts.push('-R');
  if (options.bidir === true) parts.push('--bidir');
  return parts.join(' · ');
}

/** EngineOptionFields renders the option form for one engine. */
export function EngineOptionFields({ engine, options, onChange, forceOpenAdvancedSignal }: Props) {
  if (engine === 'ookla') return <OoklaFields options={options} onChange={onChange} />;
  if (engine === 'cloudflare') return <CloudflareFields options={options} onChange={onChange} />;
  if (engine === 'iperf3') {
    return (
      <Iperf3Fields options={options} onChange={onChange} forceOpenAdvancedSignal={forceOpenAdvancedSignal} />
    );
  }
  return (
    <p className="text-sm text-muted">
      The <span className="font-mono">{engine}</span> engine takes no configuration.
    </p>
  );
}

/** OoklaResultsList renders the server search hits as a Command list; kept
 * separate from OoklaFields so it can be tested (or reused) without going
 * through the Popover that wraps it. */
export function OoklaResultsList({ servers, isFetching, isError, onSelect, near }: {
  servers: OoklaServer[];
  isFetching: boolean;
  isError: boolean;
  onSelect: (s: OoklaServer) => void;
  /** near is the resolved place's display name when the query geocoded
   * through a postcode/name lookup, shown as a small header row above the
   * results so the user can confirm the search landed in the right place. */
  near?: string;
}) {
  return (
    <Command shouldFilter={false}>
      {!isFetching && !isError && near && (
        <p className="border-b border-line px-3 py-1.5 text-xs text-faint">Near: {near}</p>
      )}
      <CommandList>
        {isFetching && (
          <div className="px-2 py-3 text-xs text-faint">Searching…</div>
        )}
        {isError && (
          <div className="px-2 py-3 text-xs text-warn">Server list unavailable — enter an ID manually.</div>
        )}
        {!isFetching && !isError && servers.length === 0 && (
          <CommandEmpty>No servers found</CommandEmpty>
        )}
        {!isFetching && !isError && servers.length > 0 && (
          <CommandGroup heading="Servers">
            {servers.slice(0, 50).map((s) => (
              <CommandItem key={s.id} value={s.id} onSelect={() => onSelect(s)}>
                <div className="flex w-full items-baseline justify-between gap-2">
                  <span className="truncate text-fg">{s.name}</span>
                  <span className="shrink-0 text-xs text-faint">
                    {s.sponsor && s.sponsor !== s.name ? `${s.sponsor} · ` : ''}
                    {s.location ? `${s.location}, ` : ''}
                    {s.country}
                    {s.distance_km !== undefined ? ` · ${Math.round(s.distance_km)} km` : ''}
                  </span>
                </div>
              </CommandItem>
            ))}
          </CommandGroup>
        )}
      </CommandList>
    </Command>
  );
}

/** initialCountry resolves the country hint <select>'s starting value from
 * the browser locale: a code already in COMMON_COUNTRIES selects it
 * directly, any other 2-letter region falls into "Other…" pre-filled,
 * and no resolvable region leaves the hint empty (any country). */
function initialCountry(): { select: string; other: string } {
  const region = typeof navigator !== 'undefined' ? regionFromLocale(navigator.language) : undefined;
  if (!region) return { select: '', other: '' };
  if (COMMON_COUNTRIES.some(([code]) => code === region)) return { select: region, other: '' };
  return { select: 'other', other: region };
}

function OoklaFields({ options, onChange }: Omit<Props, 'engine'>) {
  const [search, setSearch] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  // The popover only opens while the field is focused, never merely because
  // it has enough characters — typing (fireEvent.change) alone must not
  // open it, so tests that don't focus the field never mount PopoverContent
  // (Radix's Popper positioning hangs jsdom for ~30s once mounted).
  const [focused, setFocused] = useState(false);
  // Anchors the input so PopoverContent's onInteractOutside can tell "the
  // user is still interacting with the search field" (already-focused
  // click, a second click, Playwright's fill()) apart from a real
  // outside interaction — see the onInteractOutside comment below.
  const anchorRef = useRef<HTMLInputElement>(null);

  const [countrySelect, setCountrySelect] = useState(() => initialCountry().select);
  const [countryOther, setCountryOther] = useState(() => initialCountry().other);
  const country = countrySelect === 'other' ? countryOther.trim().toUpperCase() : countrySelect;

  useEffect(() => {
    const t = setTimeout(() => setDebouncedSearch(search), DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [search]);

  const enabled = debouncedSearch.trim().length >= 2;
  const servers = useOoklaServers(debouncedSearch, country || undefined, enabled);
  const serverId = options.server_id === undefined ? '' : String(options.server_id);
  const open = focused && enabled;

  const handleSelect = (s: OoklaServer) => {
    onChange(setOption(options, 'server_id', Number(s.id)));
    setSearch(s.sponsor ? `${s.sponsor} — ${s.location}` : s.name);
    setFocused(false);
  };

  return (
    <div className="grid gap-3">
      <div>
        <label className={label} htmlFor="ookla-server-id">Ookla server ID</label>
        <input
          id="ookla-server-id"
          className={field}
          value={serverId}
          placeholder="auto (nearest server)"
          onChange={(e) => onChange(setOption(options, 'server_id', numberOr(e.target.value)))}
        />
      </div>
      <div className="flex gap-2">
        <div className="flex-1">
          <label className={label} htmlFor="ookla-server-search">Search servers</label>
          <Popover open={open} onOpenChange={(o) => { if (!o) setFocused(false); }}>
            <PopoverAnchor asChild>
              <input
                ref={anchorRef}
                id="ookla-server-search"
                className={field}
                value={search}
                autoComplete="off"
                placeholder="city, postcode, sponsor or host"
                onFocus={() => setFocused(true)}
                onChange={(e) => { setSearch(e.target.value); setFocused(true); }}
              />
            </PopoverAnchor>
            <PopoverContent
              align="start"
              onOpenAutoFocus={(e) => e.preventDefault()}
              // Radix treats any pointer-down outside PopoverContent as a
              // dismiss, including a second click/fill() on the already-
              // focused anchor input (jsdom aside, this reproduces in real
              // browsers and Playwright): focus never changes, so the popover
              // never reopens on its own. Interacting with the anchor itself
              // must never count as "outside".
              onInteractOutside={(e) => {
                if (anchorRef.current?.contains(e.target as Node)) e.preventDefault();
              }}
              className="w-(--radix-popover-trigger-width) p-0"
            >
              <OoklaResultsList
                servers={servers.data?.servers ?? []}
                isFetching={servers.isFetching}
                isError={servers.isError}
                onSelect={handleSelect}
                near={servers.data?.near}
              />
            </PopoverContent>
          </Popover>
        </div>
        <div className="w-28 shrink-0">
          <label className={label} htmlFor="ookla-country">Country</label>
          <select
            id="ookla-country"
            className={field}
            value={countrySelect}
            onChange={(e) => setCountrySelect(e.target.value)}
          >
            <option value="">Any</option>
            {COMMON_COUNTRIES.map(([code, name]) => <option key={code} value={code}>{name}</option>)}
            <option value="other">Other…</option>
          </select>
          {countrySelect === 'other' && (
            <input
              id="ookla-country-other"
              aria-label="Country code"
              className={`${field} mt-1`}
              value={countryOther}
              maxLength={2}
              placeholder="e.g. IE"
              onChange={(e) => setCountryOther(e.target.value)}
            />
          )}
        </div>
      </div>
    </div>
  );
}

/** Iperf3ResultsList renders the public iperf3 server list search hits as
 * a Command list; kept separate from Iperf3Fields for the same reason as
 * OoklaResultsList (testable without the Popover). */
export function Iperf3ResultsList({ servers, isFetching, isError, onSelect }: {
  servers: Iperf3Server[];
  isFetching: boolean;
  isError: boolean;
  onSelect: (s: Iperf3Server) => void;
}) {
  return (
    <Command shouldFilter={false}>
      <CommandList>
        {isFetching && (
          <div className="px-2 py-3 text-xs text-faint">Searching…</div>
        )}
        {isError && (
          <div className="px-2 py-3 text-xs text-warn">Server list unavailable — enter a host manually.</div>
        )}
        {!isFetching && !isError && servers.length === 0 && (
          <CommandEmpty>No servers found</CommandEmpty>
        )}
        {!isFetching && !isError && servers.length > 0 && (
          <CommandGroup heading="Public iperf3 servers">
            {servers.slice(0, 50).map((s) => (
              <CommandItem key={`${s.host}:${s.port}`} value={`${s.host}:${s.port}`} onSelect={() => onSelect(s)}>
                <div className="flex w-full items-baseline justify-between gap-2">
                  <span className="truncate text-fg">{s.host}:{s.port}</span>
                  <span className="shrink-0 text-xs text-faint">
                    {[s.site, s.country].filter(Boolean).join(', ')}
                    {s.provider ? ` · ${s.provider}` : ''}
                    {s.supports_reverse ? ' · supports -R' : ''}
                    {s.supports_udp ? ' · supports UDP' : ''}
                  </span>
                </div>
              </CommandItem>
            ))}
          </CommandGroup>
        )}
      </CommandList>
    </Command>
  );
}

function CloudflareFields({ options, onChange }: Omit<Props, 'engine'>) {
  const sizes = (key: string) =>
    Array.isArray(options[key]) ? (options[key] as number[]).join(',') : '';
  const parseSizes = (raw: string) =>
    raw.split(',').map((s) => Number(s.trim())).filter((n) => Number.isFinite(n) && n > 0);

  return (
    <div className="grid gap-3">
      <div>
        <label className={label} htmlFor="cf-download-sizes">Download sizes (bytes, comma separated)</label>
        <input
          id="cf-download-sizes"
          className={field}
          value={sizes('download_sizes')}
          placeholder="1000000,10000000,100000000"
          onChange={(e) => {
            const parsed = parseSizes(e.target.value);
            onChange(setOption(options, 'download_sizes', parsed.length ? parsed : ''));
          }}
        />
      </div>
      <div>
        <label className={label} htmlFor="cf-upload-sizes">Upload sizes (bytes, comma separated)</label>
        <input
          id="cf-upload-sizes"
          className={field}
          value={sizes('upload_sizes')}
          placeholder="1000000,10000000"
          onChange={(e) => {
            const parsed = parseSizes(e.target.value);
            onChange(setOption(options, 'upload_sizes', parsed.length ? parsed : ''));
          }}
        />
      </div>
      <div>
        <label className={label} htmlFor="cf-latency-samples">Latency samples</label>
        <input
          id="cf-latency-samples"
          className={field}
          value={options.latency_samples === undefined ? '' : String(options.latency_samples)}
          onChange={(e) => onChange(setOption(options, 'latency_samples', numberOr(e.target.value)))}
        />
      </div>
    </div>
  );
}

function Iperf3Fields({ options, onChange, forceOpenAdvancedSignal }: Omit<Props, 'engine'>) {
  const text = (key: string) => (options[key] === undefined ? '' : String(options[key]));
  const checked = (key: string) => options[key] === true;

  const protocol = text('protocol') || 'tcp';
  const isUdp = protocol === 'udp';
  const reverseOn = checked('reverse');
  const bidirOn = checked('bidir');
  const passwordSet = typeof options.password === 'string' && options.password !== '';
  const passwordNeedsAuth = passwordSet && (!options.username || !options.rsa_public_key_path);

  const [search, setSearch] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  // Same jsdom-hang rationale as OoklaFields: the popover only opens while
  // the field is focused, never merely from typing.
  const [focused, setFocused] = useState(false);
  const [pickedHints, setPickedHints] = useState<{ reverse: boolean; udp: boolean } | null>(null);
  // Same "interacting with the anchor isn't an outside interaction" need
  // as OoklaFields — see its onInteractOutside comment.
  const anchorRef = useRef<HTMLInputElement>(null);

  // Advanced options (everything but host/picker) start collapsed on a
  // fresh target, but open by default when editing one that already has
  // any of them set, so nothing configured is hidden from view.
  const [advancedOpen, setAdvancedOpen] = useState(() => hasAdvancedIperf3Options(options));

  // A failed submit blocked by validateEngineOptions (e.g. a password with
  // no username/RSA key) bumps this signal from the parent form; force the
  // section open so the error isn't hidden behind a collapsed disclosure.
  useEffect(() => {
    if (forceOpenAdvancedSignal) setAdvancedOpen(true);
  }, [forceOpenAdvancedSignal]);

  useEffect(() => {
    const t = setTimeout(() => setDebouncedSearch(search), DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [search]);

  // Unlike the Ookla text search (which needs 2+ typed characters before
  // it's worth hitting a remote API), this list is served from our own
  // cache and meant to be browsed: it fetches (debounced) as soon as the
  // field is focused or has any text, even with an empty query, so
  // opening the picker shows the default list right away. It stays gated
  // on focus/search rather than always-on so the query doesn't fire (and
  // refetch on every list refresh) for a target form the picker was never
  // opened on.
  const enabled = focused || search.length > 0;
  const publicServers = useIperf3Servers(debouncedSearch, enabled);
  const open = focused;

  const handlePick = (s: Iperf3Server) => {
    let next = setOption(options, 'host', s.host);
    next = setOption(next, 'port', s.port);
    onChange(next);
    setPickedHints({ reverse: s.supports_reverse, udp: s.supports_udp });
    setSearch(`${s.host}:${s.port}`);
    setFocused(false);
    // Surface the advanced section so the -R hint below the host field
    // isn't the only clue that this server needs it ticked.
    if (s.supports_reverse) setAdvancedOpen(true);
  };

  const hints = pickedHints
    ? [pickedHints.reverse && 'supports -R', pickedHints.udp && 'supports UDP'].filter(Boolean)
    : [];

  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <div className="sm:col-span-2">
        <label className={label} htmlFor="iperf-host">Host</label>
        <input id="iperf-host" className={field} value={text('host')}
          onChange={(e) => onChange(setOption(options, 'host', e.target.value))} />
        {hints.length > 0 && (
          <p className="mt-1 text-xs text-faint">{hints.join(' · ')}</p>
        )}
      </div>
      <div className="sm:col-span-2">
        <label className={label} htmlFor="iperf-public-search">Pick from public list</label>
        <Popover open={open} onOpenChange={(o) => { if (!o) setFocused(false); }}>
          <PopoverAnchor asChild>
            <input
              ref={anchorRef}
              id="iperf-public-search"
              className={field}
              value={search}
              autoComplete="off"
              placeholder="host, site, country or provider"
              onFocus={() => setFocused(true)}
              onChange={(e) => { setSearch(e.target.value); setFocused(true); }}
            />
          </PopoverAnchor>
          <PopoverContent
            align="start"
            onOpenAutoFocus={(e) => e.preventDefault()}
            onInteractOutside={(e) => {
              if (anchorRef.current?.contains(e.target as Node)) e.preventDefault();
            }}
            className="w-(--radix-popover-trigger-width) p-0"
          >
            <Iperf3ResultsList
              servers={publicServers.data?.servers ?? []}
              isFetching={publicServers.isFetching}
              isError={publicServers.isError}
              onSelect={handlePick}
            />
          </PopoverContent>
        </Popover>
      </div>

      <div className="sm:col-span-2">
        <button
          type="button"
          aria-expanded={advancedOpen}
          aria-controls={advancedOpen ? 'iperf-advanced-options' : undefined}
          onClick={() => setAdvancedOpen((o) => !o)}
          className="flex items-center gap-1 text-sm font-medium text-muted hover:text-fg"
        >
          <ChevronRight className={`h-4 w-4 transition-transform ${advancedOpen ? 'rotate-90' : ''}`} />
          Advanced options
        </button>
        {!advancedOpen && (
          <p className="mt-1 text-xs text-faint">{summarizeIperf3Advanced(options)}</p>
        )}
      </div>

      {advancedOpen && (
      <div id="iperf-advanced-options" className="grid gap-3 sm:col-span-2 sm:grid-cols-2">
      <div>
        <label className={label} htmlFor="iperf-port">Port</label>
        <input id="iperf-port" className={field} value={text('port')} placeholder="5201"
          onChange={(e) => onChange(setOption(options, 'port', numberOr(e.target.value)))} />
      </div>
      <div>
        <label className={label} htmlFor="iperf-protocol">Protocol</label>
        <select id="iperf-protocol" className={field} value={protocol}
          onChange={(e) => {
            const nextProtocol = e.target.value;
            let next = setOption(options, 'protocol', nextProtocol === 'tcp' ? '' : nextProtocol);
            if (nextProtocol === 'udp' && bidirOn) next = setOption(next, 'bidir', false);
            if (nextProtocol === 'tcp' && options.udp_bitrate !== undefined) next = setOption(next, 'udp_bitrate', '');
            onChange(next);
          }}>
          <option value="tcp">TCP</option>
          <option value="udp">UDP</option>
        </select>
      </div>
      <div>
        <label className={label} htmlFor="iperf-parallel">Parallel streams</label>
        <input id="iperf-parallel" className={field} value={text('parallel')} placeholder="1"
          onChange={(e) => onChange(setOption(options, 'parallel', numberOr(e.target.value)))} />
      </div>
      <div>
        <label className={label} htmlFor="iperf-duration">Duration (s)</label>
        <input id="iperf-duration" className={field} value={text('duration_s')} placeholder="10"
          onChange={(e) => onChange(setOption(options, 'duration_s', numberOr(e.target.value)))} />
      </div>
      <div>
        <label className={label} htmlFor="iperf-bitrate">UDP bitrate</label>
        <input id="iperf-bitrate" className={field} value={text('udp_bitrate')} placeholder="100M"
          disabled={!isUdp} title={isUdp ? undefined : 'Only applies to UDP'}
          onChange={(e) => onChange(setOption(options, 'udp_bitrate', e.target.value))} />
      </div>
      <div>
        <label className={label} htmlFor="iperf-bind">Bind address</label>
        <input id="iperf-bind" className={field} value={text('bind')}
          onChange={(e) => onChange(setOption(options, 'bind', e.target.value))} />
      </div>
      <div>
        <label className={label} htmlFor="iperf-username">Username</label>
        <input id="iperf-username" className={field} value={text('username')}
          onChange={(e) => onChange(setOption(options, 'username', e.target.value))} />
      </div>
      <div>
        <label className={label} htmlFor="iperf-rsa">RSA public key path</label>
        <input id="iperf-rsa" className={field} value={text('rsa_public_key_path')}
          onChange={(e) => onChange(setOption(options, 'rsa_public_key_path', e.target.value))} />
      </div>
      <div>
        <label className={label} htmlFor="iperf-password">Password</label>
        <input id="iperf-password" type="password" className={field} value={text('password')}
          onChange={(e) => onChange(setOption(options, 'password', e.target.value))} />
        {passwordNeedsAuth && (
          <p className="mt-1 text-xs text-warn">
            Password requires a username and RSA public key path
          </p>
        )}
      </div>
      {/* Disable a box only when the other is checked and this one isn't —
          never both at once. Legacy options can have reverse and bidir both
          true (an invalid combination the engine would reject); if that
          happened, disabling both here would make it impossible to ever
          uncheck either one. */}
      <label className="flex items-center gap-2 text-sm text-muted" htmlFor="iperf-reverse">
        <Checkbox id="iperf-reverse" checked={reverseOn} disabled={bidirOn && !reverseOn}
          onCheckedChange={(checked) => onChange(setOption(options, 'reverse', checked === true))} />
        Reverse (-R)
      </label>
      <label className="flex items-center gap-2 text-sm text-muted" htmlFor="iperf-bidir">
        <Checkbox id="iperf-bidir" checked={bidirOn} disabled={(reverseOn && !bidirOn) || isUdp}
          onCheckedChange={(checked) => onChange(setOption(options, 'bidir', checked === true))} />
        Bidirectional (--bidir)
      </label>
      </div>
      )}
    </div>
  );
}
