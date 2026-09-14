import { useState } from 'react';
import { TargetForm } from '../features/targets/TargetForm';
import { useLivePanel } from '../features/live/LiveRunProvider';
import type { Target, TargetInput } from '../lib/api';
import { ApiError } from '../lib/api';
import {
  useCreateTarget, useDeleteTarget, useRunTarget, useTargets, useUpdateTarget,
} from '../lib/queries';

type Editing = { mode: 'none' } | { mode: 'new' } | { mode: 'edit'; target: Target };

/** Targets lists every target and hosts the create/edit form. */
export function Targets() {
  const targets = useTargets();
  const create = useCreateTarget();
  const update = useUpdateTarget();
  const remove = useDeleteTarget();
  const run = useRunTarget();
  const { open } = useLivePanel();
  const [editing, setEditing] = useState<Editing>({ mode: 'none' });
  const [notice, setNotice] = useState('');
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
          <button
            className="rounded bg-accent px-3 py-1.5 text-sm font-medium text-accent-fg hover:opacity-90"
            onClick={() => setEditing({ mode: 'new' })}
          >
            New target
          </button>
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
        <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="border-b border-line text-left text-xs uppercase tracking-wide text-faint">
              <th className="py-2 pr-3 font-medium">Name</th>
              <th className="py-2 pr-3 font-medium">Engine</th>
              <th className="py-2 pr-3 font-medium">Lane</th>
              <th className="py-2 pr-3 font-medium">State</th>
              <th className="py-2 text-right font-medium">Actions</th>
            </tr>
          </thead>
          <tbody>
            {targets.data.map((t) => (
              <tr key={t.id} className="border-b border-line hover:bg-raised">
                <td className="py-2 pr-3 text-fg">{t.name}</td>
                <td className="py-2 pr-3">
                  <span className="rounded bg-raised px-1.5 py-0.5 font-mono text-xs uppercase text-muted">
                    {t.engine}
                  </span>
                </td>
                <td className="py-2 pr-3 text-muted">{t.lane}</td>
                <td className="py-2 pr-3">
                  <span className={t.enabled ? 'text-ok' : 'text-faint'}>
                    {t.enabled ? 'enabled' : 'disabled'}
                  </span>
                </td>
                <td className="py-2 text-right">
                  <div className="flex justify-end gap-2">
                    <button
                      className="rounded border border-accent px-2 py-1 text-xs text-accent hover:bg-accent/20 disabled:opacity-50"
                      disabled={runningTargetID === t.id}
                      onClick={() =>
                        run.mutate(t.id, {
                          onSuccess: (res) => { setNotice(`Queued run #${res.run_id} for ${t.name}`); open(); },
                        })
                      }
                    >
                      {runningTargetID === t.id ? 'Running…' : 'Run now'}
                    </button>
                    <button
                      className="rounded border border-line px-2 py-1 text-xs text-muted hover:bg-raised"
                      onClick={() => setEditing({ mode: 'edit', target: t })}
                    >
                      Edit
                    </button>
                    <button
                      className="rounded border border-bad px-2 py-1 text-xs text-bad hover:bg-bad/20"
                      onClick={() => {
                        if (window.confirm(`Delete target "${t.name}"? This cannot be undone.`)) {
                          remove.mutate(t.id);
                        }
                      }}
                    >
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
