import { useState } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { TargetForm } from '../features/targets/TargetForm';
import { HistoryDialog } from '../features/targets/HistoryDialog';
import { useLivePanel } from '../features/live/LiveRunProvider';
import type { Target, TargetInput } from '../lib/api';
import { ApiError } from '../lib/api';
import {
  useCreateTarget, useDeletedTargets, useDeleteTarget, useRestoreTarget, useRunTarget,
  useSchedules, useTargets, useUpdateTarget,
} from '../lib/queries';

/** RecentlyDeleted lists targets whose most recent history entry is a
 * delete, with a Restore action. Stays collapsed (no expand affordance)
 * while the list is empty. */
function RecentlyDeleted() {
  const deleted = useDeletedTargets();
  const restore = useRestoreTarget();
  const [expanded, setExpanded] = useState(false);
  const count = deleted.data?.length ?? 0;

  return (
    <section className="grid gap-2">
      <button
        type="button"
        className="flex items-center gap-1 text-left text-sm font-medium text-muted disabled:opacity-60"
        onClick={() => setExpanded((e) => !e)}
        disabled={count === 0}
      >
        <span>{expanded ? '▾' : '▸'}</span>
        Recently deleted{count > 0 ? ` (${count})` : ''}
      </button>

      {expanded && count > 0 && (
        <div className="overflow-x-auto rounded-md border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Engine</TableHead>
                <TableHead>Lane</TableHead>
                <TableHead>Deleted</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {deleted.data!.map((d) => (
                <TableRow key={d.id}>
                  <TableCell className="text-fg">{d.name}</TableCell>
                  <TableCell className="text-muted">{d.engine}</TableCell>
                  <TableCell className="text-muted">{d.lane}</TableCell>
                  <TableCell className="text-muted">{new Date(d.deleted_at).toLocaleString()}</TableCell>
                  <TableCell className="text-right">
                    <Button
                      type="button" variant="outline" size="sm"
                      disabled={restore.isPending}
                      onClick={() => restore.mutate(d.id)}
                    >
                      {restore.isPending ? 'Restoring…' : 'Restore'}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </section>
  );
}

type Editing = { mode: 'none' } | { mode: 'new' } | { mode: 'edit'; target: Target };

/** Targets lists every target and hosts the create/edit form. */
export function Targets() {
  const targets = useTargets();
  const schedules = useSchedules();
  const create = useCreateTarget();
  const update = useUpdateTarget();
  const remove = useDeleteTarget();
  const run = useRunTarget();
  const { open } = useLivePanel();
  const [editing, setEditing] = useState<Editing>({ mode: 'none' });
  const [notice, setNotice] = useState('');
  const [confirmDelete, setConfirmDelete] = useState<Target | null>(null);
  // Reverse map: target id -> names of schedules that include it, built
  // from the schedules' own target_ids rather than a per-target fetch.
  const schedulesByTarget = new Map<number, string[]>();
  for (const s of schedules.data ?? []) {
    for (const id of s.target_ids) {
      const names = schedulesByTarget.get(id) ?? [];
      names.push(s.name);
      schedulesByTarget.set(id, names);
    }
  }
  // run.variables is the target id of whichever "Run now" mutation is
  // currently in flight, so each row's button can show pending state
  // independently instead of every row disabling at once.
  const runningTargetID = run.isPending ? run.variables : undefined;

  const mutationError = (err: unknown) =>
    err instanceof ApiError ? err.message : err ? String(err) : undefined;

  const submit = (input: TargetInput) => {
    if (editing.mode === 'edit') {
      update.mutate({ id: editing.target.id, target: input }, {
        onSuccess: () => setEditing({ mode: 'none' }),
      });
      return;
    }
    create.mutate(input, { onSuccess: () => setEditing({ mode: 'none' }) });
  };

  return (
    <section className="grid gap-4">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-semibold tracking-tight">Targets</h1>
        {editing.mode === 'none' && (
          <Button type="button" onClick={() => setEditing({ mode: 'new' })}>New target</Button>
        )}
      </header>

      {notice && <p className="text-sm text-accent">{notice}</p>}

      {editing.mode !== 'none' && (
        <TargetForm
          initial={editing.mode === 'edit' ? editing.target : undefined}
          onSubmit={submit}
          onCancel={() => setEditing({ mode: 'none' })}
          submitting={create.isPending || update.isPending}
          error={mutationError(create.error ?? update.error)}
        />
      )}

      {targets.isLoading && <p className="text-sm text-muted">Loading targets…</p>}
      {targets.isError && (
        <p className="text-sm text-bad">{mutationError(targets.error)}</p>
      )}

      {targets.data && targets.data.length === 0 && (
        <p className="rounded border border-dashed border-line p-6 text-center text-sm text-muted">
          No targets yet. Create one to start measuring.
        </p>
      )}

      {targets.data && targets.data.length > 0 && (
        <div className="overflow-x-auto rounded-md border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Engine</TableHead>
                <TableHead>Lane</TableHead>
                <TableHead>State</TableHead>
                <TableHead>Schedules</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {targets.data.map((t) => (
                <TableRow key={t.id}>
                  <TableCell className="text-fg">{t.name}</TableCell>
                  <TableCell>
                    <Badge variant="outline" className="font-mono uppercase">{t.engine}</Badge>
                  </TableCell>
                  <TableCell className="text-muted">{t.lane}</TableCell>
                  <TableCell>
                    <span className={t.enabled ? 'text-ok' : 'text-faint'}>
                      {t.enabled ? 'enabled' : 'disabled'}
                    </span>
                  </TableCell>
                  <TableCell className="text-muted">
                    {(schedulesByTarget.get(t.id) ?? []).join(', ') || '—'}
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex flex-wrap justify-end gap-2">
                      <Button
                        type="button" variant="outline" size="sm"
                        disabled={runningTargetID === t.id}
                        onClick={() =>
                          run.mutate(t.id, {
                            onSuccess: (res) => { setNotice(`Queued run #${res.run_id} for ${t.name}`); open(); },
                          })
                        }
                      >
                        {runningTargetID === t.id ? 'Running…' : 'Run now'}
                      </Button>
                      <Button type="button" variant="outline" size="sm" onClick={() => setEditing({ mode: 'edit', target: t })}>
                        Edit
                      </Button>
                      <HistoryDialog target={t} />
                      <Button type="button" variant="outline" size="sm" className="text-bad hover:bg-bad/10"
                        onClick={() => setConfirmDelete(t)}>
                        Delete
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <ConfirmDialog
        open={confirmDelete !== null}
        onOpenChange={(v) => { if (!v) setConfirmDelete(null); }}
        title={confirmDelete ? `Delete target "${confirmDelete.name}"?` : ''}
        description="This cannot be undone, but the target's history stays in Recently deleted for restore."
        confirmLabel="Delete"
        onConfirm={() => { if (confirmDelete) remove.mutate(confirmDelete.id); }}
      />

      <RecentlyDeleted />
    </section>
  );
}
