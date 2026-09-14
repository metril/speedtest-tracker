import type { ResultFilters, Target } from '../../lib/api';

interface Props {
  value: ResultFilters;
  onChange: (next: ResultFilters) => void;
  targets: Target[];
}

const control = 'rounded border border-slate-700 bg-slate-900 px-2 py-1 text-sm text-slate-100 focus:border-sky-500 focus:outline-none';

/** ResultFiltersBar narrows the results listing. */
export function ResultFiltersBar({ value, onChange, targets }: Props) {
  const set = (patch: Partial<ResultFilters>) => onChange({ ...value, ...patch });
  return (
    <div className="flex flex-wrap items-end gap-2">
      <select
        className={control}
        aria-label="Target"
        value={value.target_id ?? ''}
        onChange={(e) => set({ target_id: e.target.value ? Number(e.target.value) : undefined })}
      >
        <option value="">All targets</option>
        {targets.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
      </select>
      <select className={control} aria-label="Engine" value={value.engine ?? ''}
        onChange={(e) => set({ engine: e.target.value || undefined })}>
        <option value="">All engines</option>
        {['ookla', 'cloudflare', 'iperf3', 'fake'].map((e) => <option key={e} value={e}>{e}</option>)}
      </select>
      <select className={control} aria-label="Status" value={value.status ?? ''}
        onChange={(e) => set({ status: e.target.value || undefined })}>
        <option value="">Any status</option>
        {['ok', 'failed', 'degraded'].map((s) => <option key={s} value={s}>{s}</option>)}
      </select>
      <input className={control} type="datetime-local" aria-label="From"
        value={value.from?.slice(0, 16) ?? ''}
        onChange={(e) => set({ from: e.target.value ? `${e.target.value}:00.000Z` : undefined })} />
      <input className={control} type="datetime-local" aria-label="To"
        value={value.to?.slice(0, 16) ?? ''}
        onChange={(e) => set({ to: e.target.value ? `${e.target.value}:00.000Z` : undefined })} />
      <button className="rounded border border-slate-700 px-2 py-1 text-sm text-slate-300 hover:bg-slate-800"
        onClick={() => onChange({})}>
        Clear
      </button>
    </div>
  );
}
