import { useState } from 'react';
import { TargetForm } from '../features/targets/TargetForm';
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
  const [editing, setEditing] = useState<Editing>({ mode: 'none' });
  const [notice, setNotice] = useState('');

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
            className="rounded bg-sky-500 px-3 py-1.5 text-sm font-medium text-slate-950 hover:bg-sky-400"
            onClick={() => setEditing({ mode: 'new' })}
          >
            New target
          </button>
        )}
      </header>

      {notice && <p className="text-sm text-sky-300">{notice}</p>}

      {editing.mode !== 'none' && (
        <TargetForm
          initial={editing.mode === 'edit' ? editing.target : undefined}
          onSubmit={submit}
          onCancel={() => setEditing({ mode: 'none' })}
          submitting={create.isPending || update.isPending}
          error={mutationError(create.error ?? update.error)}
        />
      )}

      {targets.isLoading && <p className="text-sm text-slate-400">Loading targets…</p>}
      {targets.isError && (
        <p className="text-sm text-rose-400">{mutationError(targets.error)}</p>
      )}

      {targets.data && targets.data.length === 0 && (
        <p className="rounded border border-dashed border-slate-800 p-6 text-center text-sm text-slate-400">
          No targets yet. Create one to start measuring.
        </p>
      )}

      {targets.data && targets.data.length > 0 && (
        <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="border-b border-slate-800 text-left text-xs uppercase tracking-wide text-slate-500">
              <th className="py-2 pr-3 font-medium">Name</th>
              <th className="py-2 pr-3 font-medium">Engine</th>
              <th className="py-2 pr-3 font-medium">Lane</th>
              <th className="py-2 pr-3 font-medium">State</th>
              <th className="py-2 text-right font-medium">Actions</th>
            </tr>
          </thead>
          <tbody>
            {targets.data.map((t) => (
              <tr key={t.id} className="border-b border-slate-900 hover:bg-slate-900/50">
                <td className="py-2 pr-3 text-slate-100">{t.name}</td>
                <td className="py-2 pr-3">
                  <span className="rounded bg-slate-800 px-1.5 py-0.5 font-mono text-xs uppercase text-slate-300">
                    {t.engine}
                  </span>
                </td>
                <td className="py-2 pr-3 text-slate-400">{t.lane}</td>
                <td className="py-2 pr-3">
                  <span className={t.enabled ? 'text-emerald-400' : 'text-slate-500'}>
                    {t.enabled ? 'enabled' : 'disabled'}
                  </span>
                </td>
                <td className="py-2 text-right">
                  <div className="flex justify-end gap-2">
                    <button
                      className="rounded border border-sky-700 px-2 py-1 text-xs text-sky-300 hover:bg-sky-900/40 disabled:opacity-50"
                      disabled={run.isPending}
                      onClick={() =>
                        run.mutate(t.id, {
                          onSuccess: (res) => setNotice(`Queued run #${res.run_id} for ${t.name}`),
                        })
                      }
                    >
                      Run now
                    </button>
                    <button
                      className="rounded border border-slate-700 px-2 py-1 text-xs text-slate-300 hover:bg-slate-800"
                      onClick={() => setEditing({ mode: 'edit', target: t })}
                    >
                      Edit
                    </button>
                    <button
                      className="rounded border border-rose-800 px-2 py-1 text-xs text-rose-300 hover:bg-rose-950/40"
                      onClick={() => remove.mutate(t.id)}
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
