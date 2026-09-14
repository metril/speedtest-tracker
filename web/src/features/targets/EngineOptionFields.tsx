import { useEffect, useRef, useState } from 'react';
import { useIperf3Servers, useOoklaServers } from '../../lib/queries';
import type { Iperf3Server, OoklaServer } from '../../lib/api';
import { regionFromLocale } from '../../lib/locale';
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover';
import { Command, CommandEmpty, CommandGroup, CommandItem, CommandList } from '@/components/ui/command';
import { Checkbox } from '@/components/ui/checkbox';
import { Badge } from '@/components/ui/badge';
import { Label } from '@/components/ui/label';
import { cn } from '@/lib/utils';
import { inputClass } from '@/features/settings/styles';
import { SwitchField } from '../../components/SwitchField';
import { FormField } from '../../components/FormField';

export type Options = Record<string, unknown>;

interface Props {
  engine: string;
  options: Options;
  onChange: (next: Options) => void;
  /** Bumped by the parent form on a failed submit blocked by
   * validateEngineOptions, to force iperf3's Custom section on so the
   * blocking error (rendered next to the field it's about) isn't hidden
   * behind a field Custom-off would have hidden. Ignored by engines other
   * than iperf3. */
  forceOpenAdvancedSignal?: number;
}

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

/** iperf3 option keys shown only when Custom is on — everything but host
 * and the public-list picker. */
const IPERF3_ADVANCED_KEYS = [
  'port', 'port_range_end', 'protocol', 'parallel', 'duration_s', 'udp_bitrate', 'bind',
  'username', 'rsa_public_key_path', 'password', 'reverse', 'bidir',
] as const;

/** Keys the public-list picker writes itself (handlePick). A target that
 * only has these set was configured entirely by picking from the list, so
 * Custom should start off for it; anything else set means a person
 * hand-configured it, so Custom should start on. */
const PICK_WRITTEN_KEYS = ['port', 'port_range_end', 'reverse'] as const;

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

/** hasCustomIperf3Options decides Custom's initial state: on iff any
 * advanced key other than the ones the public-list picker writes itself
 * is set, so a target created purely by picking (host/port/reverse) opens
 * with Custom off and a hand-configured one opens with it on. */
function hasCustomIperf3Options(options: Options): boolean {
  return IPERF3_ADVANCED_KEYS.some((k) => {
    if ((PICK_WRITTEN_KEYS as readonly string[]).includes(k)) return false;
    const v = options[k];
    if (k === 'bidir') return v === true;
    return v !== undefined && v !== '';
  });
}

/** summarizePick renders the hints shown next to the read-only picked-
 * server block when Custom is off, e.g. "Reverse (-R) supported · UDP". */
function summarizePick(options: Options): string[] {
  const hints: string[] = [];
  if (options.reverse === true) hints.push('Reverse (-R) supported');
  if (options.protocol === 'udp') hints.push('UDP');
  const port = options.port;
  const portEnd = options.port_range_end;
  if (typeof port === 'number' && typeof portEnd === 'number' && portEnd > port) {
    hints.push(`ports ${port}–${portEnd}`);
  }
  return hints;
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

  const [{ select: initialSelect, other: initialOther }] = useState(initialCountry);
  const [countrySelect, setCountrySelect] = useState(initialSelect);
  const [countryOther, setCountryOther] = useState(initialOther);
  const rawCountry = countrySelect === 'other' ? countryOther.trim().toUpperCase() : countrySelect;
  // The free-text "Other…" input accepts anything as the user types; only
  // a complete 2-letter code is sent to the search (the server also
  // validates this, but sending a partial/invalid code would just 400).
  const country = /^[A-Za-z]{2}$/.test(rawCountry) ? rawCountry : undefined;

  useEffect(() => {
    const t = setTimeout(() => setDebouncedSearch(search), DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [search]);

  const enabled = debouncedSearch.trim().length >= 2;
  const servers = useOoklaServers(debouncedSearch, country, enabled);
  const serverId = options.server_id === undefined ? '' : String(options.server_id);
  const open = focused && enabled;

  const handleSelect = (s: OoklaServer) => {
    onChange(setOption(options, 'server_id', Number(s.id)));
    setSearch(s.sponsor ? `${s.sponsor} — ${s.location}` : s.name);
    setFocused(false);
  };

  return (
    <div className="grid gap-3">
      <FormField id="ookla-server-id" label="Ookla server ID">
        <input
          id="ookla-server-id"
          className={inputClass}
          value={serverId}
          placeholder="auto (nearest server)"
          onChange={(e) => onChange(setOption(options, 'server_id', numberOr(e.target.value)))}
        />
      </FormField>
      <div className="flex gap-2">
        <div className="flex-1">
          <FormField id="ookla-server-search" label="Search servers">
          <Popover open={open} onOpenChange={(o) => { if (!o) setFocused(false); }}>
            <PopoverAnchor asChild>
              <input
                ref={anchorRef}
                id="ookla-server-search"
                className={inputClass}
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
          </FormField>
        </div>
        <div className="w-28 shrink-0">
          <FormField id="ookla-country" label="Country">
            <select
              id="ookla-country"
              className={inputClass}
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
                className={cn(inputClass, "mt-1")}
                value={countryOther}
                maxLength={2}
                placeholder="e.g. IE"
                onChange={(e) => setCountryOther(e.target.value)}
              />
            )}
          </FormField>
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
            {servers.slice(0, 50).map((s) => {
              const portLabel = s.port_end && s.port_end > s.port ? `${s.port}–${s.port_end}` : `${s.port}`;
              const location = [
                s.site,
                s.country && s.continent ? `${s.country} (${s.continent})` : (s.country || s.continent),
                s.provider,
              ].filter(Boolean).join(', ');
              const gbsNumeric = s.gbs !== undefined && /^\d+(\.\d+)?$/.test(s.gbs);
              return (
                <CommandItem key={`${s.host}:${s.port}`} value={`${s.host}:${s.port}`} onSelect={() => onSelect(s)}>
                  <div className="flex w-full items-center justify-between gap-2">
                    <div className="min-w-0">
                      <span className="truncate text-fg">{s.host}:{portLabel}</span>
                      {location && <span className="block truncate text-xs text-faint">{location}</span>}
                    </div>
                    <div className="flex shrink-0 items-center gap-1">
                      {s.gbs && <Badge variant="outline">{gbsNumeric ? `${s.gbs}G` : s.gbs}</Badge>}
                      {s.supports_reverse && <Badge variant="outline">-R</Badge>}
                      {s.supports_udp && <Badge variant="outline">UDP</Badge>}
                      {s.supports_ipv6 && <Badge variant="outline">IPv6</Badge>}
                    </div>
                  </div>
                </CommandItem>
              );
            })}
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
      <FormField id="cf-download-sizes" label="Download sizes (bytes, comma separated)">
        <input
          id="cf-download-sizes"
          className={inputClass}
          value={sizes('download_sizes')}
          placeholder="1000000,10000000,100000000"
          onChange={(e) => {
            const parsed = parseSizes(e.target.value);
            onChange(setOption(options, 'download_sizes', parsed.length ? parsed : ''));
          }}
        />
      </FormField>
      <FormField id="cf-upload-sizes" label="Upload sizes (bytes, comma separated)">
        <input
          id="cf-upload-sizes"
          className={inputClass}
          value={sizes('upload_sizes')}
          placeholder="1000000,10000000"
          onChange={(e) => {
            const parsed = parseSizes(e.target.value);
            onChange(setOption(options, 'upload_sizes', parsed.length ? parsed : ''));
          }}
        />
      </FormField>
      <FormField id="cf-latency-samples" label="Latency samples">
        <input
          id="cf-latency-samples"
          className={inputClass}
          value={options.latency_samples === undefined ? '' : String(options.latency_samples)}
          onChange={(e) => onChange(setOption(options, 'latency_samples', numberOr(e.target.value)))}
        />
      </FormField>
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
  // Same "interacting with the anchor isn't an outside interaction" need
  // as OoklaFields — see its onInteractOutside comment.
  const anchorRef = useRef<HTMLInputElement>(null);

  // Custom starts off on a fresh target and for one configured purely by
  // picking from the public list, and on when editing one with any
  // hand-set advanced option — see hasCustomIperf3Options.
  const [custom, setCustom] = useState(() => hasCustomIperf3Options(options));

  // A failed submit blocked by validateEngineOptions (e.g. a password with
  // no username/RSA key) bumps this signal from the parent form; force
  // Custom on so the error isn't hidden behind a field Custom-off hides.
  useEffect(() => {
    if (forceOpenAdvancedSignal) setCustom(true);
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
    // Every pick fully replaces these two fields (not just sets them when
    // true) so switching from a server that supports -R / a wide port
    // range to one that doesn't clears the stale values instead of
    // leaving them stuck on from the previous pick.
    next = setOption(next, 'reverse', s.supports_reverse);
    next = setOption(next, 'port_range_end', s.port_end && s.port_end > s.port ? s.port_end : '');
    onChange(next);
    setSearch(`${s.host}:${s.port}`);
    setFocused(false);
  };

  const pickedHost = typeof options.host === 'string' ? options.host : '';
  const pickHints = summarizePick(options);

  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <div className="sm:col-span-2">
        <SwitchField
          id="iperf-custom" label="Custom" checked={custom} onCheckedChange={setCustom}
          hint="Enter a host and tune iperf3 flags yourself"
        />
      </div>

      {custom && (
        <div className="sm:col-span-2">
          <FormField id="iperf-host" label="Host">
            <input id="iperf-host" className={inputClass} value={text('host')}
              onChange={(e) => onChange(setOption(options, 'host', e.target.value))} />
          </FormField>
        </div>
      )}

      <div className="sm:col-span-2">
        <FormField id="iperf-public-search" label="Pick from public list">
        <Popover open={open} onOpenChange={(o) => { if (!o) setFocused(false); }}>
          <PopoverAnchor asChild>
            <input
              ref={anchorRef}
              id="iperf-public-search"
              className={inputClass}
              value={search}
              autoComplete="off"
              placeholder="host, site, country/continent or provider"
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
        </FormField>
        {!custom && pickedHost !== '' && (
          <div data-testid="iperf3-picked" className="mt-2 rounded-md border border-line bg-surface px-2 py-1.5 text-sm">
            <span className="font-mono text-fg">{pickedHost}:{options.port !== undefined ? String(options.port) : '5201'}</span>
            {pickHints.length > 0 && (
              <p className="mt-1 text-xs text-faint">{pickHints.join(' · ')}</p>
            )}
          </div>
        )}
      </div>

      {custom && (
      <div className="grid gap-3 sm:col-span-2 sm:grid-cols-2">
      <FormField id="iperf-port" label="Port">
        <input id="iperf-port" className={inputClass} value={text('port')} placeholder="5201"
          onChange={(e) => onChange(setOption(options, 'port', numberOr(e.target.value)))} />
      </FormField>
      <FormField id="iperf-port-range-end" label="Port range end">
        <input id="iperf-port-range-end" className={inputClass} value={text('port_range_end')}
          placeholder="retry ports up to this one when busy"
          onChange={(e) => onChange(setOption(options, 'port_range_end', numberOr(e.target.value)))} />
      </FormField>
      <FormField id="iperf-protocol" label="Protocol">
        <select id="iperf-protocol" className={inputClass} value={protocol}
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
      </FormField>
      <FormField id="iperf-parallel" label="Parallel streams">
        <input id="iperf-parallel" className={inputClass} value={text('parallel')} placeholder="1"
          onChange={(e) => onChange(setOption(options, 'parallel', numberOr(e.target.value)))} />
      </FormField>
      <FormField id="iperf-duration" label="Duration (s)">
        <input id="iperf-duration" className={inputClass} value={text('duration_s')} placeholder="10"
          onChange={(e) => onChange(setOption(options, 'duration_s', numberOr(e.target.value)))} />
      </FormField>
      <FormField id="iperf-bitrate" label="UDP bitrate">
        <input id="iperf-bitrate" className={inputClass} value={text('udp_bitrate')} placeholder="100M"
          disabled={!isUdp} title={isUdp ? undefined : 'Only applies to UDP'}
          onChange={(e) => onChange(setOption(options, 'udp_bitrate', e.target.value))} />
      </FormField>
      <FormField id="iperf-bind" label="Bind address">
        <input id="iperf-bind" className={inputClass} value={text('bind')}
          onChange={(e) => onChange(setOption(options, 'bind', e.target.value))} />
      </FormField>
      <FormField id="iperf-username" label="Username">
        <input id="iperf-username" className={inputClass} value={text('username')}
          onChange={(e) => onChange(setOption(options, 'username', e.target.value))} />
      </FormField>
      <FormField id="iperf-rsa" label="RSA public key path">
        <input id="iperf-rsa" className={inputClass} value={text('rsa_public_key_path')}
          onChange={(e) => onChange(setOption(options, 'rsa_public_key_path', e.target.value))} />
      </FormField>
      <FormField
        id="iperf-password" label="Password"
        error={passwordNeedsAuth ? 'Password requires a username and RSA public key path' : undefined}
      >
        <input id="iperf-password" type="password" className={inputClass} value={text('password')}
          onChange={(e) => onChange(setOption(options, 'password', e.target.value))} />
      </FormField>
      {/* Disable a box only when the other is checked and this one isn't —
          never both at once. Legacy options can have reverse and bidir both
          true (an invalid combination the engine would reject); if that
          happened, disabling both here would make it impossible to ever
          uncheck either one. */}
      <Label className="flex items-center gap-2" htmlFor="iperf-reverse">
        <Checkbox id="iperf-reverse" checked={reverseOn} disabled={bidirOn && !reverseOn}
          onCheckedChange={(checked) => onChange(setOption(options, 'reverse', checked === true))} />
        Reverse (-R)
      </Label>
      <Label className="flex items-center gap-2" htmlFor="iperf-bidir">
        <Checkbox id="iperf-bidir" checked={bidirOn} disabled={(reverseOn && !bidirOn) || isUdp}
          onCheckedChange={(checked) => onChange(setOption(options, 'bidir', checked === true))} />
        Bidirectional (--bidir)
      </Label>
      </div>
      )}
    </div>
  );
}
