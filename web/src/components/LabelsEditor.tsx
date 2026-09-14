import { useEffect, useRef, useState } from 'react';

interface Props {
  value: Record<string, string>;
  onChange: (v: Record<string, string>) => void;
  label: string;
}

interface Row {
  id: string;
  key: string;
  value: string;
}

let rowSeq = 0;
const nextRowId = () => `row-${(rowSeq += 1)}`;

function rowsFromValue(value: Record<string, string>): Row[] {
  return Object.entries(value).map(([key, val]) => ({ id: nextRowId(), key, value: val }));
}

function sameEntries(a: Record<string, string>, b: Record<string, string>): boolean {
  const ak = Object.keys(a);
  const bk = Object.keys(b);
  if (ak.length !== bk.length) return false;
  return ak.every((k) => a[k] === b[k]);
}

/** LabelsEditor edits a flat string map (VM extra labels, VL stream
 * fields) as a list of key/value rows plus a draft row to add another.
 *
 * Rows are tracked as an ordered list with stable synthetic ids, not
 * keyed by the (editable) label key itself: renaming a key would
 * otherwise remount the row on every keystroke and steal focus. Empty
 * keys and keys that collide with an earlier row are flagged inline and
 * excluded from the map handed to `onChange` (the first occurrence
 * wins) until the user fixes them. */
export function LabelsEditor({ value, onChange, label }: Props) {
  const [rows, setRows] = useState<Row[]>(() => rowsFromValue(value));
  const lastEmitted = useRef(value);

  useEffect(() => {
    if (!sameEntries(value, lastEmitted.current)) {
      setRows(rowsFromValue(value));
      lastEmitted.current = value;
    }
  }, [value]);

  const [draftKey, setDraftKey] = useState('');
  const [draftValue, setDraftValue] = useState('');

  const keyCounts = rows.reduce<Record<string, number>>((acc, r) => {
    const k = r.key.trim();
    if (k) acc[k] = (acc[k] ?? 0) + 1;
    return acc;
  }, {});

  const emit = (next: Row[]) => {
    setRows(next);
    const map: Record<string, string> = {};
    const seen = new Set<string>();
    for (const r of next) {
      const key = r.key.trim();
      if (!key || seen.has(key)) continue;
      seen.add(key);
      map[key] = r.value;
    }
    lastEmitted.current = map;
    onChange(map);
  };

  const updateKey = (id: string, key: string) => emit(rows.map((r) => (r.id === id ? { ...r, key } : r)));
  const updateValue = (id: string, val: string) => emit(rows.map((r) => (r.id === id ? { ...r, value: val } : r)));
  const remove = (id: string) => emit(rows.filter((r) => r.id !== id));
  const add = () => {
    if (!draftKey) return;
    emit([...rows, { id: nextRowId(), key: draftKey, value: draftValue }]);
    setDraftKey('');
    setDraftValue('');
  };

  return (
    <div className="grid gap-2">
      <span className="text-sm text-muted">{label}</span>
      {rows.map((row) => {
        const trimmed = row.key.trim();
        const empty = trimmed === '';
        const duplicate = !empty && keyCounts[trimmed] > 1;
        const invalid = empty || duplicate;
        return (
          <div key={row.id} className="grid gap-1">
            <div className="flex gap-2 items-center">
              <input
                aria-label={`${label} key`}
                aria-invalid={invalid}
                className="rounded border border-line bg-surface px-2 py-1 text-fg"
                value={row.key}
                onChange={(e) => updateKey(row.id, e.target.value)}
              />
              <input
                aria-label={`${label} value for ${row.key}`}
                className="rounded border border-line bg-surface px-2 py-1 text-fg"
                value={row.value}
                onChange={(e) => updateValue(row.id, e.target.value)}
              />
              <button type="button" className="text-muted hover:text-bad" onClick={() => remove(row.id)}>
                Remove {row.key}
              </button>
            </div>
            {invalid && (
              <p className="text-sm text-bad">
                {empty ? 'Key is required.' : 'Duplicate key; only the first is saved.'}
              </p>
            )}
          </div>
        );
      })}
      <div className="flex gap-2 items-center">
        <input
          aria-label={`New ${label} key`}
          placeholder="key"
          className="rounded border border-line bg-surface px-2 py-1 text-fg"
          value={draftKey}
          onChange={(e) => setDraftKey(e.target.value)}
        />
        <input
          aria-label={`New ${label} value`}
          placeholder="value"
          className="rounded border border-line bg-surface px-2 py-1 text-fg"
          value={draftValue}
          onChange={(e) => setDraftValue(e.target.value)}
        />
        <button type="button" className="text-muted hover:text-fg" onClick={add}>
          Add label
        </button>
      </div>
    </div>
  );
}
