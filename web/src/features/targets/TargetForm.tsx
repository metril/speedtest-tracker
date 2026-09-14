import { useState } from 'react';
import type { Target, TargetInput } from '../../lib/api';
import { EngineOptionFields, validateEngineOptions, type Options } from './EngineOptionFields';

const ENGINES = ['ookla', 'cloudflare', 'iperf3', 'fake'] as const;
const LANES = ['wan', 'lan'] as const;

interface Props {
  initial?: Target;
  onSubmit: (input: TargetInput) => void;
  onCancel: () => void;
  submitting: boolean;
  error?: string;
}

const field = 'w-full rounded border border-line bg-surface px-2 py-1 text-sm text-fg focus:border-accent focus:outline-none';
const label = 'block text-xs font-medium uppercase tracking-wide text-muted';

/** TargetForm creates or edits one target. */
export function TargetForm({ initial, onSubmit, onCancel, submitting, error }: Props) {
  const [name, setName] = useState(initial?.name ?? '');
  const [engine, setEngine] = useState(initial?.engine ?? 'ookla');
  const [lane, setLane] = useState(initial?.lane ?? 'wan');
  const [enabled, setEnabled] = useState(initial?.enabled ?? true);
  const [options, setOptions] = useState<Options>(initial?.options ?? {});
  const [touched, setTouched] = useState(false);

  const nameInvalid = name.trim() === '';
  const optionsError = validateEngineOptions(engine, options);

  return (
    <form
      className="grid gap-4 rounded-lg border border-line bg-raised p-4"
      onSubmit={(e) => {
        e.preventDefault();
        setTouched(true);
        if (nameInvalid || optionsError) return;
        onSubmit({ name: name.trim(), engine, enabled, lane, options });
      }}
    >
      <div className="grid gap-3 sm:grid-cols-3">
        <div>
          <label className={label} htmlFor="target-name">Name</label>
          <input id="target-name" className={field} value={name}
            onChange={(e) => setName(e.target.value)} />
          {touched && nameInvalid && (
            <p className="mt-1 text-xs text-bad">Name is required</p>
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

      <label className="flex w-fit items-center gap-2 text-sm text-muted" htmlFor="target-enabled">
        <input id="target-enabled" type="checkbox" checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)} />
        Enabled
      </label>

      <div className="border-t border-line pt-3">
        <EngineOptionFields engine={engine} options={options} onChange={setOptions} />
      </div>

      {error && <p className="text-sm text-bad">{error}</p>}

      <div className="flex gap-2">
        <button type="submit" disabled={submitting}
          className="rounded bg-accent px-3 py-1.5 text-sm font-medium text-accent-fg hover:opacity-90 disabled:opacity-50">
          Save target
        </button>
        <button type="button" onClick={onCancel}
          className="rounded border border-line px-3 py-1.5 text-sm text-muted hover:bg-raised">
          Cancel
        </button>
      </div>
    </form>
  );
}
