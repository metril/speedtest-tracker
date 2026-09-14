import type { Range } from '../../lib/api';

const RANGES: Range[] = ['24h', '7d', '30d'];

/** RangePicker selects the dashboard window. */
export function RangePicker({ value, onChange }: { value: Range; onChange: (r: Range) => void }) {
  return (
    <div role="radiogroup" aria-label="Time range" className="flex rounded border border-line text-xs">
      {RANGES.map((r) => (
        <button key={r} type="button" role="radio" aria-checked={value === r}
          tabIndex={value === r ? 0 : -1} onClick={() => onChange(r)}
          className={`px-2.5 py-1 first:rounded-l last:rounded-r ${
            value === r ? 'bg-accent text-accent-fg' : 'text-muted hover:text-fg'}`}>
          {r}
        </button>
      ))}
    </div>
  );
}
