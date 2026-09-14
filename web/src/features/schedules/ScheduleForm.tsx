import { useEffect, useMemo, useState } from 'react';
import type { Schedule, ScheduleInput, Target } from '../../lib/api';
import { ApiError } from '../../lib/api';
import { useCronPreview } from '../../lib/queries';
import { formatDateTime } from '../../lib/format';

/** Presets cover the schedules people actually create; anything else is
 * typed straight into the expression field. */
const PRESETS = [
  { label: 'Every 15 min', expr: '*/15 * * * *' },
  { label: 'Hourly', expr: '0 * * * *' },
  { label: 'Daily 03:00', expr: '0 3 * * *' },
] as const;

/** A curated shortlist of common zones, shown before the full IANA list so
 * the dropdown isn't 400 entries deep by default. */
const CURATED_TIMEZONES = [
  'UTC', 'America/New_York', 'America/Chicago', 'America/Denver', 'America/Los_Angeles',
  'Europe/London', 'Europe/Berlin', 'Europe/Zurich', 'Asia/Kolkata', 'Asia/Singapore',
  'Asia/Tokyo', 'Australia/Sydney',
];

/** How long to wait after the last keystroke before asking the server to
 * validate the cron expression. */
const PREVIEW_DEBOUNCE_MS = 300;

interface Props {
  initial?: Schedule;
  targets: Target[];
  onSubmit: (input: ScheduleInput) => void;
  onCancel: () => void;
  submitting: boolean;
  error?: string;
}

export function ScheduleForm({ initial, targets, onSubmit, onCancel, submitting, error }: Props) {
  const [name, setName] = useState(initial?.name ?? '');
  const [cron, setCron] = useState(initial?.cron ?? '0 * * * *');
  const [enabled, setEnabled] = useState(initial?.enabled ?? true);
  const [timezone, setTimezone] = useState(
    initial?.timezone ?? Intl.DateTimeFormat().resolvedOptions().timeZone ?? 'UTC');
  const [selected, setSelected] = useState<number[]>(initial?.target_ids ?? []);
  const [localError, setLocalError] = useState('');

  // Debounce the cron text before it drives the preview query, so typing
  // an expression character-by-character doesn't fire a request per key.
  const [debouncedCron, setDebouncedCron] = useState(cron);
  useEffect(() => {
    const t = setTimeout(() => setDebouncedCron(cron), PREVIEW_DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [cron]);

  // Same debounce for the timezone, since the custom free-text field fires
  // a state update per keystroke too.
  const [debouncedTimezone, setDebouncedTimezone] = useState(timezone);
  useEffect(() => {
    const t = setTimeout(() => setDebouncedTimezone(timezone), PREVIEW_DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [timezone]);

  const preview = useCronPreview(debouncedCron, debouncedTimezone);
  const byID = useMemo(() => new Map(targets.map((t) => [t.id, t])), [targets]);
  const isCustomTimezone = timezone !== '' && !CURATED_TIMEZONES.includes(timezone);
  const available = targets.filter((t) => !selected.includes(t.id));

  const move = (index: number, delta: number) => {
    setSelected((prev) => {
      const next = [...prev];
      const to = index + delta;
      if (to < 0 || to >= next.length) return prev;
      [next[index], next[to]] = [next[to], next[index]];
      return next;
    });
  };

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      setLocalError('Name is required.');
      return;
    }
    if (selected.length === 0) {
      setLocalError('Pick at least one target.');
      return;
    }
    setLocalError('');
    onSubmit({ name: name.trim(), cron: cron.trim(), enabled, timezone, target_ids: selected });
  };

  const previewError = preview.error instanceof ApiError ? preview.error.message : undefined;

  return (
    <form onSubmit={submit} className="grid gap-4 rounded border border-slate-800 bg-slate-900/40 p-4">
      <div className="grid gap-1">
        <label htmlFor="schedule-name" className="text-xs uppercase tracking-wide text-slate-500">Name</label>
        <input
          id="schedule-name" value={name} onChange={(e) => setName(e.target.value)}
          className="rounded border border-slate-800 bg-slate-950 px-2 py-1.5 text-sm text-slate-100"
        />
      </div>

      <div className="grid gap-2">
        <div className="flex flex-wrap gap-2">
          {PRESETS.map((p) => (
            <button
              key={p.expr} type="button" onClick={() => setCron(p.expr)}
              className={`rounded border px-2 py-1 text-xs ${cron === p.expr ? 'border-sky-600 text-sky-300' : 'border-slate-800 text-slate-400 hover:bg-slate-900'}`}
            >
              {p.label}
            </button>
          ))}
        </div>
        <label htmlFor="schedule-cron" className="text-xs uppercase tracking-wide text-slate-500">Cron expression</label>
        <input
          id="schedule-cron" value={cron} onChange={(e) => setCron(e.target.value)}
          className="rounded border border-slate-800 bg-slate-950 px-2 py-1.5 font-mono text-sm text-slate-100"
        />
        <p data-testid="cron-preview" className="text-xs text-slate-400">
          {previewError
            ? <span className="text-rose-400">{previewError}</span>
            : (preview.data ?? []).map((t) => formatDateTime(t)).join(' · ') || 'Next runs appear here.'}
        </p>
      </div>

      <div className="grid gap-1">
        <label htmlFor="schedule-tz" className="text-xs uppercase tracking-wide text-slate-500">Timezone</label>
        <select
          id="schedule-tz" value={isCustomTimezone ? '__custom__' : timezone}
          onChange={(e) => setTimezone(e.target.value)}
          className="rounded border border-slate-800 bg-slate-950 px-2 py-1.5 text-sm text-slate-100"
        >
          {CURATED_TIMEZONES.map((tz) => <option key={tz} value={tz}>{tz}</option>)}
          {isCustomTimezone && <option value="__custom__" disabled>Custom…</option>}
        </select>
        <input
          aria-label="Custom timezone"
          placeholder="Or type an IANA zone, e.g. Europe/Zurich"
          value={isCustomTimezone ? timezone : ''}
          onChange={(e) => setTimezone(e.target.value)}
          className="rounded border border-slate-800 bg-slate-950 px-2 py-1.5 text-xs text-slate-300"
        />
      </div>

      <fieldset className="grid gap-2">
        <legend className="text-xs uppercase tracking-wide text-slate-500">Targets, in run order</legend>
        <ol className="grid gap-1">
          {selected.map((id, i) => (
            <li key={id} className="flex items-center gap-2 rounded border border-slate-800 px-2 py-1 text-sm">
              <span className="w-5 text-right font-mono text-xs text-slate-500">{i + 1}</span>
              <span className="flex-1 text-slate-200">{byID.get(id)?.name ?? `#${id}`}</span>
              <button type="button" aria-label={`Move ${byID.get(id)?.name ?? id} up`} disabled={i === 0}
                className="rounded border border-slate-800 px-1.5 text-xs text-slate-400 disabled:opacity-40"
                onClick={() => move(i, -1)}>↑</button>
              <button type="button" aria-label={`Move ${byID.get(id)?.name ?? id} down`} disabled={i === selected.length - 1}
                className="rounded border border-slate-800 px-1.5 text-xs text-slate-400 disabled:opacity-40"
                onClick={() => move(i, 1)}>↓</button>
              <button type="button" aria-label={`Remove ${byID.get(id)?.name ?? id}`}
                className="rounded border border-slate-800 px-1.5 text-xs text-slate-400"
                onClick={() => setSelected((prev) => prev.filter((x) => x !== id))}>×</button>
            </li>
          ))}
        </ol>
        <div className="flex flex-wrap gap-2">
          {available.map((t) => (
            <button key={t.id} type="button" aria-label={`Add ${t.name}`}
              className="rounded border border-slate-800 px-2 py-1 text-xs text-slate-300 hover:bg-slate-900"
              onClick={() => setSelected((prev) => [...prev, t.id])}>
              + {t.name}
            </button>
          ))}
        </div>
      </fieldset>

      <label className="flex items-center gap-2 text-sm text-slate-300">
        <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
        Enabled
      </label>

      {(localError || error) && <p className="text-sm text-rose-400">{localError || error}</p>}

      <div className="flex gap-2">
        <button type="submit" disabled={submitting}
          className="rounded bg-sky-500 px-3 py-1.5 text-sm font-medium text-slate-950 hover:bg-sky-400 disabled:opacity-50">
          Save schedule
        </button>
        <button type="button" onClick={onCancel}
          className="rounded border border-slate-700 px-3 py-1.5 text-sm text-slate-300 hover:bg-slate-900">
          Cancel
        </button>
      </div>
    </form>
  );
}
