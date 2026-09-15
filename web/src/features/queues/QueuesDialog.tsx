import { useState } from 'react';
import { Button } from '@/components/ui/button';
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
} from '@/components/ui/dialog';
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table';
import { inputClass } from '@/lib/styles';
import { ConfirmDialog } from '../../components/ConfirmDialog';
import { ApiError, type Queue } from '../../lib/api';
import { useCreateQueue, useDeleteQueue, useQueues, useRenameQueue } from '../../lib/queries';

const mutationError = (err: unknown) =>
  (err instanceof ApiError ? err.message : err ? String(err) : undefined);

/** QueueRow renders one queue as either static text or, once "Rename" is
 * clicked, an inline text input with Save/Cancel -- the same
 * click-to-edit pattern the rest of the app uses for short single-field
 * edits, without a separate dialog. */
function QueueRow({ queue, onDelete }: { queue: Queue; onDelete: () => void }) {
  const rename = useRenameQueue();
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(queue.name);

  const save = () => {
    const trimmed = name.trim();
    if (trimmed === '' || trimmed === queue.name) {
      setEditing(false);
      setName(queue.name);
      return;
    }
    rename.mutate({ id: queue.id, queue: { name: trimmed } }, {
      onSuccess: () => setEditing(false),
    });
  };

  return (
    <TableRow>
      <TableCell className="text-fg">
        {editing ? (
          <form
            className="flex items-center gap-2"
            onSubmit={(e) => { e.preventDefault(); save(); }}
          >
            <input
              autoFocus
              className={inputClass}
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={64}
              aria-label={`Rename queue ${queue.name}`}
            />
            <Button type="submit" size="sm" disabled={rename.isPending}>Save</Button>
            <Button
              type="button" size="sm" variant="outline"
              onClick={() => { setEditing(false); setName(queue.name); }}
            >
              Cancel
            </Button>
          </form>
        ) : queue.name}
        {editing && rename.isError && rename.variables?.id === queue.id && (
          <p className="mt-1 text-xs text-bad">{mutationError(rename.error)}</p>
        )}
      </TableCell>
      <TableCell className="text-right">
        {!editing && (
          <div className="flex flex-wrap justify-end gap-2">
            <Button type="button" variant="outline" size="sm" onClick={() => setEditing(true)}>
              Rename
            </Button>
            <Button
              type="button" variant="outline" size="sm" className="text-bad hover:bg-bad/10"
              onClick={onDelete}
            >
              Delete
            </Button>
          </div>
        )}
      </TableCell>
    </TableRow>
  );
}

/** QueuesDialog lists every queue and hosts the create form, opened from
 * the Targets page (queues are managed alongside the targets that use
 * them, rather than on their own page). Targets in the same queue run
 * one at a time; different queues run in parallel. */
export function QueuesDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const queues = useQueues(open);
  const create = useCreateQueue();
  const remove = useDeleteQueue();
  const [newName, setNewName] = useState('');
  const [confirmDelete, setConfirmDelete] = useState<Queue | null>(null);
  // deleteError survives the dialog auto-closing on confirm: a failed
  // delete (e.g. queue_in_use) reopens it showing the reason instead of
  // just vanishing the click into a page-level error.
  const [deleteError, setDeleteError] = useState<string | undefined>();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Queues</DialogTitle>
        </DialogHeader>
        <p className="text-sm text-muted">
          Targets in the same queue run one at a time; different queues run in parallel.
        </p>

        <form
          className="flex flex-wrap items-start gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            const trimmed = newName.trim();
            if (trimmed === '') return;
            create.mutate({ name: trimmed }, { onSuccess: () => setNewName('') });
          }}
        >
          <input
            className={inputClass}
            placeholder="Queue name"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            maxLength={64}
            aria-label="New queue name"
          />
          <Button type="submit" disabled={create.isPending}>Add queue</Button>
          {create.isError && <p className="text-sm text-bad">{mutationError(create.error)}</p>}
        </form>

        {queues.isLoading && <p className="text-sm text-muted">Loading queues…</p>}
        {queues.isError && <p className="text-sm text-bad">{mutationError(queues.error)}</p>}

        {queues.data && queues.data.length > 0 && (
          <div className="overflow-x-auto rounded-md border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {queues.data.map((q) => (
                  <QueueRow key={q.id} queue={q} onDelete={() => setConfirmDelete(q)} />
                ))}
              </TableBody>
            </Table>
          </div>
        )}

        <ConfirmDialog
          open={confirmDelete !== null}
          onOpenChange={(v) => { if (!v) { setConfirmDelete(null); setDeleteError(undefined); } }}
          title={confirmDelete ? `Delete queue "${confirmDelete.name}"?` : ''}
          description={deleteError ?? 'Targets still assigned to this queue must be moved first.'}
          confirmLabel="Delete"
          onConfirm={() => {
            if (!confirmDelete) return;
            const target = confirmDelete;
            setDeleteError(undefined);
            remove.mutate(target.id, {
              onError: (err) => { setDeleteError(mutationError(err)); setConfirmDelete(target); },
            });
          }}
        />
      </DialogContent>
    </Dialog>
  );
}
