import { Fragment, useState } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { ScheduleForm } from '../features/schedules/ScheduleForm';
import { useLivePanel } from '../features/live/LiveRunProvider';
import type { Schedule, ScheduleInput, ScheduleRun, ScheduleSaved } from '../lib/api';
import { ApiError } from '../lib/api';
import { formatDateTime, formatRelative } from '../lib/format';
import {
  useCreateSchedule, useDeleteSchedule, useRunSchedule, useSchedules,
  useTargets, useUpdateSchedule,
} from '../lib/queries';

type Editing = { mode: 'none' } | { mode: 'new' } | { mode: 'edit'; schedule: Schedule };

/** LastRun shows the most recent run of one schedule, from the batched
 * `last_run` field on the schedule row rather than a per-row fetch. */
function LastRun({ last }: { last: ScheduleRun | null }) {
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
  const [expanded, setExpanded] = useState<Set<number>>(new Set());
  const [confirmDelete, setConfirmDelete] = useState<Schedule | null>(null);
  const targetByID = new Map((targets.data ?? []).map((t) => [t.id, t]));

  const toggleExpanded = (id: number) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  };

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
          <Button type="button" onClick={() => { setWarnings([]); setEditing({ mode: 'new' }); }}>
            New schedule
          </Button>
        )}
      </header>

      {warnings.length > 0 && (
        <div role="status" className="flex items-start justify-between gap-3 rounded border border-warn/60 bg-warn/10 px-3 py-2 text-sm text-warn">
          <div>{warnings.map((wmsg) => <p key={wmsg}>{wmsg}</p>)}</div>
          <button type="button" aria-label="Dismiss warnings" className="text-warn hover:opacity-80"
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
        <div className="overflow-x-auto rounded-md border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Cron</TableHead>
                <TableHead>Next run</TableHead>
                <TableHead>Last run</TableHead>
                <TableHead>Targets</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {schedules.data.map((s) => (
                <Fragment key={s.id}>
                  <TableRow>
                    <TableCell className="text-fg">
                      {s.name}
                      {!s.enabled && <span className="ml-2 text-xs text-faint">disabled</span>}
                    </TableCell>
                    <TableCell className="font-mono text-xs text-muted">{s.cron}</TableCell>
                    <TableCell className="text-muted">
                      {s.next_run ? formatDateTime(s.next_run) : '—'}
                      <span className="ml-2 text-xs text-faint">{s.timezone}</span>
                    </TableCell>
                    <TableCell><LastRun last={s.last_run} /></TableCell>
                    <TableCell>
                      <button type="button" className="text-muted underline decoration-dotted hover:text-fg"
                        onClick={() => toggleExpanded(s.id)}>
                        {s.target_ids.length}
                      </button>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex flex-wrap justify-end gap-2">
                        <Button
                          type="button" variant="outline" size="sm"
                          disabled={run.isPending && run.variables === s.id}
                          onClick={() => run.mutate(s.id, { onSuccess: (res) => open(res.run_id) })}>
                          Run now
                        </Button>
                        <Button type="button" variant="outline" size="sm"
                          onClick={() => { setWarnings([]); setEditing({ mode: 'edit', schedule: s }); }}>
                          Edit
                        </Button>
                        <Button type="button" variant="outline" size="sm" className="text-bad hover:bg-bad/10"
                          onClick={() => setConfirmDelete(s)}>
                          Delete
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                  {expanded.has(s.id) && (
                    <TableRow className="bg-raised/50 hover:bg-raised/50">
                      <TableCell colSpan={6}>
                        <div className="flex flex-wrap gap-1.5">
                          {s.target_ids.length === 0 && <span className="text-xs text-faint">No targets.</span>}
                          {s.target_ids.map((id) => (
                            <Badge key={id} variant="outline" className="rounded-full font-normal">
                              {targetByID.get(id)?.name ?? `#${id}`}
                            </Badge>
                          ))}
                        </div>
                      </TableCell>
                    </TableRow>
                  )}
                </Fragment>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <ConfirmDialog
        open={confirmDelete !== null}
        onOpenChange={(v) => { if (!v) setConfirmDelete(null); }}
        title={confirmDelete ? `Delete schedule "${confirmDelete.name}"?` : ''}
        confirmLabel="Delete"
        onConfirm={() => { if (confirmDelete) remove.mutate(confirmDelete.id); }}
      />
    </section>
  );
}
