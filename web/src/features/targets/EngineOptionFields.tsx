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
import { BYTE_UNITS, formatBytes, splitBytes, toBytes, type ByteUnit } from '../../lib/bytes';

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
  if (iperf3HostList(options).length === 0) {
    return 'At least one host is required';
  }
  const password = options.password;
  if (typeof password === 'string' && password !== '') {
    if (!options.username || !options.rsa_public_key_path) {
      return 'Password requires a username and RSA public key path';
    }
  }
  return undefined;
}

/** iperf3HostList reads the ordered host list from options, falling back
 * to the legacy single `host` field (as a one-element list) so an
 * existing single-host target still validates and edits cleanly. */
function iperf3HostList(options: Options): string[] {
  const hosts = options.hosts;
  if (Array.isArray(hosts) && hosts.every((h) => typeof h === 'string')) return hosts as string[];
  return typeof options.host === 'string' && options.host !== '' ? [options.host] : [];
}

/** ooklaServerIDList reads the ordered server-id list from options,
 * falling back to the legacy single `server_id` field. */
function ooklaServerIDList(options: Options): number[] {
  const ids = options.server_ids;
  if (Array.isArray(ids) && ids.every((n) => typeof n === 'number')) return ids as number[];
  return typeof options.server_id === 'number' ? [options.server_id] : [];
}

/** writeOrderedList stores `next` under `listKey`, dropping `singleKey`
 * entirely so the two never coexist: empty means "unset" (drop both),
 * one-or-more always writes the list — the backend folds a one-element
 * list back into the singular field itself, so this round-trips a
 * never-edited single-host/server target with no behavior change. */
function writeOrderedList(options: Options, listKey: string, singleKey: string, next: (string | number)[]): Options {
  let out = setOption(options, singleKey, '');
  out = setOption(out, listKey, next.length > 0 ? next : '');
  return out;
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
  // A rotating (multi-host) target only makes sense as a hand-configured
  // one — the public-list picker never writes `hosts` itself — so it
  // always opens with Custom on, same as any other advanced option below.
  if (Array.isArray(options.hosts)) return true;
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

/** RotationListEditor shows an ordered list of hosts/server ids with
 * ↑/↓/× per row — the markup mirrors SortableTargetList's Row (schedules'
 * target-order editor) minus the drag handle and dnd-kit, since this list
 * is always short enough that buttons alone are enough. */
function RotationListEditor({ idPrefix, items, onMove, onRemove }: {
  idPrefix: string;
  items: (string | number)[];
  onMove: (index: number, delta: number) => void;
  onRemove: (index: number) => void;
}) {
  if (items.length === 0) {
    return <p className="text-xs text-faint">No entries yet — add at least one below.</p>;
  }
  return (
    <ol className="grid gap-1" data-testid={`${idPrefix}-rotation-list`}>
      {items.map((item, i) => (
        <li
          key={`${idPrefix}-${i}`}
          className="flex items-center gap-2 rounded border border-line bg-app px-2 py-1 text-sm"
        >
          <span className="w-5 text-right font-mono text-xs text-faint">{i + 1}</span>
          <span className="flex-1 truncate font-mono text-fg">{item}</span>
          <button
            type="button" aria-label={`Move ${item} up`} disabled={i === 0}
            className="rounded border border-line px-1.5 text-xs text-muted disabled:opacity-40"
            onClick={() => onMove(i, -1)}
          >↑</button>
          <button
            type="button" aria-label={`Move ${item} down`} disabled={i === items.length - 1}
            className="rounded border border-line px-1.5 text-xs text-muted disabled:opacity-40"
            onClick={() => onMove(i, 1)}
          >↓</button>
          <button
            type="button" aria-label={`Remove ${item}`}
            className="rounded border border-line px-1.5 text-xs text-muted"
            onClick={() => onRemove(i)}
          >×</button>
        </li>
      ))}
    </ol>
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
  // Rotation mode starts on iff the target already carries a server_ids
  // list (however long) so an existing rotating target reopens the way it
  // was saved; a plain server_id target opens in the original single-value
  // form untouched, satisfying "must load and save without change".
  const [rotating, setRotating] = useState(() => Array.isArray(options.server_ids));
  const [addValue, setAddValue] = useState('');
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
  const ids = ooklaServerIDList(options);

  const addID = (id: number) => {
    if (!Number.isFinite(id) || id <= 0 || ids.includes(id)) return;
    onChange(writeOrderedList(options, 'server_ids', 'server_id', [...ids, id]));
  };

  const handleSelect = (s: OoklaServer) => {
    if (rotating) {
      addID(Number(s.id));
    } else {
      onChange(setOption(options, 'server_id', Number(s.id)));
    }
    setSearch(s.sponsor ? `${s.sponsor} — ${s.location}` : s.name);
    setFocused(false);
  };

  const handleRotatingChange = (next: boolean) => {
    if (next) {
      onChange(writeOrderedList(options, 'server_ids', 'server_id', ids));
    } else {
      let out = setOption(options, 'server_ids', '');
      out = setOption(out, 'server_id', ids[0] ?? '');
      onChange(out);
    }
    setRotating(next);
  };

  return (
    <div className="grid gap-3">
      <SwitchField
        id="ookla-rotate" label="Rotate through multiple servers" checked={rotating}
        onCheckedChange={handleRotatingChange}
        hint="Each run uses the next server in the list, in order"
      />

      {!rotating && (
        <FormField id="ookla-server-id" label="Ookla server ID">
          <input
            id="ookla-server-id"
            className={inputClass}
            value={serverId}
            placeholder="auto (nearest server)"
            onChange={(e) => onChange(setOption(options, 'server_id', numberOr(e.target.value)))}
          />
        </FormField>
      )}

      {rotating && (
        <div className="grid gap-2">
          <RotationListEditor
            idPrefix="ookla"
            items={ids.map((id) => `Server ${id}`)}
            onMove={(i, delta) => {
              const to = i + delta;
              if (to < 0 || to >= ids.length) return;
              const next = [...ids];
              [next[i], next[to]] = [next[to], next[i]];
              onChange(writeOrderedList(options, 'server_ids', 'server_id', next));
            }}
            onRemove={(i) => onChange(
              writeOrderedList(options, 'server_ids', 'server_id', ids.filter((_, idx) => idx !== i)),
            )}
          />
          <div className="flex gap-2">
            <input
              aria-label="Add server ID"
              className={inputClass}
              value={addValue}
              placeholder="server ID"
              onChange={(e) => setAddValue(e.target.value)}
              onKeyDown={(e) => {
                if (e.key !== 'Enter') return;
                e.preventDefault();
                addID(Number(addValue));
                setAddValue('');
              }}
            />
            <button
              type="button"
              className="shrink-0 rounded border border-line px-3 text-sm text-muted hover:bg-raised"
              onClick={() => { addID(Number(addValue)); setAddValue(''); }}
            >
              Add
            </button>
          </div>
        </div>
      )}

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

export const CF_SIZE_PRESETS: number[] = [1e5, 1e6, 1e7, 2.5e7, 1e8];
export const CF_DEFAULT_DOWNLOAD: number[] = [1e6, 1e7, 2.5e7, 1e8];
export const CF_DEFAULT_UPLOAD: number[] = [1e5, 1e6, 1e7];
export const CF_LATENCY_PRESETS: number[] = [5, 10, 20, 50];
export const CF_DEFAULT_LATENCY = 10;

/** sizeList reads a cloudflare size-list option, falling back to `fallback`
 * (the engine's own default list) when the option isn't a number array. */
export function sizeList(options: Options, key: string, fallback: number[]): number[] {
  const v = options[key];
  return Array.isArray(v) && v.every((n) => typeof n === 'number') ? (v as number[]) : fallback;
}

/** sameNumbers compares two number arrays regardless of order. */
export function sameNumbers(a: number[], b: number[]): boolean {
  if (a.length !== b.length) return false;
  const sa = [...a].sort((x, y) => x - y);
  const sb = [...b].sort((x, y) => x - y);
  return sa.every((v, i) => v === sb[i]);
}

/** writeSizes stores a sorted copy of `next` under `key`, omitting the key
 * entirely when the list is empty or matches the engine's own defaults. */
export function writeSizes(options: Options, key: string, next: number[], defaults: number[]): Options {
  const sorted = [...next].sort((a, b) => a - b);
  if (sorted.length === 0 || sameNumbers(sorted, defaults)) return setOption(options, key, '');
  return setOption(options, key, sorted);
}

/** hasCustomCloudflareOptions decides Custom sizes' initial state: on iff
 * any stored download/upload size isn't one of the presets, or a
 * latency_samples value is set that isn't one of the latency presets. */
export function hasCustomCloudflareOptions(options: Options): boolean {
  const hasNonPresetSize = (v: unknown) =>
    Array.isArray(v) && v.some((n) => typeof n === 'number' && !CF_SIZE_PRESETS.includes(n));
  if (hasNonPresetSize(options.download_sizes) || hasNonPresetSize(options.upload_sizes)) return true;
  const latency = options.latency_samples;
  return latency !== undefined && !CF_LATENCY_PRESETS.includes(latency as number);
}

interface SizeRow {
  id: number;
  value: string;
  unit: ByteUnit;
}

function rowsFromList(list: number[], nextId: () => number): SizeRow[] {
  return list.map((n) => {
    const split = splitBytes(n);
    return { id: nextId(), value: String(split.value), unit: split.unit };
  });
}

/** SizePresetChips renders one sizes field as a row of preset checkboxes,
 * used while Custom sizes is off. `usingDefaults` is true when the option
 * key is unset (either untouched, or emptied back down to nothing — the
 * engine can't represent "no sizes", so an empty selection reverts to
 * showing the defaults checked; the note explains why that just happened). */
function SizePresetChips({ idPrefix, label, presets, selected, emptied, onToggle }: {
  idPrefix: string;
  label: string;
  presets: number[];
  selected: number[];
  /** emptied is true right after the user has unchecked every chip in this
   * field: the engine can't represent "no sizes", so `selected` reverts to
   * showing the defaults checked again — this flags that revert so a note
   * can explain it, rather than just silently un-doing the click. */
  emptied: boolean;
  onToggle: (size: number, checked: boolean) => void;
}) {
  return (
    <FormField id={`cf-${idPrefix}-sizes`} label={label}>
      <div data-testid={`cf-${idPrefix}-chips`} className="flex flex-wrap gap-3">
        {presets.map((size) => {
          const id = `cf-${idPrefix}-${size}`;
          return (
            <Label key={size} htmlFor={id} className="flex items-center gap-2 text-sm text-muted">
              <Checkbox
                id={id}
                checked={selected.includes(size)}
                onCheckedChange={(checked) => onToggle(size, checked === true)}
              />
              {formatBytes(size)}
            </Label>
          );
        })}
      </div>
      {emptied && <p className="text-xs text-faint">Using engine defaults</p>}
    </FormField>
  );
}

/** SizeRowsEditor renders one sizes field as editable value+unit rows, used
 * while Custom sizes is on. Row state (including in-progress unit-less
 * edits) is kept by the caller so a half-typed row isn't reordered or lost
 * before it's committed via `onCommit`. */
function SizeRowsEditor({ idPrefix, label, rows, onRowsChange, onCommit, nextId }: {
  idPrefix: string;
  label: string;
  rows: SizeRow[];
  onRowsChange: (rows: SizeRow[]) => void;
  onCommit: (rows: SizeRow[]) => void;
  nextId: () => number;
}) {
  const update = (next: SizeRow[]) => { onRowsChange(next); onCommit(next); };
  return (
    <FormField id={`cf-${idPrefix}-sizes`} label={label}>
      <div className="grid gap-2">
        {rows.length === 0 && <p className="text-xs text-faint">Using engine defaults</p>}
        {rows.map((row, i) => (
          <div key={row.id} className="flex items-center gap-2">
            <input
              aria-label={`${idPrefix} size ${i + 1}`}
              className={inputClass}
              value={row.value}
              onChange={(e) => update(rows.map((r) => (r.id === row.id ? { ...r, value: e.target.value } : r)))}
            />
            <select
              aria-label={`${idPrefix} size ${i + 1} unit`}
              className={inputClass}
              value={row.unit}
              onChange={(e) => update(rows.map((r) => (r.id === row.id ? { ...r, unit: e.target.value as ByteUnit } : r)))}
            >
              {BYTE_UNITS.map((u) => <option key={u} value={u}>{u}</option>)}
            </select>
            <button
              type="button"
              className="shrink-0 text-xs text-faint hover:text-fg"
              onClick={() => update(rows.filter((r) => r.id !== row.id))}
            >
              Remove
            </button>
          </div>
        ))}
        <button
          type="button"
          className="w-fit text-xs text-accent hover:underline"
          onClick={() => update([...rows, { id: nextId(), value: '1', unit: 'MB' }])}
        >
          Add size
        </button>
      </div>
    </FormField>
  );
}

function CloudflareFields({ options, onChange }: Omit<Props, 'engine'>) {
  // Custom sizes starts on iff any stored option is outside the presets —
  // see hasCustomCloudflareOptions.
  const [custom, setCustom] = useState(() => hasCustomCloudflareOptions(options));
  const rowIdRef = useRef(0);
  const nextRowId = () => { rowIdRef.current += 1; return rowIdRef.current; };

  const [downloadRows, setDownloadRows] = useState<SizeRow[]>(
    () => rowsFromList(sizeList(options, 'download_sizes', CF_DEFAULT_DOWNLOAD), nextRowId),
  );
  const [uploadRows, setUploadRows] = useState<SizeRow[]>(
    () => rowsFromList(sizeList(options, 'upload_sizes', CF_DEFAULT_UPLOAD), nextRowId),
  );

  // Tracks "the user just unchecked the last chip in this field" — see
  // SizePresetChips' `emptied` doc. Local (not derived from options)
  // because the option key ends up omitted either way, same as it is on a
  // fresh, untouched target.
  const [downloadEmptied, setDownloadEmptied] = useState(false);
  const [uploadEmptied, setUploadEmptied] = useState(false);

  // The server stores sizes as a Go []int, so a fractional byte count (e.g.
  // 1.5 B, or 16.1 MB — which is 16100000.000000002 through toBytes due to
  // float multiplication) must be rounded before it's sent, and a value
  // that rounds to <= 0 is dropped rather than sent as a non-positive size.
  const commitRows = (key: string, defaults: number[]) => (rows: SizeRow[]) => {
    const bytes = rows
      .map((r) => {
        const n = Number(r.value);
        if (!Number.isFinite(n)) return undefined;
        const rounded = Math.round(toBytes(n, r.unit));
        return rounded > 0 ? rounded : undefined;
      })
      .filter((n): n is number => n !== undefined);
    onChange(writeSizes(options, key, bytes, defaults));
  };

  const toggleSize = (key: string, defaults: number[], setEmptied: (v: boolean) => void) => (
    size: number, checked: boolean,
  ) => {
    const current = sizeList(options, key, defaults);
    const next = checked ? [...current, size] : current.filter((n) => n !== size);
    setEmptied(next.length === 0);
    onChange(writeSizes(options, key, next, defaults));
  };

  const handleCustomChange = (next: boolean) => {
    if (next) {
      setDownloadRows(rowsFromList(sizeList(options, 'download_sizes', CF_DEFAULT_DOWNLOAD), nextRowId));
      setUploadRows(rowsFromList(sizeList(options, 'upload_sizes', CF_DEFAULT_UPLOAD), nextRowId));
    } else {
      const dl = sizeList(options, 'download_sizes', CF_DEFAULT_DOWNLOAD).filter((n) => CF_SIZE_PRESETS.includes(n));
      const ul = sizeList(options, 'upload_sizes', CF_DEFAULT_UPLOAD).filter((n) => CF_SIZE_PRESETS.includes(n));
      let nextOptions = writeSizes(options, 'download_sizes', dl, CF_DEFAULT_DOWNLOAD);
      nextOptions = writeSizes(nextOptions, 'upload_sizes', ul, CF_DEFAULT_UPLOAD);
      // A custom latency value (e.g. 7) has no matching <option> in the
      // preset <select> once Custom sizes goes off — drop it, same as the
      // size lists above, so the select doesn't render with a stale value.
      const lat = nextOptions.latency_samples;
      if (typeof lat === 'number' && !CF_LATENCY_PRESETS.includes(lat)) {
        nextOptions = setOption(nextOptions, 'latency_samples', '');
      }
      onChange(nextOptions);
    }
    setDownloadEmptied(false);
    setUploadEmptied(false);
    setCustom(next);
  };

  // The server wants an integer >= 0; round fractional input and drop
  // negative input entirely rather than send a value it would reject.
  const handleCustomLatencyChange = (raw: string) => {
    if (raw.trim() === '') { onChange(setOption(options, 'latency_samples', '')); return; }
    const n = Number(raw);
    if (!Number.isFinite(n) || n < 0) { onChange(setOption(options, 'latency_samples', '')); return; }
    onChange(setOption(options, 'latency_samples', Math.round(n)));
  };

  const latency = typeof options.latency_samples === 'number' ? options.latency_samples : CF_DEFAULT_LATENCY;

  return (
    <div className="grid gap-3">
      <SwitchField
        id="cf-custom" label="Custom sizes" checked={custom} onCheckedChange={handleCustomChange}
        hint="Enter any sizes with units"
      />

      {custom ? (
        <>
          <SizeRowsEditor
            idPrefix="download" label="Download sizes" rows={downloadRows}
            onRowsChange={setDownloadRows} onCommit={commitRows('download_sizes', CF_DEFAULT_DOWNLOAD)}
            nextId={nextRowId}
          />
          <SizeRowsEditor
            idPrefix="upload" label="Upload sizes" rows={uploadRows}
            onRowsChange={setUploadRows} onCommit={commitRows('upload_sizes', CF_DEFAULT_UPLOAD)}
            nextId={nextRowId}
          />
          <FormField id="cf-latency-samples" label="Latency samples">
            <input
              id="cf-latency-samples"
              className={inputClass}
              value={options.latency_samples === undefined ? '' : String(options.latency_samples)}
              onChange={(e) => handleCustomLatencyChange(e.target.value)}
            />
          </FormField>
        </>
      ) : (
        <>
          <SizePresetChips
            idPrefix="download" label="Download sizes" presets={CF_SIZE_PRESETS}
            selected={sizeList(options, 'download_sizes', CF_DEFAULT_DOWNLOAD)}
            emptied={downloadEmptied}
            onToggle={toggleSize('download_sizes', CF_DEFAULT_DOWNLOAD, setDownloadEmptied)}
          />
          <SizePresetChips
            idPrefix="upload" label="Upload sizes" presets={CF_SIZE_PRESETS}
            selected={sizeList(options, 'upload_sizes', CF_DEFAULT_UPLOAD)}
            emptied={uploadEmptied}
            onToggle={toggleSize('upload_sizes', CF_DEFAULT_UPLOAD, setUploadEmptied)}
          />
          <FormField id="cf-latency-samples" label="Latency samples">
            <select
              id="cf-latency-samples"
              className={inputClass}
              value={String(latency)}
              onChange={(e) => {
                const n = Number(e.target.value);
                onChange(setOption(options, 'latency_samples', n === CF_DEFAULT_LATENCY ? '' : n));
              }}
            >
              {CF_LATENCY_PRESETS.map((n) => <option key={n} value={n}>{n}</option>)}
            </select>
          </FormField>
        </>
      )}
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

  // Rotation mode starts on iff the target already carries a hosts list
  // (however long), same reasoning as OoklaFields' `rotating` — a plain
  // single-host target opens in the original Host-input form untouched.
  const [rotating, setRotating] = useState(() => Array.isArray(options.hosts));
  const [addHostValue, setAddHostValue] = useState('');

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

  const hosts = iperf3HostList(options);

  const addHost = (host: string) => {
    const h = host.trim();
    if (h === '' || hosts.includes(h)) return;
    onChange(writeOrderedList(options, 'hosts', 'host', [...hosts, h]));
  };

  const handleRotatingChange = (next: boolean) => {
    if (next) {
      onChange(writeOrderedList(options, 'hosts', 'host', hosts));
    } else {
      let out = setOption(options, 'hosts', '');
      out = setOption(out, 'host', hosts[0] ?? '');
      onChange(out);
    }
    setRotating(next);
  };

  const handlePick = (s: Iperf3Server) => {
    if (rotating) {
      if (!hosts.includes(s.host)) {
        let next = writeOrderedList(options, 'hosts', 'host', [...hosts, s.host]);
        // Port and the -R/port-range flags are shared across every host in
        // the list, so only the first pick sets them; later picks just add
        // the host without disturbing what's already configured.
        if (hosts.length === 0) {
          next = setOption(next, 'port', s.port);
          next = setOption(next, 'reverse', s.supports_reverse);
          next = setOption(next, 'port_range_end', s.port_end && s.port_end > s.port ? s.port_end : '');
        }
        onChange(next);
      }
    } else {
      let next = setOption(options, 'host', s.host);
      next = setOption(next, 'port', s.port);
      // Every pick fully replaces these two fields (not just sets them when
      // true) so switching from a server that supports -R / a wide port
      // range to one that doesn't clears the stale values instead of
      // leaving them stuck on from the previous pick.
      next = setOption(next, 'reverse', s.supports_reverse);
      next = setOption(next, 'port_range_end', s.port_end && s.port_end > s.port ? s.port_end : '');
      onChange(next);
    }
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
        <div className="sm:col-span-2 grid gap-2">
          <SwitchField
            id="iperf-rotate" label="Rotate through multiple hosts" checked={rotating}
            onCheckedChange={handleRotatingChange}
            hint="Each run uses the next host in the list, in order"
          />

          {!rotating && (
            <FormField id="iperf-host" label="Host">
              <input id="iperf-host" className={inputClass} value={text('host')}
                onChange={(e) => onChange(setOption(options, 'host', e.target.value))} />
            </FormField>
          )}

          {rotating && (
            <div className="grid gap-2">
              <RotationListEditor
                idPrefix="iperf3"
                items={hosts}
                onMove={(i, delta) => {
                  const to = i + delta;
                  if (to < 0 || to >= hosts.length) return;
                  const next = [...hosts];
                  [next[i], next[to]] = [next[to], next[i]];
                  onChange(writeOrderedList(options, 'hosts', 'host', next));
                }}
                onRemove={(i) => onChange(
                  writeOrderedList(options, 'hosts', 'host', hosts.filter((_, idx) => idx !== i)),
                )}
              />
              <div className="flex gap-2">
                <input
                  aria-label="Add host"
                  className={inputClass}
                  value={addHostValue}
                  placeholder="host"
                  onChange={(e) => setAddHostValue(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key !== 'Enter') return;
                    e.preventDefault();
                    addHost(addHostValue);
                    setAddHostValue('');
                  }}
                />
                <button
                  type="button"
                  className="shrink-0 rounded border border-line px-3 text-sm text-muted hover:bg-raised"
                  onClick={() => { addHost(addHostValue); setAddHostValue(''); }}
                >
                  Add
                </button>
              </div>
            </div>
          )}
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
