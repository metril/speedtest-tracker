import type { ResultFilters, Target } from '../../lib/api';
import { useTags } from '../../lib/queries';

interface Props {
  value: ResultFilters;
  onChange: (next: ResultFilters) => void;
  targets: Target[];
}

const control = 'rounded border border-line bg-surface px-2 py-1 text-sm text-fg focus:border-accent focus:outline-none';

const pad = (n: number) => String(n).padStart(2, '0');

/** localInputToISO converts a <input type="datetime-local"> value (which
 * carries no timezone and is interpreted as local time by `new Date`) to a
 * UTC ISO-8601 string for the API. */
function localInputToISO(v: string): string | undefined {
  if (!v) return undefined;
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return undefined;
  return d.toISOString();
}

/** isoToLocalInput converts a UTC ISO-8601 string back to the
 * minute-precision local-time value a <input type="datetime-local"> needs,
 * so the field displays and round-trips in the viewer's own timezone. */
function isoToLocalInput(iso: string | undefined): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** ResultFiltersBar narrows the results listing. */
export function ResultFiltersBar({ value, onChange, targets }: Props) {
  const set = (patch: Partial<ResultFilters>) => onChange({ ...value, ...patch });
  const tags = useTags();
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
      <select className={control} aria-label="Tag" value={value.tag ?? ''}
        onChange={(e) => set({ tag: e.target.value || undefined })}>
        <option value="">All tags</option>
        {(tags.data ?? []).map((t) => <option key={t.id} value={t.name}>{t.name}</option>)}
      </select>
      <input className={control} type="datetime-local" aria-label="From"
        value={isoToLocalInput(value.from)}
        onChange={(e) => set({ from: localInputToISO(e.target.value) })} />
      <input className={control} type="datetime-local" aria-label="To"
        value={isoToLocalInput(value.to)}
        onChange={(e) => set({ to: localInputToISO(e.target.value) })} />
      <button className="rounded border border-line px-2 py-1 text-sm text-muted hover:bg-raised"
        onClick={() => onChange({})}>
        Clear
      </button>
    </div>
  );
}
