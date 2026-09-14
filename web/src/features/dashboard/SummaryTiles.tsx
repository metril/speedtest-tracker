import type { SummaryStats } from '../../lib/api';
import { formatBps, formatMs, formatPercent } from '../../lib/format';

/** Tile renders one label + big value stat. */
function Tile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-line bg-surface px-4 py-3">
      <p className="text-xs uppercase tracking-wide text-muted">{label}</p>
      <p className="mt-1 text-2xl font-semibold tabular-nums text-fg">{value}</p>
    </div>
  );
}

/** SummaryTiles shows the four headline numbers for the selected range:
 * total tests run, overall success rate, the count-weighted average
 * download across targets, and the single worst ping observed. */
export function SummaryTiles({ stats }: { stats: SummaryStats }) {
  if (stats.total_results === 0) {
    return (
      <div className="rounded border border-line bg-surface px-4 py-6 text-center text-sm text-muted">
        No tests in this range.
      </div>
    );
  }

  const totalCount = stats.targets.reduce((sum, t) => sum + t.count, 0);
  const avgDownload = totalCount === 0 ? 0
    : stats.targets.reduce((sum, t) => sum + t.avg_download_bps * t.count, 0) / totalCount;
  // formatMs renders a dash for a non-positive value, so a target with no
  // ok reading in range (max_ping_ms left at 0) reads as "no data" rather
  // than a suspiciously fast "0 ms".
  const worstPing = stats.targets.length === 0 ? 0
    : Math.max(...stats.targets.map((t) => t.max_ping_ms));

  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
      <Tile label="Tests" value={String(stats.total_results)} />
      <Tile label="Success rate" value={formatPercent(stats.success_rate)} />
      <Tile label="Avg download" value={formatBps(avgDownload)} />
      <Tile label="Worst ping" value={formatMs(worstPing)} />
    </div>
  );
}
