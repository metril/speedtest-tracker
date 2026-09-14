import { useState } from 'react';

interface Props {
  value: Record<string, string>;
  onChange: (v: Record<string, string>) => void;
  label: string;
}

/** LabelsEditor edits a flat string map (VM extra labels, VL stream
 * fields) as a list of key/value rows plus a draft row to add another. */
export function LabelsEditor({ value, onChange, label }: Props) {
  const [draftKey, setDraftKey] = useState('');
  const [draftValue, setDraftValue] = useState('');
  const entries = Object.entries(value);

  const renameKey = (oldKey: string, newKey: string) => {
    const next: Record<string, string> = {};
    for (const [k, v] of entries) next[k === oldKey ? newKey : k] = v;
    onChange(next);
  };
  const setValue = (key: string, newValue: string) => onChange({ ...value, [key]: newValue });
  const remove = (key: string) => {
    const next = { ...value };
    delete next[key];
    onChange(next);
  };
  const add = () => {
    if (!draftKey) return;
    onChange({ ...value, [draftKey]: draftValue });
    setDraftKey('');
    setDraftValue('');
  };

  return (
    <div className="grid gap-2">
      <span className="text-sm text-muted">{label}</span>
      {entries.map(([key, val]) => (
        <div key={key} className="flex gap-2 items-center">
          <input
            aria-label={`${label} key`}
            className="rounded border border-line bg-surface px-2 py-1 text-fg"
            value={key}
            onChange={(e) => renameKey(key, e.target.value)}
          />
          <input
            aria-label={`${label} value for ${key}`}
            className="rounded border border-line bg-surface px-2 py-1 text-fg"
            value={val}
            onChange={(e) => setValue(key, e.target.value)}
          />
          <button type="button" className="text-muted hover:text-bad" onClick={() => remove(key)}>
            Remove {key}
          </button>
        </div>
      ))}
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
