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
  if (!last) return <span className="text-slate-600">never run</span>;
  const tone = last.status === 'done' ? 'text-emerald-400'
    : last.status === 'skipped' ? 'text-amber-400'
      : last.status === 'running' || last.status === 'queued' ? 'text-sky-400' : 'text-rose-400';
  return (
    <span className={tone}>
      {last.status}
      {last.started_at && <span className="text-slate-500"> · {formatRelative(last.started_at)}</span>}
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
          <button className="rounded bg-sky-500 px-3 py-1.5 text-sm font-medium text-slate-950 hover:bg-sky-400"
            onClick={() => { setWarnings([]); setEditing({ mode: 'new' }); }}>
            New schedule
          </button>
        )}
      </header>

      {warnings.length > 0 && (
        <div role="status" className="flex items-start justify-between gap-3 rounded border border-amber-800/60 bg-amber-950/30 px-3 py-2 text-sm text-amber-300">
          <div>{warnings.map((wmsg) => <p key={wmsg}>{wmsg}</p>)}</div>
          <button type="button" aria-label="Dismiss warnings" className="text-amber-400 hover:text-amber-200"
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

      {schedules.isLoading && <p className="text-sm text-slate-400">Loading schedules…</p>}
      {schedules.data?.length === 0 && (
        <p className="rounded border border-dashed border-slate-800 p-6 text-center text-sm text-slate-400">
          No schedules yet. Create one to run targets automatically.
        </p>
      )}

      {schedules.data && schedules.data.length > 0 && (
        <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="border-b border-slate-800 text-left text-xs uppercase tracking-wide text-slate-500">
              <th className="py-2 pr-3 font-medium">Name</th>
              <th className="py-2 pr-3 font-medium">Cron</th>
              <th className="py-2 pr-3 font-medium">Next run</th>
              <th className="py-2 pr-3 font-medium">Last run</th>
              <th className="py-2 text-right font-medium">Actions</th>
            </tr>
          </thead>
          <tbody>
            {schedules.data.map((s) => (
              <tr key={s.id} className="border-b border-slate-900 hover:bg-slate-900/50">
                <td className="py-2 pr-3 text-slate-100">
                  {s.name}
                  {!s.enabled && <span className="ml-2 text-xs text-slate-500">disabled</span>}
                </td>
                <td className="py-2 pr-3 font-mono text-xs text-slate-400">{s.cron}</td>
                <td className="py-2 pr-3 text-slate-400">
                  {s.next_run ? formatDateTime(s.next_run) : '—'}
                  <span className="ml-2 text-xs text-slate-600">{s.timezone}</span>
                </td>
                <td className="py-2 pr-3"><LastRun scheduleId={s.id} /></td>
                <td className="py-2 text-right">
                  <div className="flex justify-end gap-2">
                    <button
                      className="rounded border border-sky-700 px-2 py-1 text-xs text-sky-300 hover:bg-sky-900/40 disabled:opacity-50"
                      disabled={run.isPending && run.variables === s.id}
                      onClick={() => run.mutate(s.id, { onSuccess: () => open() })}>
                      Run now
                    </button>
                    <button className="rounded border border-slate-700 px-2 py-1 text-xs text-slate-300 hover:bg-slate-800"
                      onClick={() => { setWarnings([]); setEditing({ mode: 'edit', schedule: s }); }}>
                      Edit
                    </button>
                    <button className="rounded border border-slate-800 px-2 py-1 text-xs text-rose-300 hover:bg-rose-950/40"
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
