import { useMemo, useState } from 'react';
import { Checkbox } from '@/components/ui/checkbox';
import type { Target } from '../../lib/api';

interface Props {
  targets: Target[];
  selected: number[];
  onChange: (ids: number[]) => void;
}

/** TargetPicker lets a schedule pick targets out of a potentially large
 * pool: text search on name, engine/queue chip filters, and a checkbox list
 * with bulk select/clear of whatever currently matches the filters.
 * Checking a box appends to the selection order; unchecking removes it —
 * reordering the selection itself is SortableTargetList's job. */
export function TargetPicker({ targets, selected, onChange }: Props) {
  const [query, setQuery] = useState('');
  const [engines, setEngines] = useState<Set<string>>(new Set());
  const [queues, setQueues] = useState<Set<string>>(new Set());

  const allEngines = useMemo(
    () => [...new Set(targets.map((t) => t.engine))].sort(),
    [targets],
  );
  const allQueues = useMemo(
    () => [...new Set(targets.map((t) => t.queue_name))].sort(),
    [targets],
  );

  const selectedSet = useMemo(() => new Set(selected), [selected]);

  const matching = useMemo(() => {
    const q = query.trim().toLowerCase();
    return targets.filter((t) => {
      if (q && !t.name.toLowerCase().includes(q)) return false;
      if (engines.size > 0 && !engines.has(t.engine)) return false;
      if (queues.size > 0 && !queues.has(t.queue_name)) return false;
      return true;
    });
  }, [targets, query, engines, queues]);

  const toggleChip = (set: Set<string>, setSet: (s: Set<string>) => void, value: string) => {
    const next = new Set(set);
    if (next.has(value)) next.delete(value); else next.add(value);
    setSet(next);
  };

  const toggleTarget = (id: number) => {
    if (selectedSet.has(id)) {
      onChange(selected.filter((x) => x !== id));
    } else {
      onChange([...selected, id]);
    }
  };

  const selectAllMatching = () => {
    const toAdd = matching.filter((t) => !selectedSet.has(t.id)).map((t) => t.id);
    if (toAdd.length === 0) return;
    onChange([...selected, ...toAdd]);
  };

  const clearMatching = () => {
    const matchingIDs = new Set(matching.map((t) => t.id));
    onChange(selected.filter((id) => !matchingIDs.has(id)));
  };

  return (
    <div className="grid gap-2">
      <input
        type="text"
        aria-label="Search targets"
        placeholder="Search targets by name…"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        className="rounded border border-line bg-app px-2 py-1.5 text-sm text-fg"
      />

      {allEngines.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {allEngines.map((engine) => (
            <button
              key={engine} type="button" aria-pressed={engines.has(engine)}
              onClick={() => toggleChip(engines, setEngines, engine)}
              className={`rounded-full border px-2 py-0.5 text-xs uppercase ${
                engines.has(engine) ? 'border-accent bg-accent/10 text-accent' : 'border-line text-muted hover:bg-raised'
              }`}
            >
              {engine}
            </button>
          ))}
          {allQueues.map((queue) => (
            <button
              key={queue} type="button" aria-pressed={queues.has(queue)}
              onClick={() => toggleChip(queues, setQueues, queue)}
              className={`rounded-full border px-2 py-0.5 text-xs ${
                queues.has(queue) ? 'border-accent bg-accent/10 text-accent' : 'border-line text-muted hover:bg-raised'
              }`}
            >
              {queue}
            </button>
          ))}
        </div>
      )}

      <div className="flex gap-2">
        <button type="button" onClick={selectAllMatching}
          className="rounded border border-line px-2 py-1 text-xs text-muted hover:bg-raised">
          Select all matching
        </button>
        <button type="button" onClick={clearMatching}
          className="rounded border border-line px-2 py-1 text-xs text-muted hover:bg-raised">
          Clear matching
        </button>
        <span className="ml-auto self-center text-xs text-faint">
          {matching.length} matching · {selected.length} selected
        </span>
      </div>

      <ul className="grid max-h-64 gap-1 overflow-y-auto rounded border border-line p-1">
        {matching.length === 0 && (
          <li className="px-2 py-1.5 text-sm text-faint">No targets match.</li>
        )}
        {matching.map((t) => {
          const id = `target-picker-${t.id}`;
          return (
            <li key={t.id}>
              <label htmlFor={id} className="flex cursor-pointer items-center gap-2 rounded px-2 py-1 text-sm hover:bg-raised">
                <Checkbox
                  id={id} checked={selectedSet.has(t.id)}
                  onCheckedChange={() => toggleTarget(t.id)}
                />
                <span className="flex-1 text-fg">{t.name}</span>
                <span className="rounded bg-raised px-1.5 py-0.5 font-mono text-xs uppercase text-muted">{t.engine}</span>
                <span className="text-xs text-faint">{t.queue_name}</span>
              </label>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
