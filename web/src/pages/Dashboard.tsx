import { useQueries } from '@tanstack/react-query';
import { useState } from 'react';
import { Link } from 'react-router';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { SwitchField } from '../components/SwitchField';
import { HistoryChart, type Series } from '../features/dashboard/HistoryChart';
import { OutageStrip } from '../features/dashboard/OutageStrip';
import { RangePicker } from '../features/dashboard/RangePicker';
import { SummaryTiles, type DashboardSpark } from '../features/dashboard/SummaryTiles';
import { TargetCard } from '../features/dashboard/TargetCard';
import { useLivePanel } from '../features/live/LiveRunProvider';
import * as api from '../lib/api';
import type { History, HistoryPoint, Range, TargetSummary, ThresholdSet } from '../lib/api';
import { SERIES } from '../lib/chart';
import { formatBps, formatMs } from '../lib/format';
import {
  queryKeys, useOutages, usePreviousSummary, useRunTarget, useSettings, useSummary, useTargetHistory, useTargets,
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

/** useAllTargetHistories fetches every target's history for `range` and
 * shares its cache entries (by query key) with whoever else asks for the
 * same target+range -- the per-card sparkline query and HistorySection's
 * merged charts both read through the same underlying fetches. Pass
 * `offset: 1` to fetch the immediately-preceding window instead (used for
 * the "compare with previous period" overlay); that variant is gated by
 * `enabled` so it only fires once the toggle is on. */
function useAllTargetHistories(
  targets: TargetSummary[], range: Range, opts?: { offset?: 0 | 1; enabled?: boolean },
) {
  const offset = opts?.offset;
  const enabled = opts?.enabled ?? true;
  return useQueries({
    queries: targets.map((t) => ({
      queryKey: queryKeys.history(t.target_id, range, offset),
      queryFn: () => api.targetHistory(t.target_id, range, offset),
      staleTime: 60_000,
      enabled,
    })),
  });
}

/** slotOf places a bucket at its position within its own history's window:
 * round((bucket_start - from) / bucket_seconds). The previous window is
 * fetched via `offset: 1` (see useAllTargetHistories), which the backend
 * shifts back by exactly the range's own span, so a current bucket and
 * the previous bucket at the same slot cover the same position within
 * their respective (equal-length) windows -- "24h ago at 3pm" lines up
 * with "today at 3pm" regardless of gaps or how many buckets either
 * window actually has data for. */
function slotOf(bucketStart: string, from: string, bucketSeconds: number): number {
  return Math.round((Date.parse(bucketStart) - Date.parse(from)) / (bucketSeconds * 1000));
}

/** mergePrevByOffset overlays a previous-period history onto rows already
 * merged onto the current window's x-axis (mergeByBucket), joining each
 * target's current and previous points by slot (see slotOf) rather than
 * position in the array. That means a gap in either window's data is
 * just a missing slot -- it does not shift any later point's alignment,
 * and targets with different point counts each align independently. */
export function mergePrevByOffset(
  rows: Record<string, number | string>[],
  currentHistories: Map<number, History>,
  prevHistories: Map<number, History>,
  targetIds: number[],
  keys: (keyof HistoryPoint)[],
): Record<string, number | string>[] {
  const prevBySlot = new Map<number, Map<number, HistoryPoint>>();
  for (const id of targetIds) {
    const h = prevHistories.get(id);
    if (!h) continue;
    const bySlot = new Map<number, HistoryPoint>();
    for (const p of h.points) bySlot.set(slotOf(p.bucket_start, h.from, h.bucket_seconds), p);
    prevBySlot.set(id, bySlot);
  }
  return rows.map((row) => {
    const next = { ...row };
    for (const id of targetIds) {
      const hasCurrent = keys.some((key) => `${key}_${id}` in row);
      if (!hasCurrent) continue;
      const h = currentHistories.get(id);
      if (!h) continue;
      const slot = slotOf(String(row.bucket_start), h.from, h.bucket_seconds);
      const p = prevBySlot.get(id)?.get(slot);
      if (!p) continue;
      for (const key of keys) next[`${key}_${id}_prev`] = Number(p[key]);
    }
    return next;
  });
}

/** aggregateSpark averages each bucket's metric across every target with
 * data in it, for the KPI row's range sparklines. It is intentionally
 * simple (an unweighted per-bucket mean): a headline trend line, not a
 * precise recomputation of the weighted stat above it. */
function aggregateSpark(targets: TargetSummary[], queries: ReturnType<typeof useAllTargetHistories>): DashboardSpark {
  const byBucket = new Map<string, { download: number[]; upload: number[]; ping: number[] }>();
  targets.forEach((_t, i) => {
    for (const p of queries[i].data?.points ?? []) {
      const row = byBucket.get(p.bucket_start) ?? { download: [], upload: [], ping: [] };
      row.download.push(p.avg_download_bps);
      row.upload.push(p.avg_upload_bps);
      row.ping.push(p.avg_ping_ms);
      byBucket.set(p.bucket_start, row);
    }
  });
  const buckets = [...byBucket.keys()].sort();
  const avg = (xs: number[]) => (xs.length ? xs.reduce((a, b) => a + b, 0) / xs.length : 0);
  return {
    download: buckets.map((b) => avg(byBucket.get(b)!.download)),
    upload: buckets.map((b) => avg(byBucket.get(b)!.upload)),
    ping: buckets.map((b) => avg(byBucket.get(b)!.ping)),
  };
}

/** HistorySection renders the per-target series toggle, the merged
 * throughput and latency charts (as Tabs in one Card), and the outage
 * timeline in its own Card. Split out so its history queries only run
 * once targets exist. */
function HistorySection({ targets, range }: { targets: TargetSummary[]; range: Range }) {
  const [visible, setVisible] = useState<Set<number>>(
    () => new Set(targets.map((t) => t.target_id)),
  );
  const outages = useOutages(range);
  const historyQueries = useAllTargetHistories(targets, range);
  const [compare, setCompare] = useState(false);
  const prevHistoryQueries = useAllTargetHistories(targets, range, { offset: 1, enabled: compare });

  const histories = new Map<number, HistoryPoint[]>();
  targets.forEach((t, i) => histories.set(t.target_id, historyQueries[i].data?.points ?? []));
  const historyByTarget = new Map<number, History>();
  targets.forEach((t, i) => {
    const h = historyQueries[i].data;
    if (h) historyByTarget.set(t.target_id, h);
  });
  const prevHistoryByTarget = new Map<number, History>();
  targets.forEach((t, i) => {
    const h = prevHistoryQueries[i].data;
    if (h) prevHistoryByTarget.set(t.target_id, h);
  });

  const visibleTargets = targets.filter((t) => visible.has(t.target_id));
  const visibleIds = visibleTargets.map((t) => t.target_id);

  const toggle = (id: number) => {
    setVisible((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  };

  let throughputPoints = mergeByBucket(histories, visibleIds, ['avg_download_bps', 'avg_upload_bps']);
  let latencyPoints = mergeByBucket(histories, visibleIds, ['avg_ping_ms', 'avg_jitter_ms']);
  if (compare) {
    throughputPoints = mergePrevByOffset(
      throughputPoints, historyByTarget, prevHistoryByTarget, visibleIds, ['avg_download_bps', 'avg_upload_bps'],
    );
    latencyPoints = mergePrevByOffset(
      latencyPoints, historyByTarget, prevHistoryByTarget, visibleIds, ['avg_ping_ms', 'avg_jitter_ms'],
    );
  }

  const throughputSeries: Series[] = visibleTargets.flatMap((t, i) => {
    const downloadColor = COLOR_CYCLE[(i * 2) % COLOR_CYCLE.length];
    const uploadColor = COLOR_CYCLE[(i * 2 + 1) % COLOR_CYCLE.length];
    const base: Series[] = [
      { key: `avg_download_bps_${t.target_id}`, label: `${t.target_name} download`, color: downloadColor, unit: formatBps },
      { key: `avg_upload_bps_${t.target_id}`, label: `${t.target_name} upload`, color: uploadColor, unit: formatBps },
    ];
    if (!compare) return base;
    return [
      ...base,
      {
        key: `avg_download_bps_${t.target_id}_prev`, label: `${t.target_name} download (prev)`,
        color: downloadColor, unit: formatBps, dashed: true,
      },
      {
        key: `avg_upload_bps_${t.target_id}_prev`, label: `${t.target_name} upload (prev)`,
        color: uploadColor, unit: formatBps, dashed: true,
      },
    ];
  });

  const latencySeries: Series[] = visibleTargets.flatMap((t, i) => {
    const pingColor = COLOR_CYCLE[(i * 2) % COLOR_CYCLE.length];
    const jitterColor = COLOR_CYCLE[(i * 2 + 1) % COLOR_CYCLE.length];
    const base: Series[] = [
      { key: `avg_ping_ms_${t.target_id}`, label: `${t.target_name} ping`, color: pingColor, unit: formatMs },
      { key: `avg_jitter_ms_${t.target_id}`, label: `${t.target_name} jitter`, color: jitterColor, unit: formatMs },
    ];
    if (!compare) return base;
    return [
      ...base,
      {
        key: `avg_ping_ms_${t.target_id}_prev`, label: `${t.target_name} ping (prev)`,
        color: pingColor, unit: formatMs, dashed: true,
      },
      {
        key: `avg_jitter_ms_${t.target_id}_prev`, label: `${t.target_name} jitter (prev)`,
        color: jitterColor, unit: formatMs, dashed: true,
      },
    ];
  });

  return (
    <div className="grid gap-4">
      <Card>
        <CardHeader className="pb-2">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <CardTitle className="text-base">History</CardTitle>
            {targets.length > 1 && (
              <div className="flex flex-wrap items-center gap-1.5">
                {targets.map((t) => (
                  <button
                    key={t.target_id}
                    type="button"
                    aria-pressed={visible.has(t.target_id)}
                    onClick={() => toggle(t.target_id)}
                    className={`rounded-full border px-2.5 py-0.5 text-xs transition-colors ${
                      visible.has(t.target_id)
                        ? 'border-accent bg-accent/10 text-accent'
                        : 'border-line text-faint hover:text-muted'
                    }`}
                  >
                    {t.target_name}
                  </button>
                ))}
              </div>
            )}
          </div>
        </CardHeader>
        <CardContent>
          <div className="mb-3 w-fit">
            <SwitchField
              id="compare-previous-period" label="Compare with previous period"
              checked={compare} onCheckedChange={setCompare}
              hint="Overlays the same range shifted back by its own span (24h → the preceding 24h), aligned by position within the range."
            />
          </div>
          <Tabs defaultValue="throughput">
            <TabsList>
              <TabsTrigger value="throughput">Throughput</TabsTrigger>
              <TabsTrigger value="latency">Latency</TabsTrigger>
            </TabsList>
            <TabsContent value="throughput">
              <HistoryChart title="Download & upload" points={throughputPoints} series={throughputSeries} />
            </TabsContent>
            <TabsContent value="latency">
              <HistoryChart title="Ping & jitter" points={latencyPoints} series={latencySeries} />
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Outages</CardTitle>
        </CardHeader>
        <CardContent>
          {outages.data
            ? <OutageStrip incidents={outages.data.incidents} from={outages.data.from} to={outages.data.to} />
            : <Skeleton className="h-8 w-full" />}
        </CardContent>
      </Card>
    </div>
  );
}

function DashboardSkeleton() {
  return (
    <div className="grid gap-4">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        {[0, 1, 2, 3].map((i) => (
          <Card key={i}><CardContent className="p-4"><Skeleton className="h-4 w-20" /><Skeleton className="mt-2 h-7 w-24" /></CardContent></Card>
        ))}
      </div>
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {[0, 1, 2].map((i) => <Card key={i}><CardContent className="p-4"><Skeleton className="h-24 w-full" /></CardContent></Card>)}
      </div>
      <Card><CardContent className="p-4"><Skeleton className="h-60 w-full" /></CardContent></Card>
    </div>
  );
}

/** Dashboard is the app's home page: a range selector, headline KPI tiles
 * with previous-period deltas, a latest-per-target card grid with
 * threshold badges, tabbed throughput/latency history, and the outage
 * timeline. */
export function Dashboard() {
  const [range, setRange] = useState<Range>('24h');
  const summary = useSummary(range);
  const previousSummary = usePreviousSummary(range);
  const targetsQuery = useTargets();
  const settingsQuery = useSettings();
  const run = useRunTarget();
  const { open } = useLivePanel();
  const runningTargetID = run.isPending ? run.variables : undefined;

  const targets = summary.data?.targets ?? [];
  const spark = aggregateSpark(targets, useAllTargetHistories(targets, range));
  const thresholdsByTarget = new Map(
    (targetsQuery.data ?? []).map((t) => [t.id, t.thresholds as ThresholdSet]),
  );

  const handleRun = (id: number) => {
    run.mutate(id, { onSuccess: () => open() });
  };

  return (
    <section className="grid gap-4">
      <header className="flex items-center justify-between">
        <h1 className="text-xl font-semibold tracking-tight">Dashboard</h1>
        <RangePicker value={range} onChange={setRange} />
      </header>

      {summary.isLoading && <DashboardSkeleton />}

      {summary.isError && (
        <div className="rounded border border-line bg-surface px-4 py-3 text-sm text-bad">
          <p>Failed to load dashboard data.</p>
          <Button type="button" variant="outline" size="sm" className="mt-2" onClick={() => summary.refetch()}>
            Retry
          </Button>
        </div>
      )}

      {summary.data && (
        <>
          <SummaryTiles
            stats={summary.data} previousStats={previousSummary.data} spark={spark}
            general={settingsQuery.data?.general}
          />

          {summary.data.targets.length === 0 ? (
            <Card>
              <CardContent className="flex flex-col items-center gap-3 p-10 text-center">
                <p className="text-sm text-muted">
                  No targets configured yet. Add one to start collecting results.
                </p>
                <Button asChild>
                  <Link to="/targets">Add a target</Link>
                </Button>
              </CardContent>
            </Card>
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
                    thresholds={thresholdsByTarget.get(t.target_id)}
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

/** DashboardCard fetches its own history for the sparkline, keyed to the
 * dashboard's selected range so it shares the cache entry with the merged
 * charts below for the same target. */
function DashboardCard({
  summary, range, onRun, running, thresholds,
}: {
  summary: TargetSummary; range: Range; onRun: (id: number) => void; running: boolean;
  thresholds?: ThresholdSet;
}) {
  const history = useTargetHistory(summary.target_id, range);
  const spark = (history.data?.points ?? []).map((p) => p.avg_download_bps);
  return <TargetCard summary={summary} spark={spark} onRun={onRun} running={running} thresholds={thresholds} />;
}
