import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router';
import { Button } from '@/components/ui/button';
import { ResultFiltersBar } from '../features/results/ResultFilters';
import { ResultsTable } from '../features/results/ResultsTable';
import { TagsPanel } from '../features/results/TagsPanel';
import { useLiveControls } from '../features/live/LiveRunProvider';
import { resultsCsvUrl } from '../lib/api';
import type { ResultFilters } from '../lib/api';
import { ApiError } from '../lib/api';
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
  const { open } = useLiveControls();

  const rows = results.data?.pages.flatMap((p) => p.results) ?? [];

  // ?result_id= (the live panel's "View result" link) highlights that row.
  // Keep it in state so it survives clearing the param from the URL.
  const [searchParams, setSearchParams] = useSearchParams();
  const resultIdParam = Number(searchParams.get('result_id'));
  const [highlightId, setHighlightId] = useState<number | undefined>();
  useEffect(() => {
    if (!searchParams.has('result_id')) return;
    if (Number.isInteger(resultIdParam) && resultIdParam > 0) setHighlightId(resultIdParam);
    const next = new URLSearchParams(searchParams);
    next.delete('result_id');
    setSearchParams(next, { replace: true });
  }, [searchParams, resultIdParam, setSearchParams]);
  const highlightShown = highlightId !== undefined && rows.some((r) => r.id === highlightId);
  useEffect(() => {
    if (!highlightShown) return undefined;
    const timer = setTimeout(() => setHighlightId(undefined), 5000);
    return () => clearTimeout(timer);
  }, [highlightShown]);

  const errors = [remove.error, replay.error, tag.error]
    .filter(Boolean)
    .map((err) => (err instanceof ApiError ? err.message : String(err)));

  return (
    <section className="grid gap-4">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-semibold tracking-tight">Results</h1>
        <div className="flex items-center gap-3">
          <span className="text-sm text-faint">{rows.length} loaded</span>
          <Button asChild variant="outline" size="sm">
            <a href={resultsCsvUrl(filters)} download>Export CSV</a>
          </Button>
        </div>
      </header>

      <ResultFiltersBar value={filters} onChange={setFilters} targets={targets.data ?? []} />

      <details className="rounded border border-line p-3">
        <summary className="cursor-pointer text-sm text-muted">Manage tags</summary>
        <div className="mt-3">
          <TagsPanel />
        </div>
      </details>

      {results.isLoading && <p className="text-sm text-muted">Loading results…</p>}
      {results.isError && <p className="text-sm text-bad">Could not load results.</p>}
      {errors.map((msg) => <p key={msg} role="alert" className="text-sm text-bad">{msg}</p>)}
      {rows.length === 0 && !results.isLoading && (
        <p className="rounded border border-dashed border-line p-6 text-center text-sm text-muted">
          No results match these filters.
        </p>
      )}

      {rows.length > 0 && (
        <ResultsTable
          rows={rows}
          highlightId={highlightId}
          onDelete={(id) => remove.mutate(id)}
          onReexecute={(id) => replay.mutate(id, { onSuccess: (res) => open(res.run_id) })}
          onTag={(id, tags) => tag.mutate({ id, tags })}
        />
      )}

      {results.hasNextPage && (
        <Button
          type="button" variant="outline" size="sm" className="mx-auto"
          disabled={results.isFetchingNextPage}
          onClick={() => results.fetchNextPage()}
        >
          {results.isFetchingNextPage ? 'Loading…' : 'Load more'}
        </Button>
      )}
    </section>
  );
}
