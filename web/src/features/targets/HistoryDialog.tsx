import { useState } from 'react';
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger,
} from '../../components/ui/dialog';
import { Badge } from '../../components/ui/badge';
import type { RevisionAction, Target } from '../../lib/api';
import { useRevertTargetRevision, useTargetRevisions } from '../../lib/queries';

const ACTION_LABEL: Record<RevisionAction, string> = {
  create: 'Created', update: 'Updated', delete: 'Deleted', revert: 'Reverted', restore: 'Restored',
};

const ACTION_BADGE_CLASS: Record<RevisionAction, string> = {
  create: 'border-transparent bg-ok/20 text-ok',
  update: 'border-transparent bg-accent/20 text-accent',
  delete: 'border-transparent bg-bad/20 text-bad',
  revert: 'border-line text-muted',
  restore: 'border-line text-muted',
};

interface Props {
  target: Target;
}

/** HistoryDialog lists a target's revision history and lets the user
 * revert to an earlier version (with an inline confirm step). */
export function HistoryDialog({ target }: Props) {
  const [open, setOpen] = useState(false);
  const [confirmVersion, setConfirmVersion] = useState<number | null>(null);
  const revisions = useTargetRevisions(target.id, open);
  const revert = useRevertTargetRevision();

  const handleOpenChange = (next: boolean) => {
    setOpen(next);
    if (!next) setConfirmVersion(null);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>
        <button className="rounded border border-line px-2 py-1 text-xs text-muted hover:bg-raised">
          History
        </button>
      </DialogTrigger>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>History — {target.name}</DialogTitle>
        </DialogHeader>

        {revisions.isLoading && <p className="text-sm text-muted">Loading history…</p>}
        {revisions.isError && <p className="text-sm text-bad">Failed to load history.</p>}

        {revisions.data && revisions.data.length > 0 && (
          <ul className="grid max-h-96 gap-2 overflow-auto">
            {revisions.data.map((rev, i) => {
              const isCurrent = i === 0;
              return (
                <li
                  key={rev.version}
                  className="flex items-center justify-between gap-3 rounded border border-line p-2 text-sm"
                >
                  <div className="grid gap-1">
                    <div className="flex items-center gap-2">
                      <span className="font-medium text-fg">v{rev.version}</span>
                      <Badge className={ACTION_BADGE_CLASS[rev.action]}>{ACTION_LABEL[rev.action]}</Badge>
                      {isCurrent && <Badge variant="outline">Current</Badge>}
                    </div>
                    <span className="text-xs text-muted">
                      {new Date(rev.created_at).toLocaleString()}
                      {rev.changed.length > 0 && ` · changed: ${rev.changed.join(', ')}`}
                    </span>
                  </div>
                  {!isCurrent && (
                    confirmVersion === rev.version ? (
                      <div className="flex shrink-0 gap-2">
                        <button
                          className="rounded border border-bad px-2 py-1 text-xs text-bad hover:bg-bad/20 disabled:opacity-50"
                          disabled={revert.isPending}
                          onClick={() => revert.mutate(
                            { id: target.id, version: rev.version },
                            { onSuccess: () => setConfirmVersion(null) },
                          )}
                        >
                          {revert.isPending ? 'Reverting…' : 'Confirm revert'}
                        </button>
                        <button
                          className="rounded border border-line px-2 py-1 text-xs text-muted hover:bg-raised"
                          onClick={() => setConfirmVersion(null)}
                        >
                          Cancel
                        </button>
                      </div>
                    ) : (
                      <button
                        className="shrink-0 rounded border border-line px-2 py-1 text-xs text-muted hover:bg-raised"
                        onClick={() => setConfirmVersion(rev.version)}
                      >
                        Revert
                      </button>
                    )
                  )}
                </li>
              );
            })}
          </ul>
        )}
        {revert.isError && <p className="text-sm text-bad">Revert failed.</p>}
      </DialogContent>
    </Dialog>
  );
}
