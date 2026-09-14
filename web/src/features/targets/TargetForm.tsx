import { useState } from 'react';
import type { Target, TargetInput } from '../../lib/api';
import { EngineOptionFields, type Options } from './EngineOptionFields';

const ENGINES = ['ookla', 'cloudflare', 'iperf3', 'fake'] as const;
const LANES = ['wan', 'lan'] as const;

interface Props {
  initial?: Target;
  onSubmit: (input: TargetInput) => void;
  onCancel: () => void;
  submitting: boolean;
  error?: string;
}

const field = 'w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-sm text-slate-100 focus:border-sky-500 focus:outline-none';
const label = 'block text-xs font-medium uppercase tracking-wide text-slate-400';

/** TargetForm creates or edits one target. */
export function TargetForm({ initial, onSubmit, onCancel, submitting, error }: Props) {
  const [name, setName] = useState(initial?.name ?? '');
  const [engine, setEngine] = useState(initial?.engine ?? 'ookla');
  const [lane, setLane] = useState(initial?.lane ?? 'wan');
  const [enabled, setEnabled] = useState(initial?.enabled ?? true);
  const [options, setOptions] = useState<Options>(initial?.options ?? {});
  const [touched, setTouched] = useState(false);

  const nameInvalid = name.trim() === '';

  return (
    <form
      className="grid gap-4 rounded-lg border border-slate-800 bg-slate-900/60 p-4"
      onSubmit={(e) => {
        e.preventDefault();
        setTouched(true);
        if (nameInvalid) return;
        onSubmit({ name: name.trim(), engine, enabled, lane, options });
      }}
    >
      <div className="grid gap-3 sm:grid-cols-3">
        <div>
          <label className={label} htmlFor="target-name">Name</label>
          <input id="target-name" className={field} value={name}
            onChange={(e) => setName(e.target.value)} />
          {touched && nameInvalid && (
            <p className="mt-1 text-xs text-rose-400">Name is required</p>
          )}
        </div>
        <div>
          <label className={label} htmlFor="target-engine">Engine</label>
          <select id="target-engine" className={field} value={engine}
            onChange={(e) => { setEngine(e.target.value); setOptions({}); }}>
            {ENGINES.map((e) => <option key={e} value={e}>{e}</option>)}
          </select>
        </div>
        <div>
          <label className={label} htmlFor="target-lane">Lane</label>
          <select id="target-lane" className={field} value={lane}
            onChange={(e) => setLane(e.target.value)}>
            {LANES.map((l) => <option key={l} value={l}>{l}</option>)}
          </select>
        </div>
      </div>

      <label className="flex w-fit items-center gap-2 text-sm text-slate-300" htmlFor="target-enabled">
        <input id="target-enabled" type="checkbox" checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)} />
        Enabled
      </label>

      <div className="border-t border-slate-800 pt-3">
        <EngineOptionFields engine={engine} options={options} onChange={setOptions} />
      </div>

      {error && <p className="text-sm text-rose-400">{error}</p>}

      <div className="flex gap-2">
        <button type="submit" disabled={submitting}
          className="rounded bg-sky-500 px-3 py-1.5 text-sm font-medium text-slate-950 hover:bg-sky-400 disabled:opacity-50">
          Save target
        </button>
        <button type="button" onClick={onCancel}
          className="rounded border border-slate-700 px-3 py-1.5 text-sm text-slate-300 hover:bg-slate-800">
          Cancel
        </button>
      </div>
    </form>
  );
}
