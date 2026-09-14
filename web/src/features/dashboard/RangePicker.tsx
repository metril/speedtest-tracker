import { useRef } from 'react';
import type { Range } from '../../lib/api';

const RANGES: Range[] = ['24h', '7d', '30d'];

/** RangePicker selects the dashboard window. */
export function RangePicker({ value, onChange }: { value: Range; onChange: (r: Range) => void }) {
  const btnRefs = useRef<(HTMLButtonElement | null)[]>([]);

  const select = (index: number) => {
    const r = RANGES[index];
    onChange(r);
    btnRefs.current[index]?.focus();
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLButtonElement>, index: number) => {
    switch (e.key) {
      case 'ArrowLeft':
      case 'ArrowUp':
        e.preventDefault();
        select((index - 1 + RANGES.length) % RANGES.length);
        break;
      case 'ArrowRight':
      case 'ArrowDown':
        e.preventDefault();
        select((index + 1) % RANGES.length);
        break;
      case 'Home':
        e.preventDefault();
        select(0);
        break;
      case 'End':
        e.preventDefault();
        select(RANGES.length - 1);
        break;
      default:
        break;
    }
  };

  return (
    <div role="radiogroup" aria-label="Time range" className="flex rounded border border-line text-xs">
      {RANGES.map((r, i) => (
        <button key={r} ref={(el) => { btnRefs.current[i] = el; }}
          type="button" role="radio" aria-checked={value === r}
          tabIndex={value === r ? 0 : -1} onClick={() => onChange(r)}
          onKeyDown={(e) => onKeyDown(e, i)}
          className={`px-2.5 py-1 first:rounded-l last:rounded-r ${
            value === r ? 'bg-accent text-accent-fg' : 'text-muted hover:text-fg'}`}>
          {r}
        </button>
      ))}
    </div>
  );
}
