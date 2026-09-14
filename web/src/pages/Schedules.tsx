import { useState } from 'react';
import { ScheduleForm } from '../features/schedules/ScheduleForm';
import { useLivePanel } from '../features/live/LiveRunProvider';
import type { Schedule, ScheduleInput, ScheduleSaved } from '../lib/api';
import { ApiError } from '../lib/api';
import { formatDateTime, formatRelative } from '../lib/format';
import {
  useCreateSchedule, useDeleteSchedule, useRunSchedule, useScheduleRuns, useSchedules,
  useTargets, useUpdateSchedule,
} from '../lib/queries';

type Editing = { mode: 'none' } | { mode: 'new' } | { mode: 'edit'; schedule: Schedule };

/** LastRun shows the most recent run of one schedule. */
function LastRun({ scheduleId }: { scheduleId: number }) {
  const runs = useScheduleRuns(scheduleId);
  const last = runs.data?.runs[0];
  if (!last) return <span className="text-faint">never run</span>;
  const tone = last.status === 'done' ? 'text-ok'
    : last.status === 'skipped' ? 'text-warn'
      : last.status === 'running' || last.status === 'queued' ? 'text-accent' : 'text-bad';
  return (
    <span className={tone}>
      {last.status}
      {last.started_at && <span className="text-faint"> · {formatRelative(last.started_at)}</span>}
    </span>
  );
}

export function Schedules() {
  const schedules = useSchedules();
  const targets = useTargets();
  const create = useCreateSchedule();
  const update = useUpdateSchedule();
  const remove = useDeleteSchedule();
  const run = useRunSchedule();
  const { open } = useLivePanel();
  const [editing, setEditing] = useState<Editing>({ mode: 'none' });
  const [warnings, setWarnings] = useState<string[]>([]);

  const message = (err: unknown) => (err instanceof ApiError ? err.message : err ? String(err) : undefined);

  const submit = (input: ScheduleInput) => {
    const onSuccess = (saved: ScheduleSaved) => {
      setWarnings(saved.warnings);
      setEditing({ mode: 'none' });
    };
    if (editing.mode === 'edit') {
      update.mutate({ id: editing.schedule.id, schedule: input }, { onSuccess });
      return;
    }
    create.mutate(input, { onSuccess });
  };

  return (
    <section className="grid gap-4">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-semibold tracking-tight">Schedules</h1>
        {editing.mode === 'none' && (
          <button className="rounded bg-accent px-3 py-1.5 text-sm font-medium text-accent-fg hover:opacity-90"
            onClick={() => { setWarnings([]); setEditing({ mode: 'new' }); }}>
            New schedule
          </button>
        )}
      </header>

      {warnings.length > 0 && (
        <div role="status" className="flex items-start justify-between gap-3 rounded border border-warn/60 bg-warn/10 px-3 py-2 text-sm text-warn">
          <div>{warnings.map((wmsg) => <p key={wmsg}>{wmsg}</p>)}</div>
          <button type="button" aria-label="Dismiss warnings" className="text-warn hover:text-warn"
            onClick={() => setWarnings([])}>
            ×
          </button>
        </div>
      )}

      {editing.mode !== 'none' && (
        <ScheduleForm
          key={editing.mode === 'edit' ? `edit-${editing.schedule.id}` : 'new'}
          initial={editing.mode === 'edit' ? editing.schedule : undefined}
          targets={targets.data ?? []}
          onSubmit={submit}
          onCancel={() => { setWarnings([]); setEditing({ mode: 'none' }); }}
          submitting={create.isPending || update.isPending}
          error={message(create.error ?? update.error)}
        />
      )}

      {schedules.isLoading && <p className="text-sm text-muted">Loading schedules…</p>}
      {schedules.data?.length === 0 && (
        <p className="rounded border border-dashed border-line p-6 text-center text-sm text-muted">
          No schedules yet. Create one to run targets automatically.
        </p>
      )}

      {schedules.data && schedules.data.length > 0 && (
        <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="border-b border-line text-left text-xs uppercase tracking-wide text-faint">
              <th className="py-2 pr-3 font-medium">Name</th>
              <th className="py-2 pr-3 font-medium">Cron</th>
              <th className="py-2 pr-3 font-medium">Next run</th>
              <th className="py-2 pr-3 font-medium">Last run</th>
              <th className="py-2 text-right font-medium">Actions</th>
            </tr>
          </thead>
          <tbody>
            {schedules.data.map((s) => (
              <tr key={s.id} className="border-b border-line hover:hover:bg-raised">
                <td className="py-2 pr-3 text-fg">
                  {s.name}
                  {!s.enabled && <span className="ml-2 text-xs text-faint">disabled</span>}
                </td>
                <td className="py-2 pr-3 font-mono text-xs text-muted">{s.cron}</td>
                <td className="py-2 pr-3 text-muted">
                  {s.next_run ? formatDateTime(s.next_run) : '—'}
                  <span className="ml-2 text-xs text-faint">{s.timezone}</span>
                </td>
                <td className="py-2 pr-3"><LastRun scheduleId={s.id} /></td>
                <td className="py-2 text-right">
                  <div className="flex justify-end gap-2">
                    <button
                      className="rounded border border-accent px-2 py-1 text-xs text-accent hover:bg-accent/20 disabled:opacity-50"
                      disabled={run.isPending && run.variables === s.id}
                      onClick={() => run.mutate(s.id, { onSuccess: () => open() })}>
                      Run now
                    </button>
                    <button className="rounded border border-line px-2 py-1 text-xs text-muted hover:bg-raised"
                      onClick={() => { setWarnings([]); setEditing({ mode: 'edit', schedule: s }); }}>
                      Edit
                    </button>
                    <button className="rounded border border-line px-2 py-1 text-xs text-bad hover:bg-bad/20"
                      onClick={() => { if (window.confirm(`Delete schedule "${s.name}"?`)) remove.mutate(s.id); }}>
                      Delete
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
