import { useQueries } from '@tanstack/react-query';
import { useState } from 'react';
import { Link } from 'react-router';
import { HistoryChart } from '../features/dashboard/HistoryChart';
import { OutageStrip } from '../features/dashboard/OutageStrip';
import { RangePicker } from '../features/dashboard/RangePicker';
import { SummaryTiles } from '../features/dashboard/SummaryTiles';
import { TargetCard } from '../features/dashboard/TargetCard';
import { useLivePanel } from '../features/live/LiveRunProvider';
import * as api from '../lib/api';
import type { HistoryPoint, Range, TargetSummary } from '../lib/api';
import { SERIES } from '../lib/chart';
import { formatBps, formatMs } from '../lib/format';
import {
  queryKeys, useOutages, useRunTarget, useSummary, useTargetHistory,
} from '../lib/queries';

const COLOR_CYCLE = [SERIES.download, SERIES.upload, SERIES.ping, SERIES.jitter];

/** mergeByBucket combines several targets' histories into one row per
 * bucket_start, keyed per target (`<key>_<id>`), so multiple targets'
 * series can share one chart's x-axis. */
function mergeByBucket(
  histories: Map<number, HistoryPoint[]>,
  targetIds: number[],
  keys: (keyof HistoryPoint)[],
): Record<string, number | string>[] {
  const byBucket = new Map<string, Record<string, number | string>>();
  for (const id of targetIds) {
    for (const p of histories.get(id) ?? []) {
      const row = byBucket.get(p.bucket_start) ?? { bucket_start: p.bucket_start };
      for (const key of keys) row[`${key}_${id}`] = Number(p[key]);
      byBucket.set(p.bucket_start, row);
    }
  }
  return [...byBucket.values()].sort((a, b) =>
    String(a.bucket_start).localeCompare(String(b.bucket_start)));
}

/** DashboardCard fetches its own 24h download history for the sparkline,
 * keyed to the dashboard's selected range so it shares the cache entry
 * with the merged charts below for the same target. */
function DashboardCard({
  summary, range, onRun, running,
}: {
  summary: TargetSummary; range: Range; onRun: (id: number) => void; running: boolean;
}) {
  const history = useTargetHistory(summary.target_id, range);
  const spark = (history.data?.points ?? []).map((p) => p.avg_download_bps);
  return <TargetCard summary={summary} spark={spark} onRun={onRun} running={running} />;
}

/** HistorySection renders the per-target series toggle, the merged
 * throughput and latency charts, and the outage strip for the selected
 * range. Split out so its history queries only run once targets exist. */
function HistorySection({ targets, range }: { targets: TargetSummary[]; range: Range }) {
  const [visible, setVisible] = useState<Set<number>>(
    () => new Set(targets.map((t) => t.target_id)),
  );
  const outages = useOutages(range);

  const historyQueries = useQueries({
    queries: targets.map((t) => ({
      queryKey: queryKeys.history(t.target_id, range),
      queryFn: () => api.targetHistory(t.target_id, range),
      staleTime: 60_000,
    })),
  });

  const histories = new Map<number, HistoryPoint[]>();
  targets.forEach((t, i) => histories.set(t.target_id, historyQueries[i].data?.points ?? []));

  const visibleTargets = targets.filter((t) => visible.has(t.target_id));
  const visibleIds = visibleTargets.map((t) => t.target_id);

  const toggle = (id: number) => {
    setVisible((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  };

  const throughputPoints = mergeByBucket(histories, visibleIds, ['avg_download_bps', 'avg_upload_bps']);
  const latencyPoints = mergeByBucket(histories, visibleIds, ['avg_ping_ms', 'avg_jitter_ms']);

  const throughputSeries = visibleTargets.flatMap((t, i) => [
    {
      key: `avg_download_bps_${t.target_id}` as keyof HistoryPoint,
      label: `${t.target_name} download`,
      color: COLOR_CYCLE[(i * 2) % COLOR_CYCLE.length],
      unit: formatBps,
    },
    {
      key: `avg_upload_bps_${t.target_id}` as keyof HistoryPoint,
      label: `${t.target_name} upload`,
      color: COLOR_CYCLE[(i * 2 + 1) % COLOR_CYCLE.length],
      unit: formatBps,
    },
  ]);

  const latencySeries = visibleTargets.flatMap((t, i) => [
    {
      key: `avg_ping_ms_${t.target_id}` as keyof HistoryPoint,
      label: `${t.target_name} ping`,
      color: COLOR_CYCLE[(i * 2) % COLOR_CYCLE.length],
      unit: formatMs,
    },
    {
      key: `avg_jitter_ms_${t.target_id}` as keyof HistoryPoint,
      label: `${t.target_name} jitter`,
      color: COLOR_CYCLE[(i * 2 + 1) % COLOR_CYCLE.length],
      unit: formatMs,
    },
  ]);

  return (
    <div className="grid gap-4">
      {targets.length > 1 && (
        <fieldset className="flex flex-wrap items-center gap-3 rounded border border-line bg-surface p-3">
          <legend className="px-1 text-xs font-medium text-muted">Targets</legend>
          {targets.map((t) => (
            <label key={t.target_id} className="flex items-center gap-1.5 text-sm text-fg">
              <input
                type="checkbox"
                checked={visible.has(t.target_id)}
                onChange={() => toggle(t.target_id)}
              />
              {t.target_name}
            </label>
          ))}
        </fieldset>
      )}

      <HistoryChart title="Download & upload" points={throughputPoints as unknown as HistoryPoint[]} series={throughputSeries} />
      <HistoryChart title="Ping & jitter" points={latencyPoints as unknown as HistoryPoint[]} series={latencySeries} />

      {outages.data && (
        <OutageStrip incidents={outages.data.incidents} from={outages.data.from} to={outages.data.to} />
      )}
    </div>
  );
}

/** Dashboard is the app's home page: a range selector, headline stats, a
 * latest-per-target card grid, merged history charts and the outage
 * timeline strip. */
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
            <>
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

              <HistorySection targets={summary.data.targets} range={range} />
            </>
          )}
        </>
      )}
    </section>
  );
}
