import { useState } from 'react';
import { ResultFiltersBar } from '../features/results/ResultFilters';
import { ResultsTable } from '../features/results/ResultsTable';
import { useLivePanel } from '../features/live/LiveRunProvider';
import type { ResultFilters } from '../lib/api';
import {
  useDeleteResult, useReexecute, useResults, useSetTags, useTargets,
} from '../lib/queries';

/** Results is the filterable, virtualised history table. */
export function Results() {
  const [filters, setFilters] = useState<ResultFilters>({});
  const targets = useTargets();
  const results = useResults(filters);
  const remove = useDeleteResult();
  const replay = useReexecute();
  const tag = useSetTags();
  const { open } = useLivePanel();

  const rows = results.data?.pages.flatMap((p) => p.results) ?? [];

  return (
    <section className="grid gap-4">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-semibold tracking-tight">Results</h1>
        <span className="text-sm text-slate-500">{rows.length} loaded</span>
      </header>

      <ResultFiltersBar value={filters} onChange={setFilters} targets={targets.data ?? []} />

      {results.isLoading && <p className="text-sm text-slate-400">Loading results…</p>}
      {results.isError && <p className="text-sm text-rose-400">Could not load results.</p>}
      {rows.length === 0 && !results.isLoading && (
        <p className="rounded border border-dashed border-slate-800 p-6 text-center text-sm text-slate-400">
          No results match these filters.
        </p>
      )}

      {rows.length > 0 && (
        <ResultsTable
          rows={rows}
          onDelete={(id) => remove.mutate(id)}
          onReexecute={(id) => replay.mutate(id, { onSuccess: () => open() })}
          onTag={(id, tags) => tag.mutate({ id, tags })}
        />
      )}

      {results.hasNextPage && (
        <button
          className="mx-auto rounded border border-slate-700 px-4 py-1.5 text-sm text-slate-300 hover:bg-slate-800 disabled:opacity-50"
          disabled={results.isFetchingNextPage}
          onClick={() => results.fetchNextPage()}
        >
          {results.isFetchingNextPage ? 'Loading…' : 'Load more'}
        </button>
      )}
    </section>
  );
}
