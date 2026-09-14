import { useEffect, useState } from 'react';
import { useOoklaServers } from '../../lib/queries';

export type Options = Record<string, unknown>;

interface Props {
  engine: string;
  options: Options;
  onChange: (next: Options) => void;
}

const field = 'w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-sm text-slate-100 focus:border-sky-500 focus:outline-none';
const label = 'block text-xs font-medium uppercase tracking-wide text-slate-400';

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

/** EngineOptionFields renders the option form for one engine. */
export function EngineOptionFields({ engine, options, onChange }: Props) {
  if (engine === 'ookla') return <OoklaFields options={options} onChange={onChange} />;
  if (engine === 'cloudflare') return <CloudflareFields options={options} onChange={onChange} />;
  if (engine === 'iperf3') return <Iperf3Fields options={options} onChange={onChange} />;
  return (
    <p className="text-sm text-slate-400">
      The <span className="font-mono">{engine}</span> engine takes no configuration.
    </p>
  );
}

function OoklaFields({ options, onChange }: Omit<Props, 'engine'>) {
  const [search, setSearch] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');

  useEffect(() => {
    const t = setTimeout(() => setDebouncedSearch(search), DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [search]);

  const servers = useOoklaServers(debouncedSearch, debouncedSearch.trim().length >= 2);
  const serverId = options.server_id === undefined ? '' : String(options.server_id);

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
      <div>
        <label className={label} htmlFor="ookla-server-search">Search servers</label>
        <input
          id="ookla-server-search"
          className={field}
          value={search}
          placeholder="city, country or host"
          onChange={(e) => setSearch(e.target.value)}
        />
        {servers.isFetching && <p className="mt-1 text-xs text-slate-500">Searching…</p>}
        {servers.data && servers.data.length > 0 && (
          <ul className="mt-1 max-h-48 divide-y divide-slate-800 overflow-y-auto rounded border border-slate-800">
            {servers.data.slice(0, 50).map((s) => (
              <li key={s.id}>
                <button
                  type="button"
                  className="flex w-full items-baseline justify-between px-2 py-1 text-left text-sm hover:bg-slate-800"
                  onClick={() => onChange(setOption(options, 'server_id', Number(s.id)))}
                >
                  <span className="text-slate-200">{s.name}</span>
                  <span className="text-xs text-slate-500">{s.location}, {s.country}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
        {servers.isError && (
          <p className="mt-1 text-xs text-amber-400">Server list unavailable — enter an ID manually.</p>
        )}
      </div>
    </div>
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

function Iperf3Fields({ options, onChange }: Omit<Props, 'engine'>) {
  const text = (key: string) => (options[key] === undefined ? '' : String(options[key]));
  const checked = (key: string) => options[key] === true;

  const protocol = text('protocol') || 'tcp';
  const isUdp = protocol === 'udp';
  const reverseOn = checked('reverse');
  const bidirOn = checked('bidir');
  const passwordSet = typeof options.password === 'string' && options.password !== '';
  const passwordNeedsAuth = passwordSet && (!options.username || !options.rsa_public_key_path);

  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <div className="sm:col-span-2">
        <label className={label} htmlFor="iperf-host">Host</label>
        <input id="iperf-host" className={field} value={text('host')}
          onChange={(e) => onChange(setOption(options, 'host', e.target.value))} />
      </div>
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
          <p className="mt-1 text-xs text-amber-400">
            Password requires a username and RSA public key path
          </p>
        )}
      </div>
      {/* Disable a box only when the other is checked and this one isn't —
          never both at once. Legacy options can have reverse and bidir both
          true (an invalid combination the engine would reject); if that
          happened, disabling both here would make it impossible to ever
          uncheck either one. */}
      <label className="flex items-center gap-2 text-sm text-slate-300" htmlFor="iperf-reverse">
        <input id="iperf-reverse" type="checkbox" checked={reverseOn} disabled={bidirOn && !reverseOn}
          onChange={(e) => onChange(setOption(options, 'reverse', e.target.checked))} />
        Reverse (-R)
      </label>
      <label className="flex items-center gap-2 text-sm text-slate-300" htmlFor="iperf-bidir">
        <input id="iperf-bidir" type="checkbox" checked={bidirOn} disabled={(reverseOn && !bidirOn) || isUdp}
          onChange={(e) => onChange(setOption(options, 'bidir', e.target.checked))} />
        Bidirectional (--bidir)
      </label>
    </div>
  );
}
