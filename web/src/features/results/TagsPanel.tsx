import { useState } from 'react';
import { ApiError } from '../../lib/api';
import { useDeleteTag, useRenameTag, useTags } from '../../lib/queries';

/** TagsPanel is a small management panel for the result tags: rename inline
 * or delete (with confirmation). */
export function TagsPanel() {
  const tags = useTags();
  const rename = useRenameTag();
  const remove = useDeleteTag();
  const [editingId, setEditingId] = useState<number | null>(null);
  const [draft, setDraft] = useState('');

  const message = (err: unknown) => (err instanceof ApiError ? err.message : err ? String(err) : undefined);
  const error = message(rename.error ?? remove.error);

  const startEdit = (id: number, name: string) => {
    setEditingId(id);
    setDraft(name);
  };

  const save = (id: number) => {
    rename.mutate({ id, name: draft }, { onSuccess: () => setEditingId(null) });
  };

  return (
    <section className="grid gap-2">
      {error && <p className="text-sm text-bad">{error}</p>}
      {tags.data?.length === 0 && (
        <p className="text-sm text-muted">No tags yet. Tag a result to create one.</p>
      )}
      <div className="flex flex-wrap gap-2">
        {(tags.data ?? []).map((t) => (
          editingId === t.id ? (
            <span key={t.id} className="flex items-center gap-1 rounded border border-line px-2 py-1">
              <label className="sr-only" htmlFor={`tag-name-${t.id}`}>Tag name</label>
              <input
                id={`tag-name-${t.id}`}
                aria-label="Tag name"
                className="w-28 rounded border border-line bg-surface px-1 py-0.5 text-sm text-fg focus:border-accent focus:outline-none"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
              />
              <button
                type="button"
                className="rounded border border-line px-1.5 py-0.5 text-xs text-muted hover:bg-raised"
                onClick={() => save(t.id)}
              >
                Save
              </button>
              <button
                type="button"
                className="rounded border border-line px-1.5 py-0.5 text-xs text-muted hover:bg-raised"
                onClick={() => setEditingId(null)}
              >
                Cancel
              </button>
            </span>
          ) : (
            <span key={t.id} className="flex items-center gap-1 rounded border border-line px-2 py-1 text-sm text-fg">
              {t.name}
              <button
                type="button"
                aria-label={`Rename tag ${t.name}`}
                className="text-muted hover:text-fg"
                onClick={() => startEdit(t.id, t.name)}
              >
                ✎
              </button>
              <button
                type="button"
                aria-label={`Delete tag ${t.name}`}
                className="text-muted hover:text-bad"
                onClick={() => {
                  if (window.confirm(`Delete tag "${t.name}"?`)) remove.mutate(t.id);
                }}
              >
                ×
              </button>
            </span>
          )
        ))}
      </div>
    </section>
  );
}
