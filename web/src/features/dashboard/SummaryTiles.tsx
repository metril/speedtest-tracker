import type { GeneralSettings, SummaryStats } from '../../lib/api';
import { formatBps, formatMs, formatPercent } from '../../lib/format';
import { SERIES } from '../../lib/chart';
import { KpiTile } from './KpiTile';

/** weightedAvg returns the count-weighted average of a per-target metric,
 * 0 when nothing has run. When excludeNonPositive is set, targets whose
 * metric value is <= 0 (e.g. a target with no successful ping reading) are
 * left out of both the numerator and denominator, rather than dragging
 * the average toward 0. */
function weightedAvg(
  stats: SummaryStats,
  pick: (t: SummaryStats['targets'][number]) => number,
  excludeNonPositive = false,
): number {
  const targets = excludeNonPositive
    ? stats.targets.filter((t) => pick(t) > 0)
    : stats.targets;
  const totalCount = targets.reduce((sum, t) => sum + t.count, 0);
  if (totalCount === 0) return 0;
  return targets.reduce((sum, t) => sum + pick(t) * t.count, 0) / totalCount;
}

/** delta returns the signed fractional change from previous to current,
 * or undefined when there is nothing to compare against. */
function delta(current: number, previous: number): number | undefined {
  if (!Number.isFinite(previous) || previous <= 0) return undefined;
  return (current - previous) / previous;
}

export interface DashboardSpark {
  download: number[];
  upload: number[];
  ping: number[];
}

/** SummaryTiles shows the four headline numbers for the selected range —
 * success rate, average download, average upload and average ping — each
 * with an optional previous-period delta and range sparkline. */
export function SummaryTiles({ stats, previousStats, spark, general }: {
  stats: SummaryStats;
  previousStats?: SummaryStats;
  spark?: DashboardSpark;
  /** general is the current SLA plan speeds, used for the "Meets plan"
   * tile's subtext. Omit if unavailable -- the tile still shows without it. */
  general?: GeneralSettings;
}) {
  if (stats.total_results === 0) {
    return (
      <div className="rounded border border-line bg-surface px-4 py-6 text-center text-sm text-muted">
        No tests in this range.
      </div>
    );
  }

  const avgDownload = weightedAvg(stats, (t) => t.avg_download_bps);
  const avgUpload = weightedAvg(stats, (t) => t.avg_upload_bps);
  const avgPing = weightedAvg(stats, (t) => t.avg_ping_ms, true);

  const prevAvgDownload = previousStats && previousStats.total_results > 0
    ? weightedAvg(previousStats, (t) => t.avg_download_bps) : undefined;
  const prevAvgUpload = previousStats && previousStats.total_results > 0
    ? weightedAvg(previousStats, (t) => t.avg_upload_bps) : undefined;
  const prevAvgPing = previousStats && previousStats.total_results > 0
    ? weightedAvg(previousStats, (t) => t.avg_ping_ms, true) : undefined;
  const prevSuccessRate = previousStats && previousStats.total_results > 0
    ? previousStats.success_rate : undefined;

  const slaCompliance = stats.sla_compliance;
  const prevSlaCompliance = previousStats && previousStats.total_results > 0
    ? previousStats.sla_compliance : undefined;
  // Omit whichever direction has no plan speed configured (0/unset)
  // rather than rendering a misleading "0" for it.
  const planDownload = general?.sla_download_mbps || undefined;
  const planUpload = general?.sla_upload_mbps || undefined;
  const planSubtext = planDownload && planUpload
    ? `${planDownload}/${planUpload} Mbps plan`
    : planDownload
      ? `${planDownload} Mbps download plan`
      : planUpload
        ? `${planUpload} Mbps upload plan`
        : undefined;

  return (
    <div className={`grid grid-cols-2 gap-3 sm:grid-cols-4 ${slaCompliance != null ? 'lg:grid-cols-5' : ''}`}>
      <KpiTile
        label="Success rate"
        value={formatPercent(stats.success_rate)}
        subtext={`${stats.total_results.toLocaleString()} tests`}
        delta={prevSuccessRate !== undefined ? delta(stats.success_rate, prevSuccessRate) : undefined}
        favorable
      />
      <KpiTile
        label="Avg download"
        value={formatBps(avgDownload)}
        spark={spark?.download}
        sparkColor={SERIES.download}
        delta={prevAvgDownload !== undefined ? delta(avgDownload, prevAvgDownload) : undefined}
        favorable
      />
      <KpiTile
        label="Avg upload"
        value={formatBps(avgUpload)}
        spark={spark?.upload}
        sparkColor={SERIES.upload}
        delta={prevAvgUpload !== undefined ? delta(avgUpload, prevAvgUpload) : undefined}
        favorable
      />
      <KpiTile
        label="Avg ping"
        value={formatMs(avgPing)}
        spark={spark?.ping}
        sparkColor={SERIES.ping}
        delta={prevAvgPing !== undefined ? delta(avgPing, prevAvgPing) : undefined}
        favorable={false}
      />
      {slaCompliance != null && (
        <KpiTile
          label="Meets plan"
          value={formatPercent(slaCompliance)}
          subtext={planSubtext}
          delta={prevSlaCompliance != null ? delta(slaCompliance, prevSlaCompliance) : undefined}
          favorable
        />
      )}
    </div>
  );
}
