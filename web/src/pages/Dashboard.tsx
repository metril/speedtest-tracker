import { useState } from 'react';
import { Link } from 'react-router';
import { RangePicker } from '../features/dashboard/RangePicker';
import { SummaryTiles } from '../features/dashboard/SummaryTiles';
import { TargetCard } from '../features/dashboard/TargetCard';
import { useLivePanel } from '../features/live/LiveRunProvider';
import type { Range, TargetSummary } from '../lib/api';
import {
  useRunTarget, useSummary, useTargetHistory,
} from '../lib/queries';

/** DashboardCard fetches its own 24h download history for the sparkline,
 * keyed to the dashboard's selected range so it shares the cache entry
 * with any chart drilldown for the same target (Task 11). */
function DashboardCard({
  summary, range, onRun, running,
}: {
  summary: TargetSummary; range: Range; onRun: (id: number) => void; running: boolean;
}) {
  const history = useTargetHistory(summary.target_id, range);
  const spark = (history.data?.points ?? []).map((p) => p.avg_download_bps);
  return <TargetCard summary={summary} spark={spark} onRun={onRun} running={running} />;
}

/** Dashboard is the app's home page: a range selector, headline stats and
 * a latest-per-target card grid. Charts and the outage strip land in
 * Task 11 below the card grid. */
export function Dashboard() {
  const [range, setRange] = useState<Range>('24h');
  const summary = useSummary(range);
  const run = useRunTarget();
  const { open } = useLivePanel();
  const runningTargetID = run.isPending ? run.variables : undefined;

  const handleRun = (id: number) => {
    run.mutate(id, { onSuccess: () => open() });
  };

  return (
    <section className="grid gap-4">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-semibold tracking-tight">Dashboard</h1>
        <RangePicker value={range} onChange={setRange} />
      </header>

      {summary.isLoading && <p className="text-sm text-muted">Loading…</p>}

      {summary.isError && (
        <div className="rounded border border-line bg-surface px-4 py-3 text-sm text-bad">
          <p>Failed to load dashboard data.</p>
          <button
            type="button"
            onClick={() => summary.refetch()}
            className="mt-2 rounded border border-line px-2 py-1 text-xs text-fg hover:bg-raised"
          >
            Retry
          </button>
        </div>
      )}

      {summary.data && (
        <>
          <SummaryTiles stats={summary.data} />

          {summary.data.targets.length === 0 ? (
            <div className="rounded border border-line bg-surface px-4 py-6 text-center text-sm text-muted">
              No targets configured yet.{' '}
              <Link to="/targets" className="text-accent hover:underline">Add a target</Link>
              {' '}to start collecting results.
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {summary.data.targets.map((t) => (
                <DashboardCard
                  key={t.target_id}
                  summary={t}
                  range={range}
                  onRun={handleRun}
                  running={runningTargetID === t.target_id}
                />
              ))}
            </div>
          )}
        </>
      )}

      {/* Charts and outage strip: Task 11 */}
    </section>
  );
}
