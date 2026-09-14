import { useEffect, useMemo, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { inputClass } from '@/features/settings/styles';
import { FormField } from '../../components/FormField';
import { SwitchField } from '../../components/SwitchField';
import { TimezoneSelect } from '../../components/TimezoneSelect';
import type { Schedule, ScheduleInput, Target } from '../../lib/api';
import { ApiError } from '../../lib/api';
import { useCronPreview } from '../../lib/queries';
import { formatDateTime } from '../../lib/format';
import { SortableTargetList } from './SortableTargetList';
import { TargetPicker } from './TargetPicker';

/** Presets cover the schedules people actually create; anything else is
 * typed straight into the expression field. */
const PRESETS = [
  { label: 'Every 15 min', expr: '*/15 * * * *' },
  { label: 'Hourly', expr: '0 * * * *' },
  { label: 'Daily 03:00', expr: '0 3 * * *' },
] as const;

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
    <Card>
      <CardHeader>
        <CardTitle>{initial ? 'Edit schedule' : 'New schedule'}</CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={submit} className="grid gap-4">
          <FormField id="schedule-name" label="Name">
            <input
              id="schedule-name" value={name} onChange={(e) => setName(e.target.value)}
              className={inputClass}
            />
          </FormField>

          <div className="grid gap-2">
            <div className="flex flex-wrap gap-2">
              {PRESETS.map((p) => (
                <button
                  key={p.expr} type="button" onClick={() => setCron(p.expr)}
                  className={`rounded-md border px-2 py-1 text-xs ${cron === p.expr ? 'border-accent text-accent' : 'border-line-strong text-muted hover:bg-raised'}`}
                >
                  {p.label}
                </button>
              ))}
            </div>
            <FormField id="schedule-cron" label="Cron expression">
              <input
                id="schedule-cron" value={cron} onChange={(e) => setCron(e.target.value)}
                className={cn(inputClass, 'font-mono')}
              />
            </FormField>
            <p data-testid="cron-preview" className="text-xs text-muted">
              {previewError
                ? <span className="text-bad">{previewError}</span>
                : (preview.data ?? []).map((t) => formatDateTime(t)).join(' · ') || 'Next runs appear here.'}
            </p>
          </div>

          <FormField id="schedule-tz" label="Timezone">
            <TimezoneSelect
              id="schedule-tz" value={timezone} onChange={setTimezone}
              className={inputClass}
            />
          </FormField>

          <fieldset className="grid gap-2">
            <legend className="text-xs uppercase tracking-wide text-faint">Targets, in run order</legend>
            <SortableTargetList selected={selected} byID={byID} onChange={setSelected} />
            <TargetPicker targets={targets} selected={selected} onChange={setSelected} />
          </fieldset>

          <SwitchField id="schedule-enabled" label="Enabled" checked={enabled} onCheckedChange={setEnabled} />

          {(localError || error) && <p className="text-sm text-bad">{localError || error}</p>}

          <div className="flex gap-2">
            <Button type="submit" disabled={submitting}>
              Save schedule
            </Button>
            <Button type="button" variant="outline" onClick={onCancel}>
              Cancel
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}
