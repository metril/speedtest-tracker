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
        <span className="text-sm text-faint">{rows.length} loaded</span>
      </header>

      <ResultFiltersBar value={filters} onChange={setFilters} targets={targets.data ?? []} />

      {results.isLoading && <p className="text-sm text-muted">Loading results…</p>}
      {results.isError && <p className="text-sm text-bad">Could not load results.</p>}
      {rows.length === 0 && !results.isLoading && (
        <p className="rounded border border-dashed border-line p-6 text-center text-sm text-muted">
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
          className="mx-auto rounded border border-line px-4 py-1.5 text-sm text-muted hover:bg-raised disabled:opacity-50"
          disabled={results.isFetchingNextPage}
          onClick={() => results.fetchNextPage()}
        >
          {results.isFetchingNextPage ? 'Loading…' : 'Load more'}
        </button>
      )}
    </section>
  );
}
